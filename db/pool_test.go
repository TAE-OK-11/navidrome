package db_test

import (
	"runtime"
	"testing"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/db"
)

func TestMaxOpenConnsBounds(t *testing.T) {
	got := db.MaxOpenConns()
	wantMax := max(2, min(8, runtime.GOMAXPROCS(0)))
	if got != wantMax {
		t.Fatalf("MaxOpenConns() = %d, want %d", got, wantMax)
	}
	if got < 2 || got > 8 {
		t.Fatalf("MaxOpenConns() = %d, want in [2,8]", got)
	}
}

func TestUsesImmediateTxLock(t *testing.T) {
	origPath := db.Path
	origConf := conf.Server.DbPath
	t.Cleanup(func() {
		db.Path = origPath
		conf.Server.DbPath = origConf
	})

	db.Path = ""
	conf.Server.DbPath = "navidrome.db?_journal_mode=WAL"
	if db.UsesImmediateTxLock() {
		t.Fatal("expected false without _txlock=immediate")
	}

	conf.Server.DbPath = "file:/data/navidrome.db?_txlock=immediate&_busy_timeout=1000"
	if !db.UsesImmediateTxLock() {
		t.Fatal("expected true from conf DbPath")
	}

	db.Path = "navidrome.db?_TXLOCK=IMMEDIATE"
	if !db.UsesImmediateTxLock() {
		t.Fatal("expected true from Path (case-insensitive)")
	}
}
