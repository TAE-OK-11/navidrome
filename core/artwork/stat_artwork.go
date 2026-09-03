package artwork

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/utils"
)

func maxFolderImagesUpdatedAt(folders []model.Folder) time.Time {
	var updatedAt time.Time
	for _, f := range folders {
		if f.ImagesUpdatedAt.After(updatedAt) {
			updatedAt = f.ImagesUpdatedAt
		}
	}
	return updatedAt
}

func albumImagesUpdatedAt(ctx context.Context, ds model.DataStore, album model.Album) (time.Time, error) {
	folders, err := loadFolders(ctx, ds, album.FolderIDs)
	if err != nil {
		return time.Time{}, err
	}
	parent, err := albumRootParent(ctx, ds, folders, album.FolderIDs)
	if err != nil {
		return time.Time{}, err
	}
	if parent != nil {
		folders = append(folders, *parent)
	}
	return maxFolderImagesUpdatedAt(folders), nil
}

func (a *artwork) statArtworkLastUpdated(ctx context.Context, artID model.ArtworkID, size int, square bool) (time.Time, error) {
	if size > 0 || square {
		return a.statArtworkLastUpdated(ctx, artID, 0, false)
	}

	switch artID.Kind {
	case model.KindAlbumArtwork:
		al, err := a.ds.Album(ctx).Get(artID.ID)
		if err != nil {
			return time.Time{}, err
		}
		last := utils.TimeNewest(al.UpdatedAt, al.ImportedAt)
		imagesAt, err := albumImagesUpdatedAt(ctx, a.ds, *al)
		if err != nil {
			return time.Time{}, err
		}
		if imagesAt.After(last) {
			last = imagesAt
		}
		return last, nil
	case model.KindMediaFileArtwork:
		mf, err := a.ds.MediaFile(ctx).Get(artID.ID)
		if err != nil {
			return time.Time{}, err
		}
		al, err := a.ds.Album(ctx).Get(mf.AlbumID)
		if err != nil {
			return time.Time{}, err
		}
		last := mf.UpdatedAt
		if al.UpdatedAt.After(last) {
			last = al.UpdatedAt
		}
		imagesAt, err := albumImagesUpdatedAt(ctx, a.ds, *al)
		if err != nil {
			return time.Time{}, err
		}
		if imagesAt.After(last) {
			last = imagesAt
		}
		return last, nil
	case model.KindDiscArtwork:
		albumID, _, err := model.ParseDiscArtworkID(artID.ID)
		if err != nil {
			return time.Time{}, err
		}
		al, err := a.ds.Album(ctx).Get(albumID)
		if err != nil {
			return time.Time{}, err
		}
		last := utils.TimeNewest(al.UpdatedAt, al.ImportedAt)
		imagesAt, err := albumImagesUpdatedAt(ctx, a.ds, *al)
		if err != nil {
			return time.Time{}, err
		}
		if imagesAt.After(last) {
			last = imagesAt
		}
		return last, nil
	case model.KindPlaylistArtwork:
		return statPlaylistArtworkLastUpdated(ctx, a.ds, artID)
	case model.KindRadioArtwork:
		r, err := a.ds.Radio(ctx).Get(artID.ID)
		if err != nil {
			return time.Time{}, err
		}
		return r.UpdatedAt, nil
	default:
		reader, err := a.getArtworkReader(ctx, artID, size, square)
		if err != nil {
			return time.Time{}, err
		}
		return reader.LastUpdated(), nil
	}
}

func statPlaylistArtworkLastUpdated(ctx context.Context, ds model.DataStore, artID model.ArtworkID) (time.Time, error) {
	pl, err := ds.Playlist(ctx).Get(artID.ID)
	if err != nil {
		return time.Time{}, err
	}
	last := pl.UpdatedAt
	for _, path := range []string{
		findPlaylistSidecarPath(ctx, pl.Path),
		pl.ExternalImageURL,
	} {
		if path == "" || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.ModTime().After(last) {
			last = info.ModTime()
		}
	}
	return last, nil
}
