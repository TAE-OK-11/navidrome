package scanner

import (
	"context"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/navidrome/navidrome/model"
)

func TestWalkDirTreeSkipsGoFallbackWhenDisabled(t *testing.T) {
	orig := allowGoWalkerFallback
	allowGoWalkerFallback = func() bool { return false }
	t.Cleanup(func() { allowGoWalkerFallback = orig })

	ctx := context.Background()
	job := &scanJob{
		localRoot: filepath.Join(t.TempDir(), "missing-root"),
		lib:       model.Library{Name: "test", Path: "/music"},
		fs:        &mockMusicFS{FS: fstest.MapFS{"song.mp3": &fstest.MapFile{Data: []byte("x")}}},
	}
	results, err := walkDirTree(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for range results {
		n++
	}
	if n != 0 {
		t.Fatalf("got %d folders, want none when Go walker fallback is disabled", n)
	}
}

func TestWalkDirTreeFakeFSStillUsesGoWalker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	job := &scanJob{
		fs: &mockMusicFS{FS: fstest.MapFS{
			"album/song.mp3": &fstest.MapFile{Data: []byte("x")},
		}},
		lib: model.Library{Name: "fake", Path: "/music"},
	}
	results, err := walkDirTree(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	folders := map[string]struct{}{}
	for folder := range results {
		folders[folder.path] = struct{}{}
	}
	if _, ok := folders["album"]; !ok {
		t.Fatalf("FakeFS walk missing album folder: %v", folders)
	}
}

func TestFolderFromProtoRoundTrip(t *testing.T) {
	t.Parallel()

	src := &rustScanFolder{
		Path:              "Artist/Album",
		ModTimeNS:         123,
		ImagesUpdatedAtNS: 456,
		NumPlaylists:      1,
		NumSubfolders:     2,
		AudioFiles:        map[string]rustScanFile{"a.mp3": {Name: "a.mp3", Size: 10, ModTimeNS: 1}},
		ImageFiles:        map[string]rustScanFile{"cover.jpg": {Name: "cover.jpg", Size: 20, ModTimeNS: 2}},
		Hash:              "abc",
	}
	got := folderFromProto(toProtoWalkFolder(src))
	if got.Path != src.Path || got.Hash != src.Hash || got.NumPlaylists != src.NumPlaylists {
		t.Fatalf("proto round trip mismatch: %+v", got)
	}
	if got.AudioFiles["a.mp3"].Size != 10 || got.ImageFiles["cover.jpg"].Size != 20 {
		t.Fatalf("proto file round trip mismatch: %+v", got)
	}
}
