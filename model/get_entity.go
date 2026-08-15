package model

import (
	"context"
	"errors"
)

// GetEntityByID resolves an id to the first matching library object.
// Media files are checked first because stream, download, scrobble and
// similar-song requests almost always carry a track id. A real datastore
// error (not ErrNotFound) stops the search immediately so a broken table
// cannot be mistaken for a missing row.
func GetEntityByID(ctx context.Context, ds DataStore, id string) (any, error) {
	if mf, err := ds.MediaFile(ctx).Get(id); err == nil {
		return mf, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if al, err := ds.Album(ctx).Get(id); err == nil {
		return al, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if ar, err := ds.Artist(ctx).Get(id); err == nil {
		return ar, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if pls, err := ds.Playlist(ctx).Get(id); err == nil {
		return pls, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if r, err := ds.Radio(ctx).Get(id); err == nil {
		return r, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return nil, ErrNotFound
}
