package subsonic

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/core/scrobbler"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server/subsonic/filter"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	"github.com/navidrome/navidrome/utils/req"
	"github.com/navidrome/navidrome/utils/run"
	"github.com/navidrome/navidrome/utils/slice"
)

const (
	albumListCacheTTL   = 45 * time.Second
	albumListCacheLimit = 128
)

type albumListCacheEntry struct {
	expires time.Time
	albums  model.Albums
	count   int64
}

type albumListCache struct {
	mu      sync.RWMutex
	entries map[string]albumListCacheEntry
}

func (c *albumListCache) get(key string, now time.Time) (model.Albums, int64, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || !now.Before(entry.expires) {
		if ok {
			c.mu.Lock()
			delete(c.entries, key)
			c.mu.Unlock()
		}
		return nil, 0, false
	}
	return entry.albums, entry.count, true
}

func (c *albumListCache) put(key string, now time.Time, albums model.Albums, count int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]albumListCacheEntry)
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= albumListCacheLimit {
		for candidate, value := range c.entries {
			if !now.Before(value.expires) {
				delete(c.entries, candidate)
			}
		}
	}
	if len(c.entries) >= albumListCacheLimit {
		var oldestKey string
		var oldestExpiry time.Time
		for candidate, value := range c.entries {
			if oldestKey == "" || value.expires.Before(oldestExpiry) {
				oldestKey, oldestExpiry = candidate, value.expires
			}
		}
		delete(c.entries, oldestKey)
	}
	c.entries[key] = albumListCacheEntry{albums: albums, count: count, expires: now.Add(albumListCacheTTL)}
}

func (c *albumListCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]albumListCacheEntry)
}

func albumListCacheKey(r *http.Request, typ string, musicFolderIds []int, offset, size int, genre string, fromYear, toYear int) string {
	user, ok := request.UserFrom(r.Context())
	userKey := ""
	if ok {
		userKey = genreResponseCacheKey(user)
	}
	ids := append([]int(nil), musicFolderIds...)
	slices.Sort(ids)
	var key strings.Builder
	key.Grow(64 + len(ids)*4 + len(genre))
	key.WriteString(typ)
	key.WriteByte('|')
	key.WriteString(userKey)
	key.WriteByte('|')
	key.WriteString(strconv.Itoa(offset))
	key.WriteByte('|')
	key.WriteString(strconv.Itoa(size))
	key.WriteByte('|')
	for _, id := range ids {
		key.WriteString(strconv.Itoa(id))
		key.WriteByte(',')
	}
	if genre != "" {
		key.WriteByte('|')
		key.WriteString(genre)
	}
	if fromYear != 0 || toYear != 0 {
		key.WriteByte('|')
		key.WriteString(strconv.Itoa(fromYear))
		key.WriteByte('-')
		key.WriteString(strconv.Itoa(toYear))
	}
	return key.String()
}

func (api *Router) getAlbumList(r *http.Request) (model.Albums, int64, error) {
	p := req.Params(r)
	typ, err := p.String("type")
	if err != nil {
		return nil, 0, err
	}

	var genre string
	var fromYear, toYear int
	var opts filter.Options
	switch typ {
	case "newest":
		opts = filter.AlbumsByNewest()
	case "recent":
		opts = filter.AlbumsByRecent()
	case "random":
		opts = filter.AlbumsByRandom()
	case "alphabeticalByName":
		opts = filter.AlbumsByName()
	case "alphabeticalByArtist":
		opts = filter.AlbumsByArtist()
	case "frequent":
		opts = filter.AlbumsByFrequent()
	case "starred":
		opts = filter.ByStarred()
	case "highest":
		opts = filter.ByRating()
	case "byGenre":
		genre, err = p.String("genre")
		if err != nil {
			return nil, 0, err
		}
		opts = filter.AlbumsByGenre(genre)
	case "byYear":
		fromYear, err = p.Int("fromYear")
		if err != nil {
			return nil, 0, err
		}
		toYear, err = p.Int("toYear")
		if err != nil {
			return nil, 0, err
		}
		opts = filter.AlbumsByYear(fromYear, toYear)
	default:
		log.Error(r, "albumList type not implemented", "type", typ)
		return nil, 0, newError(responses.ErrorGeneric, "type '%s' not implemented", typ)
	}

	// Get optional library IDs from musicFolderId parameter
	musicFolderIds, err := selectedMusicFolderIds(r, false)
	if err != nil {
		return nil, 0, err
	}
	opts = filter.ApplyLibraryFilter(opts, musicFolderIds)

	opts.Offset = p.IntOr("offset", 0)
	opts.Max = min(p.IntOr("size", 10), 500)

	cacheable := typ != "random" && typ != "recent" && typ != "frequent"
	now := time.Now()
	if cacheable {
		cacheKey := albumListCacheKey(r, typ, musicFolderIds, opts.Offset, opts.Max, genre, fromYear, toYear)
		if albums, count, ok := api.albumListCache.get(cacheKey, now); ok {
			return albums, count, nil
		}
	}

	var albums model.Albums
	var count int64
	err = run.Parallel(
		func() error {
			var err error
			albums, err = api.ds.Album(r.Context()).GetAll(opts)
			if err != nil {
				log.Error(r, "Error retrieving albums", err)
				return newError(responses.ErrorGeneric, "internal error")
			}
			return nil
		},
		func() error {
			var err error
			count, err = api.ds.Album(r.Context()).CountAll(opts)
			if err != nil {
				log.Error(r, "Error counting albums", err)
				return newError(responses.ErrorGeneric, "internal error")
			}
			return nil
		},
	)()
	if err != nil {
		return nil, 0, err
	}

	if cacheable {
		cacheKey := albumListCacheKey(r, typ, musicFolderIds, opts.Offset, opts.Max, genre, fromYear, toYear)
		api.albumListCache.put(cacheKey, now, albums, count)
	}

	return albums, count, nil
}

func (api *Router) GetAlbumList(w http.ResponseWriter, r *http.Request) (*responses.Subsonic, error) {
	albums, count, err := api.getAlbumList(r)
	if err != nil {
		return nil, err
	}

	w.Header().Set("x-total-count", strconv.Itoa(int(count)))

	response := newResponse()
	response.AlbumList = &responses.AlbumList{
		Album: slice.MapWithArg(albums, r.Context(), childFromAlbum),
	}
	return response, nil
}

func (api *Router) GetAlbumList2(w http.ResponseWriter, r *http.Request) (*responses.Subsonic, error) {
	albums, pageCount, err := api.getAlbumList(r)
	if err != nil {
		return nil, err
	}

	w.Header().Set("x-total-count", strconv.FormatInt(pageCount, 10))

	response := newResponse()
	response.AlbumList2 = &responses.AlbumList2{
		Album: slice.MapWithArg(albums, r.Context(), buildAlbumID3),
	}
	return response, nil
}

func (api *Router) getStarredItems(r *http.Request) (model.Artists, model.Albums, model.MediaFiles, error) {
	ctx := r.Context()

	// Get optional library IDs from musicFolderId parameter
	musicFolderIds, err := selectedMusicFolderIds(r, false)
	if err != nil {
		return nil, nil, nil, err
	}

	// Prepare variables to capture results from parallel execution
	var artists model.Artists
	var albums model.Albums
	var mediaFiles model.MediaFiles

	// Execute all three queries in parallel for better performance
	err = run.Parallel(
		// Query starred artists
		func() error {
			artistOpts := filter.ApplyArtistLibraryFilter(filter.ArtistsByStarred(), musicFolderIds)
			var err error
			artists, err = api.ds.Artist(ctx).GetAll(artistOpts)
			if err != nil {
				log.Error(r, "Error retrieving starred artists", err)
			}
			return err
		},
		// Query starred albums
		func() error {
			albumOpts := filter.ApplyLibraryFilter(filter.ByStarred(), musicFolderIds)
			var err error
			albums, err = api.ds.Album(ctx).GetAll(albumOpts)
			if err != nil {
				log.Error(r, "Error retrieving starred albums", err)
			}
			return err
		},
		// Query starred media files
		func() error {
			mediaFileOpts := filter.ApplyLibraryFilter(filter.ByStarred(), musicFolderIds)
			mediaFileOpts.ExcludeHeavyFields = true
			var err error
			mediaFiles, err = api.ds.MediaFile(ctx).GetAll(mediaFileOpts)
			if err != nil {
				log.Error(r, "Error retrieving starred mediaFiles", err)
			}
			return err
		},
	)()

	// Return the first error if any occurred
	if err != nil {
		return nil, nil, nil, err
	}

	return artists, albums, mediaFiles, nil
}

func (api *Router) GetStarred(r *http.Request) (*responses.Subsonic, error) {
	artists, albums, mediaFiles, err := api.getStarredItems(r)
	if err != nil {
		return nil, err
	}

	response := newResponse()
	response.Starred = &responses.Starred{}
	response.Starred.Artist = slice.MapWithArg(artists, r, toArtist)
	response.Starred.Album = slice.MapWithArg(albums, r.Context(), childFromAlbum)
	response.Starred.Song = slice.MapWithArg(mediaFiles, r.Context(), childFromMediaFile)
	return response, nil
}

func (api *Router) GetStarred2(r *http.Request) (*responses.Subsonic, error) {
	artists, albums, mediaFiles, err := api.getStarredItems(r)
	if err != nil {
		return nil, err
	}

	response := newResponse()
	response.Starred2 = &responses.Starred2{}
	response.Starred2.Artist = slice.MapWithArg(artists, r, toArtistID3)
	response.Starred2.Album = slice.MapWithArg(albums, r.Context(), buildAlbumID3)
	response.Starred2.Song = slice.MapWithArg(mediaFiles, r.Context(), childFromMediaFile)
	return response, nil
}

func (api *Router) GetNowPlaying(r *http.Request) (*responses.Subsonic, error) {
	ctx := r.Context()
	npInfo, err := api.scrobbler.GetNowPlaying(ctx)
	if err != nil {
		log.Error(r, "Error retrieving now playing list", err)
		return nil, err
	}

	response := newResponse()
	response.NowPlaying = &responses.NowPlaying{}
	var i int32
	response.NowPlaying.Entry = slice.Map(npInfo, func(np scrobbler.PlaybackSession) responses.NowPlayingEntry {
		i++
		return responses.NowPlayingEntry{
			Child:        childFromMediaFile(ctx, np.MediaFile),
			UserName:     np.Username,
			MinutesAgo:   int32(time.Since(np.Start).Minutes()),
			PlayerId:     i,
			PlayerName:   np.PlayerName,
			State:        np.State,
			PositionMs:   np.PositionMs,
			PlaybackRate: np.PlaybackRate,
		}
	})
	return response, nil
}

func (api *Router) GetRandomSongs(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	size := min(p.IntOr("size", 10), 500)
	genre, _ := p.String("genre")
	fromYear := p.IntOr("fromYear", 0)
	toYear := p.IntOr("toYear", 0)

	// Get optional library IDs from musicFolderId parameter
	musicFolderIds, err := selectedMusicFolderIds(r, false)
	if err != nil {
		return nil, err
	}
	opts := filter.SongsByGenreAndYearRange(genre, fromYear, toYear)
	opts = filter.ApplyLibraryFilter(opts, musicFolderIds)
	opts.Max = size

	songs, err := api.ds.MediaFile(r.Context()).GetRandom(opts)
	if err != nil {
		log.Error(r, "Error retrieving random songs", err)
		return nil, err
	}

	response := newResponse()
	response.RandomSongs = &responses.Songs{}
	response.RandomSongs.Songs = slice.MapWithArg(songs, r.Context(), childFromMediaFile)
	return response, nil
}

func (api *Router) GetSongsByGenre(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	count := min(p.IntOr("count", 10), 500)
	offset := p.IntOr("offset", 0)
	genre, _ := p.String("genre")

	// Get optional library IDs from musicFolderId parameter
	musicFolderIds, err := selectedMusicFolderIds(r, false)
	if err != nil {
		return nil, err
	}
	opts := filter.SongsByGenre(genre)
	opts = filter.ApplyLibraryFilter(opts, musicFolderIds)

	ctx := r.Context()
	songs, err := api.getSongs(ctx, offset, count, opts)
	if err != nil {
		log.Error(r, "Error retrieving random songs", err)
		return nil, err
	}

	response := newResponse()
	response.SongsByGenre = &responses.Songs{}
	response.SongsByGenre.Songs = slice.MapWithArg(songs, ctx, childFromMediaFile)
	return response, nil
}

func (api *Router) getSongs(ctx context.Context, offset, size int, opts filter.Options) (model.MediaFiles, error) {
	opts.Offset = offset
	opts.Max = size
	return api.ds.MediaFile(ctx).GetAll(opts)
}
