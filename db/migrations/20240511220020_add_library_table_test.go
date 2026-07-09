package migrations

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/navidrome/navidrome/conf"
)

func TestAddLibraryTableBindsMusicFolder(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if _, err = database.Exec(`
		create table property (id text);
		create table media_file (id text);
		create table album (id text);
		create table artist (id text);
	`); err != nil {
		t.Fatal(err)
	}

	originalMusicFolder := conf.Server.MusicFolder
	conf.Server.MusicFolder = "/music/Taylor's Collection"
	t.Cleanup(func() {
		conf.Server.MusicFolder = originalMusicFolder
	})

	tx, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = upAddLibraryTable(context.Background(), tx); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var path string
	if err = database.QueryRow("select path from library where id = 1").Scan(&path); err != nil {
		t.Fatal(err)
	}
	if path != conf.Server.MusicFolder {
		t.Fatalf("stored path %q, want %q", path, conf.Server.MusicFolder)
	}
}
