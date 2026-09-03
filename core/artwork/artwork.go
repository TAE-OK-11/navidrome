package artwork

import (
	"context"
	"errors"
	_ "image/gif"
	"io"
	"strconv"
	"time"

	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/external"
	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/resources"
	"github.com/navidrome/navidrome/utils/cache"
	_ "golang.org/x/image/webp"
	"golang.org/x/sync/singleflight"
)

var ErrUnavailable = errors.New("artwork unavailable")

const (
	artworkReaderCacheSize = 2048
	artworkReaderCacheTTL  = 30 * time.Second
	artworkStatCacheSize   = 4096
	artworkStatCacheTTL    = 30 * time.Second
)

type Artwork interface {
	Get(ctx context.Context, artID model.ArtworkID, size int, square bool) (io.ReadCloser, time.Time, error)
	GetOrPlaceholder(ctx context.Context, id string, size int, square bool) (io.ReadCloser, time.Time, error)
	// StatOrPlaceholder returns the cover's last-updated time without opening
	// the image cache, so HTTP 304 responses can skip disk and decode work.
	StatOrPlaceholder(ctx context.Context, id string, size int, square bool) (time.Time, error)
}

func NewArtwork(ds model.DataStore, imageCache cache.FileCache, ffmpeg ffmpeg.FFmpeg, provider external.Provider) Artwork {
	return &artwork{
		ds:       ds,
		cache:    imageCache,
		ffmpeg:   ffmpeg,
		provider: provider,
		readerCache: cache.NewSimpleCache[string, artworkReader](cache.Options{
			SizeLimit:  artworkReaderCacheSize,
			DefaultTTL: artworkReaderCacheTTL,
		}),
		statCache: cache.NewSimpleCache[string, time.Time](cache.Options{
			SizeLimit:  artworkStatCacheSize,
			DefaultTTL: artworkStatCacheTTL,
		}),
	}
}

type artwork struct {
	ds          model.DataStore
	cache       cache.FileCache
	ffmpeg      ffmpeg.FFmpeg
	provider    external.Provider
	readers     singleflight.Group
	readerCache cache.SimpleCache[string, artworkReader]
	statCache   cache.SimpleCache[string, time.Time]
	statGroup   singleflight.Group
}

type artworkReader interface {
	cache.Item
	LastUpdated() time.Time
	Reader(ctx context.Context) (io.ReadCloser, string, error)
}

func (a *artwork) GetOrPlaceholder(ctx context.Context, id string, size int, square bool) (reader io.ReadCloser, lastUpdate time.Time, err error) {
	artID, err := a.getArtworkId(ctx, id)
	if err == nil {
		reader, lastUpdate, err = a.Get(ctx, artID, size, square)
	}
	if errors.Is(err, ErrUnavailable) {
		if artID.Kind == model.KindArtistArtwork {
			reader, _ = resources.FS().Open(consts.PlaceholderArtistArt)
		} else {
			reader, _ = resources.FS().Open(consts.PlaceholderAlbumArt)
		}
		return reader, consts.ServerStart, nil
	}
	return reader, lastUpdate, err
}

func (a *artwork) StatOrPlaceholder(ctx context.Context, id string, size int, square bool) (time.Time, error) {
	artID, err := a.getArtworkId(ctx, id)
	if err == nil {
		statSize := size
		statSquare := square
		if size > 0 || square {
			statSize = 0
			statSquare = false
		}
		cacheKey := a.artworkCacheKey(ctx, artID, statSize, statSquare, 's')
		if a.statCache != nil {
			if lastUpdate, cacheErr := a.statCache.Get(cacheKey); cacheErr == nil {
				return lastUpdate, nil
			}
		}
		value, statErr, _ := a.statGroup.Do(cacheKey, func() (any, error) {
			if a.statCache != nil {
				if lastUpdate, cacheErr := a.statCache.Get(cacheKey); cacheErr == nil {
					return lastUpdate, nil
				}
			}
			lastUpdate, loadErr := a.statArtworkLastUpdated(ctx, artID, statSize, statSquare)
			if loadErr == nil && a.statCache != nil {
				_ = a.statCache.Add(cacheKey, lastUpdate)
			}
			return lastUpdate, loadErr
		})
		if statErr != nil {
			err = statErr
		} else {
			return value.(time.Time), nil
		}
	}
	if errors.Is(err, ErrUnavailable) {
		return consts.ServerStart, nil
	}
	return time.Time{}, err
}

func (a *artwork) Get(ctx context.Context, artID model.ArtworkID, size int, square bool) (reader io.ReadCloser, lastUpdate time.Time, err error) {
	artReader, err := a.getArtworkReader(ctx, artID, size, square)
	if err != nil {
		return nil, time.Time{}, err
	}

	r, err := a.cache.Get(ctx, artReader)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrUnavailable) {
			log.Error(ctx, "Error accessing image cache", "id", artID, "size", size, err)
		}
		return nil, time.Time{}, err
	}
	return r, artReader.LastUpdated(), nil
}

type coverArtGetter interface {
	CoverArtID() model.ArtworkID
}

func (a *artwork) getArtworkId(ctx context.Context, id string) (model.ArtworkID, error) {
	if id == "" {
		return model.ArtworkID{}, ErrUnavailable
	}
	artID, err := model.ParseArtworkID(id)
	if err == nil {
		return artID, nil
	}

	log.Trace(ctx, "ArtworkID invalid. Trying to figure out kind based on the ID", "id", id)
	entity, err := model.GetEntityByID(ctx, a.ds, id)
	if err != nil {
		return model.ArtworkID{}, err
	}
	if e, ok := entity.(coverArtGetter); ok {
		artID = e.CoverArtID()
	}
	switch e := entity.(type) {
	case *model.Artist:
		log.Trace(ctx, "ID is for an Artist", "id", id, "name", e.Name, "artist", e.Name)
	case *model.Album:
		log.Trace(ctx, "ID is for an Album", "id", id, "name", e.Name, "artist", e.AlbumArtist)
	case *model.MediaFile:
		log.Trace(ctx, "ID is for a MediaFile", "id", id, "title", e.Title, "album", e.Album)
	case *model.Playlist:
		log.Trace(ctx, "ID is for a Playlist", "id", id, "name", e.Name)
	}
	return artID, nil
}

func (a *artwork) getArtworkReader(ctx context.Context, artID model.ArtworkID, size int, square bool) (artworkReader, error) {
	cacheKey := a.artworkCacheKey(ctx, artID, size, square, 'r')
	if a.readerCache != nil {
		if reader, cacheErr := a.readerCache.Get(cacheKey); cacheErr == nil {
			return reader, nil
		}
	}

	value, err, _ := a.readers.Do(cacheKey, func() (any, error) {
		if a.readerCache != nil {
			if reader, cacheErr := a.readerCache.Get(cacheKey); cacheErr == nil {
				return reader, nil
			}
		}
		reader, buildErr := a.buildArtworkReader(ctx, artID, size, square)
		if buildErr == nil && a.readerCache != nil {
			_ = a.readerCache.Add(cacheKey, reader)
		}
		return reader, buildErr
	})
	if err != nil {
		return nil, err
	}
	return value.(artworkReader), nil
}

func (a *artwork) buildArtworkReader(ctx context.Context, artID model.ArtworkID, size int, square bool) (artworkReader, error) {
	var artReader artworkReader
	var err error
	if size > 0 || square {
		artReader, err = resizedFromOriginal(ctx, a, artID, size, square)
	} else {
		switch artID.Kind {
		case model.KindArtistArtwork:
			artReader, err = newArtistArtworkReader(ctx, a, artID, a.provider)
		case model.KindAlbumArtwork:
			artReader, err = newAlbumArtworkReader(ctx, a, artID, a.provider)
		case model.KindMediaFileArtwork:
			artReader, err = newMediafileArtworkReader(ctx, a, artID)
		case model.KindPlaylistArtwork:
			artReader, err = newPlaylistArtworkReader(ctx, a, artID)
		case model.KindDiscArtwork:
			artReader, err = newDiscArtworkReader(ctx, a, artID)
		case model.KindRadioArtwork:
			artReader, err = newRadioArtworkReader(ctx, a, artID)
		default:
			return nil, ErrUnavailable
		}
	}
	return artReader, err
}

func (a *artwork) artworkCacheKey(ctx context.Context, artID model.ArtworkID, size int, square bool, kind byte) string {
	artworkID := artID.String()
	key := make([]byte, 0, len(artworkID)+34)
	key = append(key, kind)
	if user, ok := request.UserFrom(ctx); ok {
		key = append(key, 'u')
		key = append(key, user.ID...)
		key = append(key, 0)
		key = strconv.AppendBool(key, user.IsAdmin)
		for _, library := range user.Libraries {
			key = append(key, ',')
			key = strconv.AppendInt(key, int64(library.ID), 10)
		}
	} else {
		key = append(key, 'n')
	}
	key = append(key, 0)
	key = append(key, artworkID...)
	key = append(key, 0)
	key = strconv.AppendInt(key, int64(size), 10)
	key = append(key, 0)
	key = strconv.AppendBool(key, square)
	return string(key)
}
