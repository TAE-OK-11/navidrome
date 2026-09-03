package persistence

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestPluginKVNamespacedIsolation(t *testing.T) {
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "plugin.db")+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := EnsurePluginRuntimeSchema(conn); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	kv := New(conn).PluginKV(ctx)
	if err := kv.Put(ctx, "plugin-a", "shared", []byte("a"), 0); err != nil {
		t.Fatal(err)
	}
	if err := kv.Put(ctx, "plugin-b", "shared", []byte("b"), 0); err != nil {
		t.Fatal(err)
	}

	a, found, err := kv.Get(ctx, "plugin-a", "shared")
	if err != nil || !found || string(a) != "a" {
		t.Fatalf("plugin-a: value=%q found=%v err=%v", a, found, err)
	}
	b, found, err := kv.Get(ctx, "plugin-b", "shared")
	if err != nil || !found || string(b) != "b" {
		t.Fatalf("plugin-b: value=%q found=%v err=%v", b, found, err)
	}
}
