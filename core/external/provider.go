package external

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/agents"
	"github.com/navidrome/navidrome/core/eventbus"
	"github.com/navidrome/navidrome/core/lifecycle"
	"github.com/navidrome/navidrome/core/matcher"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/query"
	"github.com/navidrome/navidrome/utils"
	. "github.com/navidrome/navidrome/utils/gg"
	"github.com/navidrome/navidrome/utils/random"
	"github.com/navidrome/navidrome/utils/slice"
	"github.com/navidrome/navidrome/utils/str"
	"golang.org/x/sync/errgroup"
)

const (
	maxSimilarArtists = 100
	maxSeeds          = 5
	// Subsonic passes the client's count through unbounded, and it ends up as a SQL limit.
	maxSimilarSongs    = 500
	refreshDelay       = 5 * time.Second
	refreshTimeout     = 15 * time.Second
	refreshQueueLength = 2000
)

type Provider interface {
	UpdateAlbumInfo(ctx context.Context, id string) (*model.Album, error)
	UpdateArtistInfo(ctx context.Context, id string, count int, includeNotPresent bool) (*model.Artist, error)
	SimilarSongs(ctx context.Context, id string, count int) (model.MediaFiles, error)
	TopSongs(ctx context.Context, artist, artistId string, count int) (model.MediaFiles, error)
	ArtistImage(ctx context.Context, id string) (*url.URL, error)
	AlbumImage(ctx context.Context, id string) (*url.URL, error)
	Close()
}

type similarCacheEntry struct {
	songs model.MediaFiles
	at    time.Time
}

type provider struct {
	ds          model.DataStore
	ag          Agents
	matcher     *matcher.Matcher
	artistQueue refreshQueue[auxArtist]
	albumQueue  refreshQueue[auxAlbum]
	queueCtx    context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup

	similarMu         sync.Mutex
	similarCache      map[string]similarCacheEntry
	similarRefreshing map[string]struct{}
	wg                sync.WaitGroup
}

type auxAlbum struct {
	model.Album
}

// Name returns the appropriate album name for external API calls
// based on the DevPreserveUnicodeInExternalCalls configuration option
func (a *auxAlbum) Name() string {
	if conf.Server.DevPreserveUnicodeInExternalCalls {
		return a.Album.Name
	}
	return str.Clear(a.Album.Name)
}

type auxArtist struct {
	model.Artist
}

// Name returns the appropriate artist name for external API calls
// based on the DevPreserveUnicodeInExternalCalls configuration option
func (a *auxArtist) Name() string {
	if conf.Server.DevPreserveUnicodeInExternalCalls {
		return a.Artist.Name
	}
	return str.Clear(a.Artist.Name)
}

type Agents interface {
	agents.AlbumInfoRetriever
	agents.AlbumImageRetriever
	agents.ArtistBiographyRetriever
	agents.ArtistMBIDRetriever
	agents.ArtistImageRetriever
	agents.ArtistSimilarRetriever
	agents.ArtistTopSongsRetriever
	agents.ArtistURLRetriever
	agents.SimilarSongsByTrackRetriever
	agents.SimilarSongsByAlbumRetriever
	agents.SimilarSongsByArtistRetriever
}

func NewProvider(ds model.DataStore, agents Agents, m *matcher.Matcher) Provider {
	e := &provider{
		ds:                ds,
		ag:                agents,
		matcher:           m,
		similarCache:      make(map[string]similarCacheEntry),
		similarRefreshing: make(map[string]struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.queueCtx = ctx
	e.cancel = cancel
	e.artistQueue = newRefreshQueue(ctx, &e.wg, e.populateArtistInfo)
	e.albumQueue = newRefreshQueue(ctx, &e.wg, e.populateAlbumInfo)
	lifecycle.Register(e)
	return e
}

func (e *provider) Close() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}

func (e *provider) getAlbum(ctx context.Context, id string) (auxAlbum, error) {
	if album, err := e.ds.Album(ctx).Get(id); err == nil {
		return auxAlbum{Album: *album}, nil
	} else if !errors.Is(err, model.ErrNotFound) {
		return auxAlbum{}, err
	}
	mf, err := e.ds.MediaFile(ctx).Get(id)
	if err != nil {
		return auxAlbum{}, err
	}
	if mf.AlbumID == "" || mf.AlbumID == id {
		return auxAlbum{}, model.ErrNotFound
	}
	return e.getAlbum(ctx, mf.AlbumID)
}

func (e *provider) UpdateAlbumInfo(ctx context.Context, id string) (*model.Album, error) {
	album, err := e.getAlbum(ctx, id)
	if err != nil {
		log.Info(ctx, "Not found", "id", id)
		return nil, err
	}

	updatedAt := V(album.ExternalInfoUpdatedAt)
	albumName := album.Name()
	if updatedAt.IsZero() {
		log.Debug(ctx, "AlbumInfo not cached. Retrieving it now", "updatedAt", updatedAt, "id", id, "name", albumName)
		album, err = e.populateAlbumInfo(ctx, album)
		if err != nil {
			return nil, err
		}
		return &album.Album, nil
	}

	// If info is expired, trigger a populateAlbumInfo in the background
	if time.Since(updatedAt) > conf.Server.DevAlbumInfoTimeToLive {
		log.Debug("Found expired cached AlbumInfo, refreshing in the background", "updatedAt", album.ExternalInfoUpdatedAt, "name", albumName)
		e.albumQueue.enqueue(&album)
	}

	return &album.Album, nil
}

func (e *provider) populateAlbumInfo(ctx context.Context, album auxAlbum) (auxAlbum, error) {
	start := time.Now()
	albumName := album.Name()
	info, err := e.ag.GetAlbumInfo(ctx, albumName, album.AlbumArtist, album.MbzAlbumID)
	if err != nil && !errors.Is(err, agents.ErrNotFound) {
		log.Error("Error refreshing AlbumInfo", "id", album.ID, "name", albumName, "artist", album.AlbumArtist,
			"elapsed", time.Since(start), err)
		return album, err
	}
	if err == nil && info != nil {
		album.ExternalUrl = info.URL
		if info.Description != "" {
			album.Description = info.Description
		}
	}

	e.callGetAlbumImages(ctx, e.ag, &album)

	if utils.IsCtxDone(ctx) {
		log.Warn(ctx, "AlbumInfo update canceled", "id", album.ID, "name", albumName, "elapsed", time.Since(start), ctx.Err())
		return album, ctx.Err()
	}

	album.ExternalInfoUpdatedAt = new(time.Now())
	err = e.ds.Album(ctx).UpdateExternalInfo(&album.Album)
	if err != nil {
		log.Error(ctx, "Error trying to update album external information", "id", album.ID, "name", albumName,
			"elapsed", time.Since(start), err)
	} else {
		log.Trace(ctx, "AlbumInfo collected", "album", album, "elapsed", time.Since(start))
		e.publishExternalRefresh(ctx, "album", album.ID)
	}

	return album, nil
}

func (e *provider) getArtist(ctx context.Context, id string) (auxArtist, error) {
	if artist, err := e.ds.Artist(ctx).Get(id); err == nil {
		return auxArtist{Artist: *artist}, nil
	} else if !errors.Is(err, model.ErrNotFound) {
		return auxArtist{}, err
	}
	if mf, err := e.ds.MediaFile(ctx).Get(id); err == nil {
		if mf.ArtistID == "" || mf.ArtistID == id {
			return auxArtist{}, model.ErrNotFound
		}
		return e.getArtist(ctx, mf.ArtistID)
	} else if !errors.Is(err, model.ErrNotFound) {
		return auxArtist{}, err
	}
	album, err := e.ds.Album(ctx).Get(id)
	if err != nil {
		return auxArtist{}, err
	}
	if album.AlbumArtistID == "" || album.AlbumArtistID == id {
		return auxArtist{}, model.ErrNotFound
	}
	return e.getArtist(ctx, album.AlbumArtistID)
}

func (e *provider) UpdateArtistInfo(ctx context.Context, id string, similarCount int, includeNotPresent bool) (*model.Artist, error) {
	artist, err := e.refreshArtistInfo(ctx, id)
	if err != nil {
		return nil, err
	}

	err = e.loadSimilar(ctx, &artist, similarCount, includeNotPresent)
	return &artist.Artist, err
}

func (e *provider) refreshArtistInfo(ctx context.Context, id string) (auxArtist, error) {
	artist, err := e.getArtist(ctx, id)
	if err != nil {
		return auxArtist{}, err
	}

	// If we don't have any info, retrieves it now
	updatedAt := V(artist.ExternalInfoUpdatedAt)
	artistName := artist.Name()
	if updatedAt.IsZero() {
		log.Debug(ctx, "ArtistInfo not cached. Retrieving it now", "updatedAt", updatedAt, "id", id, "name", artistName)
		return e.populateArtistInfo(ctx, artist)
	}

	// If info is expired, trigger a populateArtistInfo in the background
	if time.Since(updatedAt) > conf.Server.DevArtistInfoTimeToLive {
		log.Debug("Found expired cached ArtistInfo, refreshing in the background", "updatedAt", updatedAt, "name", artistName)
		e.artistQueue.enqueue(&artist)
	}
	return artist, nil
}

func (e *provider) populateArtistInfo(ctx context.Context, artist auxArtist) (auxArtist, error) {
	start := time.Now()
	// Get MBID first, if it is not yet available
	artistName := artist.Name()
	if artist.MbzArtistID == "" {
		mbid, err := e.ag.GetArtistMBID(ctx, artist.ID, artistName)
		if mbid != "" && err == nil {
			artist.MbzArtistID = mbid
		}
	}

	// Call all registered agents and collect information
	g := errgroup.Group{}
	g.SetLimit(2)
	g.Go(func() error { e.callGetImage(ctx, e.ag, &artist); return nil })
	g.Go(func() error { e.callGetBiography(ctx, e.ag, &artist); return nil })
	g.Go(func() error { e.callGetURL(ctx, e.ag, &artist); return nil })
	g.Go(func() error { e.callGetSimilarArtists(ctx, e.ag, &artist, maxSimilarArtists, true); return nil })
	_ = g.Wait()

	if utils.IsCtxDone(ctx) {
		log.Warn(ctx, "ArtistInfo update canceled", "id", artist.ID, "name", artistName, "elapsed", time.Since(start), ctx.Err())
		return artist, ctx.Err()
	}

	artist.ExternalInfoUpdatedAt = new(time.Now())
	err := e.ds.Artist(ctx).UpdateExternalInfo(&artist.Artist)
	if err != nil {
		log.Error(ctx, "Error trying to update artist external information", "id", artist.ID, "name", artistName,
			"elapsed", time.Since(start), err)
	} else {
		log.Trace(ctx, "ArtistInfo collected", "artist", artist, "elapsed", time.Since(start))
		e.publishExternalRefresh(ctx, "artist", artist.ID)
	}
	return artist, nil
}

func (e *provider) SimilarSongs(ctx context.Context, id string, count int) (model.MediaFiles, error) {
	if count <= 0 {
		return nil, nil
	}
	count = min(count, maxSimilarSongs)
	entity, err := model.GetEntityByID(ctx, e.ds, id)
	if err != nil {
		if !errors.Is(err, model.ErrNotFound) {
			return nil, err
		}
		genre, gerr := e.ds.Genre(ctx).Get(id)
		if gerr != nil {
			return nil, err
		}
		return e.cachedMix(ctx, "genre:"+id, count, func(runCtx context.Context) (model.MediaFiles, error) {
			return e.seedMix(runCtx, count, func() (model.MediaFiles, error) {
				return e.sampleGenreTracks(runCtx, genre, maxSeeds)
			})
		})
	}

	switch v := entity.(type) {
	case *model.MediaFile:
		return e.cachedMix(ctx, "mf:"+v.ID, count, func(runCtx context.Context) (model.MediaFiles, error) {
			return e.mixFromAgent(runCtx, count,
				func() ([]agents.Song, error) {
					return e.ag.GetSimilarSongsByTrack(runCtx, v.ID, v.Title, v.Artist, v.MbzRecordingID, count)
				},
				func() (model.MediaFiles, error) {
					if v.ArtistID != "" {
						return e.similarSongsFallback(runCtx, v.ArtistID, count)
					}
					return e.similarSongsFallback(runCtx, id, count)
				})
		})
	case *model.Album:
		return e.cachedMix(ctx, "al:"+v.ID, count, func(runCtx context.Context) (model.MediaFiles, error) {
			return e.mixFromAgent(runCtx, count,
				func() ([]agents.Song, error) {
					return e.ag.GetSimilarSongsByAlbum(runCtx, v.ID, v.Name, v.AlbumArtist, v.MbzAlbumID, count)
				},
				func() (model.MediaFiles, error) {
					if v.AlbumArtistID != "" {
						if res, ferr := e.similarSongsFallback(runCtx, v.AlbumArtistID, count); ferr == nil && len(res) > 0 {
							return res, nil
						}
					}
					return e.seedMix(runCtx, count, func() (model.MediaFiles, error) {
						return e.sampleAlbumTracks(runCtx, v.ID, maxSeeds)
					})
				})
		})
	case *model.Artist:
		return e.cachedMix(ctx, "ar:"+v.ID, count, func(runCtx context.Context) (model.MediaFiles, error) {
			return e.mixFromAgent(runCtx, count,
				func() ([]agents.Song, error) {
					return e.ag.GetSimilarSongsByArtist(runCtx, v.ID, v.Name, v.MbzArtistID, count)
				},
				func() (model.MediaFiles, error) {
					if res, ferr := e.similarSongsFallback(runCtx, v.ID, count); ferr == nil && len(res) > 0 {
						return res, nil
					}
					return e.seedMix(runCtx, count, func() (model.MediaFiles, error) {
						return e.sampleArtistTracks(runCtx, v.ID, maxSeeds)
					})
				})
		})
	case *model.Playlist:
		return e.cachedMix(ctx, "pl:"+v.ID, count, func(runCtx context.Context) (model.MediaFiles, error) {
			return e.seedMix(runCtx, count, func() (model.MediaFiles, error) {
				return e.samplePlaylistTracks(runCtx, v.ID, maxSeeds)
			})
		})
	default:
		log.Warn(ctx, "Unknown entity type", "id", id, "type", fmt.Sprintf("%T", entity))
		return nil, model.ErrNotFound
	}
}

func (e *provider) cachedMix(ctx context.Context, key string, count int, run func(context.Context) (model.MediaFiles, error)) (model.MediaFiles, error) {
	if songs, fresh := e.lookupSimilarCache(key, count); songs != nil {
		if !fresh {
			e.refreshMixAsync(key, count, run)
		}
		return songs, nil
	}
	songs, err := run(ctx)
	if err != nil {
		return nil, err
	}
	if len(songs) > 0 {
		e.storeSimilarCache(key, songs)
	}
	return songs, nil
}

func (e *provider) lookupSimilarCache(key string, count int) (model.MediaFiles, bool) {
	e.similarMu.Lock()
	defer e.similarMu.Unlock()
	entry, ok := e.similarCache[key]
	if !ok || len(entry.songs) == 0 {
		return nil, false
	}
	if count > len(entry.songs) {
		return nil, false
	}
	fresh := time.Since(entry.at) <= conf.Server.DevArtistInfoTimeToLive
	out := make(model.MediaFiles, count)
	copy(out, entry.songs[:count])
	return out, fresh
}

func (e *provider) storeSimilarCache(key string, songs model.MediaFiles) {
	stored := make(model.MediaFiles, len(songs))
	copy(stored, songs)
	e.similarMu.Lock()
	e.similarCache[key] = similarCacheEntry{songs: stored, at: time.Now()}
	e.similarMu.Unlock()
}

func (e *provider) refreshMixAsync(key string, count int, run func(context.Context) (model.MediaFiles, error)) {
	e.similarMu.Lock()
	if _, busy := e.similarRefreshing[key]; busy {
		e.similarMu.Unlock()
		return
	}
	e.similarRefreshing[key] = struct{}{}
	e.similarMu.Unlock()
	go func() {
		defer func() {
			e.similarMu.Lock()
			delete(e.similarRefreshing, key)
			e.similarMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(e.queueCtx, refreshTimeout)
		defer cancel()
		songs, err := run(ctx)
		if err != nil || len(songs) == 0 {
			return
		}
		e.storeSimilarCache(key, songs)
	}()
}

func (e *provider) mixFromAgent(ctx context.Context, count int, fetch func() ([]agents.Song, error), fallback func() (model.MediaFiles, error)) (model.MediaFiles, error) {
	songs, err := fetch()
	if err == nil {
		matched, merr := e.matcher.MatchSongs(ctx, songs, count)
		if merr != nil {
			return nil, merr
		}
		if len(matched) > 0 {
			return matched, nil
		}
	}
	return fallback()
}

func (e *provider) seedMix(ctx context.Context, count int, sample func() (model.MediaFiles, error)) (model.MediaFiles, error) {
	seeds, err := sample()
	if err != nil {
		return nil, err
	}
	if len(seeds) == 0 {
		return nil, nil
	}
	seeds = seeds[:min(len(seeds), maxSeeds)]

	perSeed := make([][]agents.Song, len(seeds))
	var g errgroup.Group
	for i, seed := range seeds {
		g.Go(func() error {
			if s, serr := e.ag.GetSimilarSongsByTrack(ctx, seed.ID, seed.Title, seed.Artist, seed.MbzRecordingID, count); serr == nil {
				perSeed[i] = s
			}
			return nil
		})
	}
	_ = g.Wait()

	var songs []agents.Song
	for _, s := range perSeed {
		songs = append(songs, s...)
	}
	matched, err := e.matcher.MatchSongs(ctx, songs, len(songs))
	if err != nil {
		return nil, err
	}
	matched = dedupByID(matched)
	if len(matched) == 0 {
		matched = seeds
	}
	rand.Shuffle(len(matched), func(i, j int) { matched[i], matched[j] = matched[j], matched[i] }) //nolint:gosec // Cryptographic randomness is not needed for recommendation ordering.
	if len(matched) > count {
		matched = matched[:count]
	}
	return matched, nil
}

func (e *provider) samplePlaylistTracks(ctx context.Context, playlistID string, n int) (model.MediaFiles, error) {
	repo := e.ds.Playlist(ctx).Tracks(playlistID, true)
	if repo == nil {
		return nil, model.ErrNotFound
	}
	tracks, err := repo.GetAll(model.QueryOptions{
		Sort:    "random",
		Max:     n * 4,
		Filters: query.NotMissing(),
	})
	if err != nil {
		return nil, err
	}
	mfs := dedupByID(tracks.MediaFiles())
	return mfs[:min(len(mfs), n)], nil
}

func dedupByID(mfs model.MediaFiles) model.MediaFiles {
	seen := make(map[string]struct{}, len(mfs))
	return slice.Filter(mfs, func(mf model.MediaFile) bool {
		if _, dup := seen[mf.ID]; dup {
			return false
		}
		seen[mf.ID] = struct{}{}
		return true
	})
}

func (e *provider) sampleAlbumTracks(ctx context.Context, albumID string, n int) (model.MediaFiles, error) {
	return e.sampleTracks(ctx, query.Eq("album_id", albumID), n)
}

func (e *provider) sampleArtistTracks(ctx context.Context, artistID string, n int) (model.MediaFiles, error) {
	filter := query.ParticipantIDFilter("media_file", artistID, model.RoleArtist, model.RoleAlbumArtist)
	return e.sampleTracks(ctx, filter, n)
}

func (e *provider) sampleGenreTracks(ctx context.Context, genre *model.Genre, n int) (model.MediaFiles, error) {
	return e.sampleTracks(ctx, query.SongGenres.ByID(genre.ID), n)
}

func (e *provider) sampleTracks(ctx context.Context, filter query.Sqlizer, n int) (model.MediaFiles, error) {
	return e.ds.MediaFile(ctx).GetRandom(model.QueryOptions{
		Filters: query.And(filter, query.NotMissing()),
		Max:     n,
	})
}

// similarSongsFallback uses the original similar artists + top songs algorithm. The idea is to
// get the artist of the given entity, retrieve similar artists, get their top songs, and pick
// a weighted random selection of songs to return as similar songs.
func (e *provider) similarSongsFallback(ctx context.Context, id string, count int) (model.MediaFiles, error) {
	artist, err := e.getArtist(ctx, id)
	if err != nil {
		return nil, err
	}

	updatedAt := V(artist.ExternalInfoUpdatedAt)
	haveCache := len(artist.SimilarArtists) > 0 || !updatedAt.IsZero()
	if haveCache {
		if !updatedAt.IsZero() && time.Since(updatedAt) > conf.Server.DevArtistInfoTimeToLive {
			e.artistQueue.enqueue(&artist)
		}
	} else {
		e.callGetSimilarArtists(ctx, e.ag, &artist, 15, false)
		if utils.IsCtxDone(ctx) {
			log.Warn(ctx, "SimilarSongs call canceled", ctx.Err())
			return nil, ctx.Err()
		}
	}

	weightedSongs := random.NewWeightedChooser[model.MediaFile]()
	addArtist := func(a model.Artist, weightedSongs *random.WeightedChooser[model.MediaFile], count, artistWeight int) error {
		if utils.IsCtxDone(ctx) {
			log.Warn(ctx, "SimilarSongs call canceled", ctx.Err())
			return ctx.Err()
		}

		topCount := max(count, 20)
		topSongs, err := e.getMatchingTopSongs(ctx, e.ag, &auxArtist{Artist: a}, topCount)
		if err != nil {
			log.Warn(ctx, "Error getting artist's top songs", "artist", a.Name, err)
			return nil
		}

		weight := topCount * (4 + artistWeight)
		for _, mf := range topSongs {
			weightedSongs.Add(mf, weight)
			weight -= 4
		}
		return nil
	}

	err = addArtist(artist.Artist, weightedSongs, count, 10)
	if err != nil {
		return nil, err
	}
	for _, a := range artist.SimilarArtists {
		err := addArtist(a, weightedSongs, count, 0)
		if err != nil {
			return nil, err
		}
	}

	var similarSongs model.MediaFiles
	for len(similarSongs) < count && weightedSongs.Size() > 0 {
		s, err := weightedSongs.Pick()
		if err != nil {
			log.Warn(ctx, "Error getting weighted song", err)
			continue
		}
		similarSongs = append(similarSongs, s)
	}

	return similarSongs, nil
}

func (e *provider) ArtistImage(ctx context.Context, id string) (*url.URL, error) {
	artist, err := e.getArtist(ctx, id)
	if err != nil {
		return nil, err
	}
	if utils.IsCtxDone(ctx) {
		log.Warn(ctx, "ArtistImage call canceled", ctx.Err())
		return nil, ctx.Err()
	}

	imageUrl := artist.ArtistImageUrl()
	updatedAt := V(artist.ExternalInfoUpdatedAt)
	if imageUrl == "" && updatedAt.IsZero() {
		log.Debug(ctx, "Artist image not cached, enqueuing background fetch", "artist", artist.Name(), "id", artist.ID)
		e.artistQueue.enqueue(&artist)
		return nil, model.ErrNotFound
	}
	if !updatedAt.IsZero() && time.Since(updatedAt) > conf.Server.DevArtistInfoTimeToLive {
		log.Debug(ctx, "Artist image info expired, enqueuing background refresh", "artist", artist.Name(), "updatedAt", updatedAt)
		e.artistQueue.enqueue(&artist)
	}
	if imageUrl == "" {
		return nil, model.ErrNotFound
	}
	return url.Parse(imageUrl)
}

func (e *provider) AlbumImage(ctx context.Context, id string) (*url.URL, error) {
	album, err := e.getAlbum(ctx, id)
	if err != nil {
		return nil, err
	}
	if utils.IsCtxDone(ctx) {
		log.Debug(ctx, "AlbumImage call canceled", ctx.Err())
		return nil, ctx.Err()
	}

	imageUrl := album.AlbumImageUrl()
	updatedAt := V(album.ExternalInfoUpdatedAt)
	if imageUrl == "" && updatedAt.IsZero() {
		log.Debug(ctx, "Album image not cached, enqueuing background fetch", "album", album.Name(), "id", album.ID)
		e.albumQueue.enqueue(&album)
		return nil, model.ErrNotFound
	}
	if !updatedAt.IsZero() && time.Since(updatedAt) > conf.Server.DevAlbumInfoTimeToLive {
		log.Debug(ctx, "Album image info expired, enqueuing background refresh", "album", album.Name(), "updatedAt", updatedAt)
		e.albumQueue.enqueue(&album)
	}
	if imageUrl == "" {
		return nil, model.ErrNotFound
	}
	return url.Parse(imageUrl)
}

func (e *provider) TopSongs(ctx context.Context, artistName, id string, count int) (model.MediaFiles, error) {
	artist, err := e.findArtist(ctx, artistName, id)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			log.Error(ctx, "Artist not found", "name", artistName, "id", id, err)
			return nil, nil
		}

		log.Error(ctx, "Failure occurred when trying to fetch artist", "name", artistName, "id", id, err)
		return nil, err
	}

	songs, err := e.cachedMix(ctx, "top:"+artist.ID, count, func(runCtx context.Context) (model.MediaFiles, error) {
		return e.getMatchingTopSongs(runCtx, e.ag, artist, count)
	})
	if err != nil {
		switch {
		case errors.Is(err, agents.ErrNotFound):
			log.Trace(ctx, "TopSongs not found", "name", artistName)
			return nil, model.ErrNotFound
		case errors.Is(err, context.Canceled):
			log.Debug(ctx, "TopSongs call canceled", err)
		default:
			log.Warn(ctx, "Error getting top songs from agent", "artist", artistName, err)
		}
		return nil, err
	}
	return songs, nil
}

func (e *provider) getMatchingTopSongs(ctx context.Context, agent agents.ArtistTopSongsRetriever, artist *auxArtist, count int) (model.MediaFiles, error) {
	artistName := artist.Name()
	songs, err := agent.GetArtistTopSongs(ctx, artist.ID, artistName, artist.MbzArtistID, count)
	if err != nil {
		return nil, fmt.Errorf("failed to get top songs for artist %s: %w", artistName, err)
	}

	// Enrich top songs with the queried artist. A song with no artists, or whose first credit the
	// agent left unnamed, is attributed to the queried artist. A first credit that already names an
	// artist is left as-is: it may be a different (e.g. featured) artist, so stamping the queried
	// MBID onto it would create a false name+MBID pairing.
	for i := range songs {
		switch {
		case len(songs[i].Artists) == 0:
			songs[i].Artists = []agents.Artist{{Name: artistName, MBID: artist.MbzArtistID}}
		case songs[i].Artists[0].Name == "":
			songs[i].Artists[0].Name = artistName
			if songs[i].Artists[0].MBID == "" {
				songs[i].Artists[0].MBID = artist.MbzArtistID
			}
		}
	}

	mfs, err := e.matcher.MatchSongs(ctx, songs, count)
	if err != nil {
		return nil, err
	}

	if len(mfs) == 0 {
		log.Debug(ctx, "No matching top songs found", "name", artistName)
	} else {
		log.Debug(ctx, "Found matching top songs", "name", artistName, "numSongs", len(mfs))
	}

	return mfs, nil
}

func (e *provider) callGetURL(ctx context.Context, agent agents.ArtistURLRetriever, artist *auxArtist) {
	artisURL, err := agent.GetArtistURL(ctx, artist.ID, artist.Name(), artist.MbzArtistID)
	if err != nil {
		return
	}
	artist.ExternalUrl = artisURL
}

func (e *provider) callGetBiography(ctx context.Context, agent agents.ArtistBiographyRetriever, artist *auxArtist) {
	bio, err := agent.GetArtistBiography(ctx, artist.ID, artist.Name(), artist.MbzArtistID)
	if err != nil {
		return
	}
	bio = str.SanitizeText(bio)
	bio = strings.ReplaceAll(bio, "\n", " ")
	artist.Biography = strings.ReplaceAll(bio, "<a ", "<a target='_blank' ")
}

func (e *provider) publishExternalRefresh(ctx context.Context, resource, id string) {
	if id == "" {
		return
	}
	refresh := (&eventbus.RefreshResource{}).Add(resource, id)
	eventbus.Get().PublishUISync(ctx, eventbus.Event{
		Topic:   eventbus.TopicRefreshResource,
		Refresh: refresh,
	}, true)
}

func (e *provider) callGetAlbumImages(ctx context.Context, agent agents.AlbumImageRetriever, album *auxAlbum) {
	images, err := agent.GetAlbumImages(ctx, album.Name(), album.AlbumArtist, album.MbzAlbumID)
	if err != nil || len(images) == 0 {
		return
	}
	slices.SortFunc(images, func(a, b agents.ExternalImage) int { return cmp.Compare(b.Size, a.Size) })
	album.LargeImageUrl = images[0].URL
	if len(images) >= 2 {
		album.MediumImageUrl = images[1].URL
	}
	if len(images) >= 3 {
		album.SmallImageUrl = images[2].URL
	}
}

func (e *provider) callGetImage(ctx context.Context, agent agents.ArtistImageRetriever, artist *auxArtist) {
	images, err := agent.GetArtistImages(ctx, artist.ID, artist.Name(), artist.MbzArtistID)
	if err != nil {
		return
	}
	slices.SortFunc(images, func(a, b agents.ExternalImage) int { return cmp.Compare(b.Size, a.Size) })

	if len(images) >= 1 {
		artist.LargeImageUrl = images[0].URL
	}
	if len(images) >= 2 {
		artist.MediumImageUrl = images[1].URL
	}
	if len(images) >= 3 {
		artist.SmallImageUrl = images[2].URL
	}
}

func (e *provider) callGetSimilarArtists(ctx context.Context, agent agents.ArtistSimilarRetriever, artist *auxArtist,
	limit int, includeNotPresent bool) {
	artistName := artist.Name()
	similar, err := agent.GetSimilarArtists(ctx, artist.ID, artistName, artist.MbzArtistID, limit)
	if len(similar) == 0 || err != nil {
		return
	}
	start := time.Now()
	sa, err := e.mapSimilarArtists(ctx, similar, limit, includeNotPresent)
	log.Debug(ctx, "Mapped Similar Artists", "artist", artistName, "numSimilar", len(sa), "elapsed", time.Since(start))
	if err != nil {
		return
	}
	artist.SimilarArtists = sa
}

func (e *provider) mapSimilarArtists(ctx context.Context, similar []agents.Artist, limit int, includeNotPresent bool) (model.Artists, error) {
	var result model.Artists
	var notPresent []string

	// Load artists by ID (highest priority)
	idMatches, err := e.loadArtistsByID(ctx, similar)
	if err != nil {
		return nil, err
	}

	// Load artists by MBID (second priority)
	mbidMatches, err := e.loadArtistsByMBID(ctx, similar, idMatches)
	if err != nil {
		return nil, err
	}

	// Load artists by name (lowest priority, fallback)
	nameMatches, err := e.loadArtistsByName(ctx, similar, idMatches, mbidMatches)
	if err != nil {
		return nil, err
	}

	count := 0

	// Process the similar artists using priority: ID → MBID → Name
	for _, s := range similar {
		if count >= limit {
			break
		}
		// Try ID match first
		if s.ID != "" {
			if artist, found := idMatches[s.ID]; found {
				result = append(result, artist)
				count++
				continue
			}
		}
		// Try MBID match second
		if s.MBID != "" {
			if artist, found := mbidMatches[s.MBID]; found {
				result = append(result, artist)
				count++
				continue
			}
		}
		// Fall back to name match
		if artist, found := nameMatches[s.Name]; found {
			result = append(result, artist)
			count++
		} else {
			notPresent = append(notPresent, s.Name)
		}
	}

	// Then fill up with non-present artists
	if includeNotPresent && count < limit {
		for _, s := range notPresent {
			// Let the ID empty to indicate that the artist is not present in the DB
			sa := model.Artist{Name: s}
			result = append(result, sa)

			count++
			if count >= limit {
				break
			}
		}
	}

	return result, nil
}

func (e *provider) loadArtistsByID(ctx context.Context, similar []agents.Artist) (map[string]model.Artist, error) {
	var ids []string
	for _, s := range similar {
		if s.ID != "" {
			ids = append(ids, s.ID)
		}
	}
	matches := map[string]model.Artist{}
	if len(ids) == 0 {
		return matches, nil
	}
	res, err := e.ds.Artist(ctx).GetAll(model.QueryOptions{
		Filters: query.Eq("artist.id", ids),
	})
	if err != nil {
		return matches, err
	}
	for _, a := range res {
		if _, ok := matches[a.ID]; !ok {
			matches[a.ID] = a
		}
	}
	return matches, nil
}

func (e *provider) loadArtistsByMBID(ctx context.Context, similar []agents.Artist, idMatches map[string]model.Artist) (map[string]model.Artist, error) {
	var mbids []string
	for _, s := range similar {
		// Skip if already matched by ID
		if s.ID != "" && idMatches[s.ID].ID != "" {
			continue
		}
		if s.MBID != "" {
			mbids = append(mbids, s.MBID)
		}
	}
	matches := map[string]model.Artist{}
	if len(mbids) == 0 {
		return matches, nil
	}
	res, err := e.ds.Artist(ctx).GetAll(model.QueryOptions{
		Filters: query.Eq("mbz_artist_id", mbids),
	})
	if err != nil {
		return matches, err
	}
	for _, a := range res {
		if id := a.MbzArtistID; id != "" {
			if _, ok := matches[id]; !ok {
				matches[id] = a
			}
		}
	}
	return matches, nil
}

func (e *provider) loadArtistsByName(ctx context.Context, similar []agents.Artist, idMatches map[string]model.Artist, mbidMatches map[string]model.Artist) (map[string]model.Artist, error) {
	var names []string
	for _, s := range similar {
		// Skip if already matched by ID or MBID
		if s.ID != "" && idMatches[s.ID].ID != "" {
			continue
		}
		if s.MBID != "" && mbidMatches[s.MBID].ID != "" {
			continue
		}
		names = append(names, s.Name)
	}
	matches := map[string]model.Artist{}
	if len(names) == 0 {
		return matches, nil
	}
	clauses := slice.Map(names, func(name string) query.Sqlizer {
		return query.Like("artist.name", name)
	})
	res, err := e.ds.Artist(ctx).GetAll(model.QueryOptions{
		Filters: query.Or(clauses...),
	})
	if err != nil {
		return matches, err
	}
	for _, a := range res {
		if _, ok := matches[a.Name]; !ok {
			matches[a.Name] = a
		}
	}
	return matches, nil
}

func (e *provider) findArtist(ctx context.Context, artistName, id string) (*auxArtist, error) {
	if id != "" {
		artist, err := e.ds.Artist(ctx).Get(id)
		if err == nil {
			return &auxArtist{Artist: *artist}, nil
		}

		if errors.Is(err, model.ErrNotFound) {
			log.Warn(ctx, "Could not find artist by id", "id", id, err)
		} else {
			return nil, err
		}
	}

	if artistName == "" {
		return nil, model.ErrNotFound
	}

	artists, err := e.ds.Artist(ctx).GetAll(model.QueryOptions{
		Filters: query.Like("artist.name", artistName),
		Max:     1,
	})
	if err != nil {
		return nil, err
	}
	if len(artists) == 0 {
		return nil, model.ErrNotFound
	}
	return &auxArtist{Artist: artists[0]}, nil
}

func (e *provider) loadSimilar(ctx context.Context, artist *auxArtist, count int, includeNotPresent bool) error {
	var ids []string
	for _, sa := range artist.SimilarArtists {
		if sa.ID == "" {
			continue
		}
		ids = append(ids, sa.ID)
	}

	similar, err := e.ds.Artist(ctx).GetAll(model.QueryOptions{
		Filters: query.Eq("artist.id", ids),
	})
	if err != nil {
		log.Error("Error loading similar artists", "id", artist.ID, "name", artist.Name(), err)
		return err
	}

	// Use a map and iterate through original array, to keep the same order
	artistMap := make(map[string]model.Artist)
	for _, sa := range similar {
		artistMap[sa.ID] = sa
	}

	var loaded model.Artists
	for _, sa := range artist.SimilarArtists {
		if len(loaded) >= count {
			break
		}
		la, ok := artistMap[sa.ID]
		if !ok {
			if !includeNotPresent {
				continue
			}
			la = sa
			la.ID = ""
		}
		loaded = append(loaded, la)
	}
	artist.SimilarArtists = loaded
	return nil
}

type refreshQueue[T any] chan<- *T

func newRefreshQueue[T any](ctx context.Context, wg *sync.WaitGroup, processFn func(context.Context, T) (T, error)) refreshQueue[T] {
	queue := make(chan *T, refreshQueueLength)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case item := <-queue:
				runCtx, cancel := context.WithTimeout(ctx, refreshTimeout)
				_, _ = processFn(runCtx, *item)
				cancel()
				select {
				case <-ctx.Done():
					return
				case <-time.After(refreshDelay):
				}
			}
		}
	}()
	return queue
}

func (q *refreshQueue[T]) enqueue(item *T) {
	select {
	case *q <- item:
	default: // It is ok to miss a refresh request
	}
}
