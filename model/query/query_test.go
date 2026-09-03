package query_test

import (
	"testing"

	"github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/query"
)

func TestParticipantIDFilter(t *testing.T) {
	sql, args, err := query.ParticipantIDFilter("media_file", "ar-1", model.RoleArtist).ToSql()
	if err != nil {
		t.Fatal(err)
	}
	want := "media_file.id IN (SELECT media_file_id FROM media_file_artists WHERE artist_id = ? AND role IN (?))"
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
	if len(args) != 2 || args[0] != "ar-1" || args[1] != "artist" {
		t.Fatalf("args = %#v", args)
	}
}

func TestNotParticipantIDFilter(t *testing.T) {
	sql, _, err := query.NotParticipantIDFilter("album", "ar-1", model.RoleAlbumArtist).ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if sql == "" || sql[:8] != "album.id" {
		t.Fatalf("unexpected sql %q", sql)
	}
}

func TestSongGenresByName(t *testing.T) {
	sql, args, err := query.SongGenres.ByName("Rock").ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if sql == "" || len(args) != 2 {
		t.Fatalf("sql=%q args=%#v", sql, args)
	}
}

func TestExists(t *testing.T) {
	sql, args, err := query.Exists("album", squirrel.Eq{"id": 1}).ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if sql != "exists (select 1 from album where id = ?)" {
		t.Fatalf("sql = %q", sql)
	}
	if len(args) != 1 || args[0] != 1 {
		t.Fatalf("args = %#v", args)
	}
}

func TestNotMissing(t *testing.T) {
	sql, args, err := query.NotMissing().ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if sql != "missing = ?" || len(args) != 1 || args[0] != false {
		t.Fatalf("sql=%q args=%#v", sql, args)
	}
}

func TestOrAnd(t *testing.T) {
	sql, _, err := query.Or(query.Eq("a", 1), query.Eq("b", 2)).ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if sql == "" {
		t.Fatal("expected sql")
	}
}
