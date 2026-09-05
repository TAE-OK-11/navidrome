package db_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/db"
	"github.com/navidrome/navidrome/log"
)

// BenchmarkSQLiteHotPath exercises the CGO sqlite amalgamation through the
// production driver (ConnectHook pragmas, WAL, stmt cache, pooled conns).
// Included in release/pgo-train.sh so Go PGO samples sqlite-heavy frames.
func BenchmarkSQLiteHotPath(b *testing.B) {
	log.SetLevel(log.LevelFatal)
	tmpDir := b.TempDir()
	conf.Server.DataFolder = conf.NewDir(tmpDir)
	conf.Server.DbPath = filepath.Join(tmpDir, "hotpath.db") +
		"?_journal_mode=WAL&_synchronous=NORMAL&_txlock=immediate&_busy_timeout=5000" +
		"&_foreign_keys=on&_cache_size=-8192&_stmt_cache_size=64&_secure_delete=OFF"

	cleanup := db.Init(context.Background())
	b.Cleanup(cleanup)
	sqlDB := db.Db()
	ctx := context.Background()

	// Warm schema + planner; property table exists after migrations.
	if _, err := sqlDB.ExecContext(ctx, `insert into property(id, value) values(?, ?)
		on conflict(id) do update set value=excluded.value`, "bench-warm", "1"); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		key := fmt.Sprintf("bench-%d", i%64)
		val := fmt.Sprintf("v-%d", i)
		if _, err := sqlDB.ExecContext(ctx, `insert into property(id, value) values(?, ?)
			on conflict(id) do update set value=excluded.value`, key, val); err != nil {
			b.Fatal(err)
		}
		var got string
		if err := sqlDB.QueryRowContext(ctx, `select value from property where id=?`, key).Scan(&got); err != nil {
			b.Fatal(err)
		}
		if got != val {
			b.Fatalf("got %q want %q", got, val)
		}
		i++
	}
}
