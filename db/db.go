package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"runtime"
	"time"

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
		return db
	})
}

func maxOpenConns() int {
	return max(2, min(16, runtime.GOMAXPROCS(0)*2))
}

func configureSQLiteConn(conn *sqlite3.SQLiteConn) error {
	_, err := conn.Exec(`
		PRAGMA foreign_keys=ON;
		PRAGMA temp_store=MEMORY;
		PRAGMA mmap_size=134217728;
		PRAGMA cache_spill=OFF;
	`, nil)
	return err
}

func Close(ctx context.Context) {
	// Ignore cancellations when closing the DB
	ctx = context.WithoutCancel(ctx)

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

	// SQLite PRAGMAs such as foreign_keys are connection-scoped. Migrations must see
	// the same setting that we establish here, so temporarily keep the pool to one
	// persistent idle connection. Without this, goose may acquire a different pooled
	// connection and run a table-recreating migration with foreign_keys still enabled.
	// Preserve the caller's configured pool size rather than assuming the production
	// default: tests and embedders deliberately restrict this pool in some environments.
	previousMaxConns := db.Stats().MaxOpenConnections
	if previousMaxConns <= 0 {
		previousMaxConns = maxOpenConns()
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Disable foreign_keys to allow re-creating tables in migrations. The sole
	// connection is normally configured with foreign_keys=ON by ConnectHook; after
	// migrations we re-enable it explicitly, and every future pooled connection gets
	// the same ON setting when it is created.
	_, err := db.ExecContext(ctx, "PRAGMA foreign_keys=off")
	defer func() {
		// Startup cancellation must never leave the sole pooled connection with
		// foreign-key enforcement disabled.
		cleanupCtx := context.WithoutCancel(ctx)
		if _, enableErr := db.ExecContext(cleanupCtx, "PRAGMA foreign_keys=on"); enableErr != nil {
			log.Error(cleanupCtx, "Error re-enabling foreign_keys", enableErr)
		}
		// Set MaxOpen first: SetMaxIdleConns is otherwise capped by the temporary
		// one-connection maximum and would remain at one after initialization.
		db.SetMaxOpenConns(previousMaxConns)
		db.SetMaxIdleConns(previousMaxConns)
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

	if hasSchemaChanges {
		log.Debug(ctx, "Running ANALYZE after schema changes")
		err = optimizeAt(ctx, db, time.Now())
		if err != nil {
			log.Error(ctx, "Error running ANALYZE", err)
		}
	}

	return func() {
		Close(ctx)
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
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='goose_db_version';")
	if err != nil {
		log.Fatal(ctx, "Database could not be opened!", err)
	}
	defer rows.Close()
	if rows.Next() {
		return false
	}
	if err = rows.Err(); err != nil {
		log.Fatal(ctx, "Error checking database schema", err)
	}
	return true
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
