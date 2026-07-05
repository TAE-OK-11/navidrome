package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"runtime"
	"strings"

	"github.com/mattn/go-sqlite3"
	"github.com/navidrome/navidrome/conf"
	_ "github.com/navidrome/navidrome/db/migrations"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/utils/hasher"
	"github.com/navidrome/navidrome/utils/singleton"
	"github.com/pressly/goose/v3"
)

var (
	Dialect = "sqlite3"
	Driver  = Dialect + "_custom"
	Path    string
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

const migrationsFolder = "migrations"

func Db() *sql.DB {
	return singleton.GetInstance(func() *sql.DB {
		sql.Register(Driver, &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if err := conn.RegisterFunc("SEEDEDRAND", hasher.HashFunc(), false); err != nil {
					return err
				}
				return configureSQLiteConn(conn)
			},
		})
		Path = conf.Server.DbPath
		if Path == ":memory:" {
			Path = "file::memory:?cache=shared&_foreign_keys=on"
			conf.Server.DbPath = Path
		} else {
			conf.Server.DataFolder.MustPath()
		}
		log.Debug("Opening DataBase", "dbPath", Path, "driver", Driver)
		db, err := sql.Open(Driver, Path)
		if err != nil {
			log.Fatal("Error opening database", err)
		}
		if db == nil {
			log.Fatal("Error opening database: sql.Open returned nil DB")
		}
		maxConns := maxOpenConns()
		db.SetMaxOpenConns(maxConns)
		db.SetMaxIdleConns(maxConns)
		if conf.Server.DevOptimizeDB {
			_, err = db.Exec("PRAGMA optimize=0x10002")
			if err != nil {
				fatalOnReadonlyDB("Error applying PRAGMA optimize", err)
				log.Error("Error applying PRAGMA optimize", err)
			}
		}
		return db
	})
}

func maxOpenConns() int {
	return max(2, min(16, runtime.GOMAXPROCS(0)*2))
}

func configureSQLiteConn(conn *sqlite3.SQLiteConn) error {
	_, err := conn.Exec(`
		PRAGMA temp_store=MEMORY;
		PRAGMA mmap_size=134217728;
		PRAGMA cache_spill=OFF;
	`, nil)
	return err
}

func Close(ctx context.Context) {
	// Ignore cancellations when closing the DB
	ctx = context.WithoutCancel(ctx)

	// Run optimize before closing
	Optimize(ctx)

	log.Info(ctx, "Closing Database")
	err := Db().Close()
	if err != nil {
		log.Error(ctx, "Error closing Database", err)
	}
}

func Init(ctx context.Context) func() {
	db := Db()
	if db == nil {
		log.Fatal(ctx, "Database initialization failed: nil DB")
	}

	// Disable foreign_keys to allow re-creating tables in migrations
	_, err := db.ExecContext(ctx, "PRAGMA foreign_keys=off")
	defer func() {
		_, err := db.ExecContext(ctx, "PRAGMA foreign_keys=on")
		if err != nil {
			log.Error(ctx, "Error re-enabling foreign_keys", err)
		}
	}()
	if err != nil {
		log.Error(ctx, "Error disabling foreign_keys", err)
	}

	goose.SetBaseFS(embedMigrations)
	err = goose.SetDialect(Dialect)
	if err != nil {
		log.Fatal(ctx, "Invalid DB driver", "driver", Driver, err)
	}
	schemaEmpty := isSchemaEmpty(ctx, db)
	hasSchemaChanges := !schemaEmpty && hasPendingMigrations(ctx, db, migrationsFolder)
	if !schemaEmpty && hasSchemaChanges {
		log.Info(ctx, "Upgrading DB Schema to latest version")
	}
	goose.SetLogger(&logAdapter{ctx: ctx, silent: schemaEmpty})
	err = goose.UpContext(ctx, db, migrationsFolder)
	if err != nil {
		log.Fatal(ctx, "Failed to apply new migrations", err)
	}

	if hasSchemaChanges && conf.Server.DevOptimizeDB {
		log.Debug(ctx, "Applying PRAGMA optimize after schema changes")
		_, err = db.ExecContext(ctx, "PRAGMA optimize")
		if err != nil {
			fatalOnReadonlyDB("Error applying PRAGMA optimize after schema changes", err)
			log.Error(ctx, "Error applying PRAGMA optimize", err)
		}
	}

	return func() {
		Close(ctx)
	}
}

// Optimize runs PRAGMA optimize on each connection in the pool
func Optimize(ctx context.Context) {
	if !conf.Server.DevOptimizeDB {
		return
	}
	numConns := Db().Stats().OpenConnections
	if numConns == 0 {
		log.Debug(ctx, "No open connections to optimize")
		return
	}
	log.Debug(ctx, "Optimizing open connections", "numConns", numConns)
	var conns []*sql.Conn
	for range numConns {
		conn, err := Db().Conn(ctx)
		if err != nil {
			fatalOnReadonlyDB("Error getting connection from pool", err)
			log.Error(ctx, "Error getting connection from pool", err)
			continue
		}
		conns = append(conns, conn)
		_, err = conn.ExecContext(ctx, "PRAGMA optimize;")
		if err != nil {
			fatalOnReadonlyDB("Error running PRAGMA optimize", err)
			log.Error(ctx, "Error running PRAGMA optimize", err)
		}
	}

	// Return all connections to the Connection Pool
	for _, conn := range conns {
		if conn != nil {
			_ = conn.Close()
		}
	}
}

func fatalOnReadonlyDB(msg string, err error) {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "readonly database") {
		log.Fatal(msg, err)
	}
}

type statusLogger struct{ numPending int }

func (*statusLogger) Fatalf(format string, v ...any) { log.Fatal(fmt.Sprintf(format, v...)) }
func (l *statusLogger) Printf(format string, v ...any) {
	if len(v) < 1 {
		return
	}
	if v0, ok := v[0].(string); !ok {
		return
	} else if v0 == "Pending" {
		l.numPending++
	}
}

func hasPendingMigrations(ctx context.Context, db *sql.DB, folder string) bool {
	l := &statusLogger{}
	goose.SetLogger(l)
	err := goose.StatusContext(ctx, db, folder)
	if err != nil {
		log.Fatal(ctx, "Failed to check for pending migrations", err)
	}
	return l.numPending > 0
}

func isSchemaEmpty(ctx context.Context, db *sql.DB) bool {
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='goose_db_version';") // nolint:rowserrcheck
	if err != nil {
		log.Fatal(ctx, "Database could not be opened!", err)
	}
	defer rows.Close()
	return !rows.Next()
}

type logAdapter struct {
	ctx    context.Context
	silent bool
}

func (l *logAdapter) Fatal(v ...any) {
	log.Fatal(l.ctx, fmt.Sprint(v...))
}

func (l *logAdapter) Fatalf(format string, v ...any) {
	log.Fatal(l.ctx, fmt.Sprintf(format, v...))
}

func (l *logAdapter) Print(v ...any) {
	if !l.silent {
		log.Info(l.ctx, fmt.Sprint(v...))
	}
}

func (l *logAdapter) Println(v ...any) {
	if !l.silent {
		log.Info(l.ctx, fmt.Sprintln(v...))
	}
}

func (l *logAdapter) Printf(format string, v ...any) {
	if !l.silent {
		log.Info(l.ctx, fmt.Sprintf(format, v...))
	}
}
