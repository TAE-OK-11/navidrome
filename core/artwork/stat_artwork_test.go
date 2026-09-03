package artwork

import (
	"context"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
)

func TestStatAlbumArtworkLastUpdated(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	imagesAt := now.Add(-time.Hour)
	album := model.Album{
		ID:        "al-1",
		UpdatedAt: now,
		FolderIDs: []string{"f1"},
	}
	folderRepo := &fakeFolderRepo{
		result: []model.Folder{{ImagesUpdatedAt: imagesAt}},
	}
	ds := &tests.MockDataStore{
		MockedFolder: folderRepo,
	}
	ds.Album(context.Background()).(*tests.MockAlbumRepo).SetData(model.Albums{album})

	aw := NewArtwork(ds, GetImageCache(), tests.NewMockFFmpeg(""), nil).(*artwork)
	last, err := aw.statArtworkLastUpdated(context.Background(), album.CoverArtID(), 0, false)
	if err != nil {
		t.Fatalf("statArtworkLastUpdated: %v", err)
	}
	if !last.Equal(now) {
		t.Fatalf("expected album UpdatedAt %v, got %v", now, last)
	}
}

func TestStatOrPlaceholderSkipsReaderCache(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	album := model.Album{
		ID:        "al-1",
		UpdatedAt: now,
		FolderIDs: []string{"f1"},
	}
	folderRepo := &fakeFolderRepo{
		result: []model.Folder{{ImagesUpdatedAt: now.Add(-time.Hour)}},
	}
	ds := &tests.MockDataStore{
		MockedFolder: folderRepo,
	}
	ds.Album(context.Background()).(*tests.MockAlbumRepo).SetData(model.Albums{album})

	aw := NewArtwork(ds, GetImageCache(), tests.NewMockFFmpeg(""), nil).(*artwork)
	_, err := aw.StatOrPlaceholder(context.Background(), album.CoverArtID().String(), 0, false)
	if err != nil {
		t.Fatalf("StatOrPlaceholder: %v", err)
	}
	if aw.readerCache.Len() != 0 {
		t.Fatalf("expected reader cache to stay empty, got %d entries", aw.readerCache.Len())
	}
	if aw.statCache.Len() != 1 {
		t.Fatalf("expected stat cache entry, got %d entries", aw.statCache.Len())
	}
}
