package subsonic

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
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
	genreResponseCacheTTL   = 45 * time.Second
	genreResponseCacheLimit = 128
)

type genreResponseCache struct {
	catalogCache[*responses.Genres]
}

func (c *genreResponseCache) put(key string, now time.Time, value *responses.Genres) {
	c.store(key, now, value, genreResponseCacheLimit, genreResponseCacheTTL)
}

type musicFoldersResponseCache struct {
	catalogCache[*responses.MusicFolders]
}

func (c *musicFoldersResponseCache) put(key string, now time.Time, value *responses.MusicFolders) {
	copied := *value
	copied.Folders = slices.Clone(value.Folders)
	c.store(key, now, &copied, genreResponseCacheLimit, genreResponseCacheTTL)
}

type artistIndexSnapshot struct {
	indexes  model.ArtistIndexes
	modified int64
}
type artistIndexCache struct {
	catalogCache[artistIndexSnapshot]
}

func artistIndexCacheKey(libIds []int, lastScan string) string {
	ids := append([]int(nil), libIds...)
	slices.Sort(ids)
	var key strings.Builder
	key.Grow(len(ids)*6 + len(lastScan) + len(conf.Server.IndexGroups) + 8)
	for _, id := range ids {
		key.WriteString(strconv.Itoa(id))
		key.WriteByte(',')
	}
	key.WriteString(lastScan)
	key.WriteByte('|')
	key.WriteString(conf.Server.IndexGroups)
	if conf.Server.Subsonic.ArtistParticipations {
		key.WriteString("|part")
	}
	if conf.Server.PreferSortTags {
		key.WriteString("|sort")
	}
	return key.String()
}

func genreResponseCacheKey(user model.User) string {
	if user.IsAdmin {
		return "admin"
	}
	ids := make([]int, len(user.Libraries))
	for i, library := range user.Libraries {
		ids[i] = library.ID
	}
	slices.Sort(ids)
	var key strings.Builder
	key.Grow(len(ids) * 4)
	for _, id := range ids {
		key.WriteString(strconv.Itoa(id))
		key.WriteByte(',')
	}
	return key.String()
}

func (api *Router) GetMusicFolders(r *http.Request) (*responses.Subsonic, error) {
	user, _ := request.UserFrom(r.Context())
	cacheKey := genreResponseCacheKey(user)
	now := time.Now()
	if cached, ok := api.musicFoldersCache.get(cacheKey, now); ok {
		response := newResponse()
		response.MusicFolders = cached
		return response, nil
	}

	libraries := getUserAccessibleLibraries(r.Context())

	folders := make([]responses.MusicFolder, len(libraries))
	for i, f := range libraries {
		folders[i].Id = int32(f.ID)
		folders[i].Name = f.Name
	}
	response := newResponse()
	response.MusicFolders = &responses.MusicFolders{Folders: folders}
	api.musicFoldersCache.put(cacheKey, now, response.MusicFolders)
	return response, nil
}

func (api *Router) getArtist(r *http.Request, libIds []int, ifModifiedSince time.Time) (model.ArtistIndexes, int64, error) {
	ctx := r.Context()

	lastScanStr, err := api.ds.Property(ctx).DefaultGet(consts.LastScanStartTimeKey, "")
	if err != nil {
		log.Error(ctx, "Error retrieving last scan start time", err)
		return nil, 0, err
	}
	lastScan := time.Now()
	if lastScanStr != "" {
		lastScan, err = time.Parse(time.RFC3339, lastScanStr)
	}

	var indexes model.ArtistIndexes
	if lastScan.After(ifModifiedSince) {
		cacheKey := catalogUserKey(r.Context()) + "|" + artistIndexCacheKey(libIds, lastScanStr)
		snapshot, loadErr := api.artistIndexCache.load(ctx, cacheKey, genreResponseCacheLimit, genreResponseCacheTTL, func(ctx context.Context) (artistIndexSnapshot, error) {
			indexes, err := api.ds.Artist(ctx).GetIndex(false, libIds, model.RoleAlbumArtist)
			if err != nil {
				return artistIndexSnapshot{}, err
			}
			if len(indexes) == 0 {
				return artistIndexSnapshot{}, newError(responses.ErrorDataNotFound, "Library not found or empty")
			}
			return artistIndexSnapshot{indexes: indexes, modified: lastScan.UnixMilli()}, nil
		})
		if loadErr != nil {
			return nil, 0, loadErr
		}
		return snapshot.indexes, snapshot.modified, nil
	}

	return indexes, lastScan.UnixMilli(), err
}

func (api *Router) getArtistIndex(r *http.Request, libIds []int, ifModifiedSince time.Time) (*responses.Indexes, error) {
	indexes, modified, err := api.getArtist(r, libIds, ifModifiedSince)
	if err != nil {
		return nil, err
	}

	res := &responses.Indexes{
		IgnoredArticles: conf.Server.IgnoredArticles,
		LastModified:    modified,
	}

	res.Index = make([]responses.Index, len(indexes))
	for i, idx := range indexes {
		res.Index[i].Name = idx.ID
		res.Index[i].Artists = slice.MapWithArg(idx.Artists, r, toArtist)
	}
	return res, nil
}

func (api *Router) getArtistIndexID3(r *http.Request, libIds []int, ifModifiedSince time.Time) (*responses.Artists, error) {
	indexes, modified, err := api.getArtist(r, libIds, ifModifiedSince)
	if err != nil {
		return nil, err
	}

	res := &responses.Artists{
		IgnoredArticles: conf.Server.IgnoredArticles,
		LastModified:    modified,
	}

	res.Index = make([]responses.IndexID3, len(indexes))
	for i, idx := range indexes {
		res.Index[i].Name = idx.ID
		res.Index[i].Artists = slice.MapWithArg(idx.Artists, r, toArtistID3)
	}
	return res, nil
}

func (api *Router) GetIndexes(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	musicFolderIds, _ := selectedMusicFolderIds(r, false)
	ifModifiedSince := p.TimeOr("ifModifiedSince", time.Time{})

	res, err := api.getArtistIndex(r, musicFolderIds, ifModifiedSince)
	if err != nil {
		return nil, err
	}

	response := newResponse()
	response.Indexes = res
	return response, nil
}

func (api *Router) GetArtists(r *http.Request) (*responses.Subsonic, error) {
	musicFolderIds, _ := selectedMusicFolderIds(r, false)
	p := req.Params(r)
	ifModifiedSince := p.TimeOr("ifModifiedSince", time.Time{})

	res, err := api.getArtistIndexID3(r, musicFolderIds, ifModifiedSince)
	if err != nil {
		return nil, err
	}

	response := newResponse()
	response.Artist = res
	return response, nil
}

func (api *Router) GetMusicDirectory(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	id, _ := p.String("id")
	ctx := r.Context()

	entity, err := model.GetEntityByID(ctx, api.ds, id)
	if errors.Is(err, model.ErrNotFound) {
		log.Error(r, "Requested ID not found ", "id", id)
		return nil, newError(responses.ErrorDataNotFound, "Directory not found")
	}
	if err != nil {
		log.Error(err)
		return nil, err
	}

	var dir *responses.Directory

	switch v := entity.(type) {
	case *model.Artist:
		dir, err = api.buildArtistDirectory(ctx, v)
	case *model.Album:
		dir, err = api.buildAlbumDirectory(ctx, v)
	default:
		log.Error(r, "Requested ID of invalid type", "id", id, "entity", v)
		return nil, newError(responses.ErrorDataNotFound, "Directory not found")
	}

	if err != nil {
		log.Error(err)
		return nil, err
	}

	response := newResponse()
	response.Directory = dir
	return response, nil
}

func (api *Router) GetArtist(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	id, _ := p.String("id")
	return api.cachedSubsonicResponse(r, entityResponseCacheKey(r, "artist", id), func(r *http.Request) (*responses.Subsonic, error) {
		return api.loadArtist(r, id)
	})
}

func (api *Router) loadArtist(r *http.Request, id string) (*responses.Subsonic, error) {
	ctx := r.Context()

	artist, err := api.ds.Artist(ctx).Get(id)
	if errors.Is(err, model.ErrNotFound) {
		log.Error(ctx, "Requested ArtistID not found ", "id", id)
		return nil, newError(responses.ErrorDataNotFound, "Artist not found")
	}
	if err != nil {
		log.Error(ctx, "Error retrieving artist", "id", id, err)
		return nil, err
	}

	response := newResponse()
	response.ArtistWithAlbumsID3, err = api.buildArtist(r, artist)
	if err != nil {
		log.Error(ctx, "Error retrieving albums by artist", "id", artist.ID, "name", artist.Name, err)
	}
	return response, err
}

func (api *Router) GetAlbum(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	id, _ := p.String("id")
	return api.cachedSubsonicResponse(r, entityResponseCacheKey(r, "album", id), func(r *http.Request) (*responses.Subsonic, error) {
		return api.loadAlbum(r, id)
	})
}

func (api *Router) loadAlbum(r *http.Request, id string) (*responses.Subsonic, error) {
	ctx := r.Context()

	var album *model.Album
	var mfs model.MediaFiles
	err := run.Parallel(
		func() error {
			var err error
			album, err = api.ds.Album(ctx).Get(id)
			if errors.Is(err, model.ErrNotFound) {
				log.Error(ctx, "Requested AlbumID not found ", "id", id)
				return newError(responses.ErrorDataNotFound, "Album not found")
			}
			if err != nil {
				log.Error(ctx, "Error retrieving album", "id", id, err)
			}
			return err
		},
		func() error {
			var err error
			mfs, err = api.ds.MediaFile(ctx).GetAll(filter.SongsByAlbum(id))
			if err != nil {
				log.Error(ctx, "Error retrieving tracks from album", "id", id, err)
			}
			return err
		},
	)()
	if err != nil {
		return nil, err
	}

	response := newResponse()
	response.AlbumWithSongsID3 = api.buildAlbum(ctx, album, mfs)
	return response, nil
}

func (api *Router) GetAlbumInfo(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	id, err := p.String("id")
	if err != nil {
		return nil, err
	}
	return api.cachedSubsonicResponse(r, entityResponseCacheKey(r, "albumInfo", id), func(r *http.Request) (*responses.Subsonic, error) {
		return api.loadAlbumInfo(r, id)
	})
}

func (api *Router) loadAlbumInfo(r *http.Request, id string) (*responses.Subsonic, error) {
	ctx := r.Context()

	album, err := api.provider.UpdateAlbumInfo(ctx, id)
	if err != nil {
		return nil, err
	}

	response := newResponse()
	response.AlbumInfo = &responses.AlbumInfo{}
	response.AlbumInfo.Notes = album.Description
	small, medium, large := entityImageURLs(r, album.SmallImageUrl, album.MediumImageUrl, album.LargeImageUrl, album.CoverArtID())
	response.AlbumInfo.SmallImageUrl = small
	response.AlbumInfo.MediumImageUrl = medium
	response.AlbumInfo.LargeImageUrl = large

	response.AlbumInfo.LastFmUrl = album.ExternalUrl
	response.AlbumInfo.MusicBrainzID = album.MbzAlbumID

	return response, nil
}

func (api *Router) GetSong(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	id, _ := p.String("id")
	return api.cachedSubsonicResponse(r, entityResponseCacheKey(r, "song", id), func(r *http.Request) (*responses.Subsonic, error) {
		return api.loadSong(r, id)
	})
}

func (api *Router) loadSong(r *http.Request, id string) (*responses.Subsonic, error) {
	ctx := r.Context()

	mf, err := api.ds.MediaFile(ctx).Get(id)
	if errors.Is(err, model.ErrNotFound) {
		log.Error(r, "Requested MediaFileID not found ", "id", id)
		return nil, newError(responses.ErrorDataNotFound, "Song not found")
	}
	if err != nil {
		log.Error(r, "Error retrieving MediaFile", "id", id, err)
		return nil, err
	}

	response := newResponse()
	response.Song = new(childFromMediaFile(ctx, *mf))
	return response, nil
}

func (api *Router) GetGenres(r *http.Request) (*responses.Subsonic, error) {
	now := time.Now()
	user, cacheable := request.UserFrom(r.Context())
	cacheKey := ""
	if cacheable {
		cacheKey = genreResponseCacheKey(user)
		if genres, ok := api.genreCache.get(cacheKey, now); ok {
			response := newResponse()
			response.Genres = genres
			return response, nil
		}
	}

	ctx := r.Context()
	genres, err := api.ds.Genre(ctx).GetAll(model.QueryOptions{Sort: "song_count, album_count, name desc", Order: "desc"})
	if err != nil {
		log.Error(r, err)
		return nil, err
	}
	for i, g := range genres {
		if g.Name == "" {
			genres[i].Name = "<Empty>"
		}
	}

	response := newResponse()
	response.Genres = toGenres(genres)
	if cacheable {
		api.genreCache.put(cacheKey, now, response.Genres)
	}
	return response, nil
}

func (api *Router) getArtistInfo(r *http.Request) (*responses.ArtistInfoBase, *model.Artists, error) {
	ctx := r.Context()
	p := req.Params(r)
	id, err := p.String("id")
	if err != nil {
		return nil, nil, err
	}
	count := p.IntOr("count", 20)
	includeNotPresent := p.BoolOr("includeNotPresent", false)

	artist, err := api.provider.UpdateArtistInfo(ctx, id, count, includeNotPresent)
	if err != nil {
		return nil, nil, err
	}

	base := responses.ArtistInfoBase{}
	base.Biography = artist.Biography
	small, medium, large := entityImageURLs(r, artist.SmallImageUrl, artist.MediumImageUrl, artist.LargeImageUrl, artist.CoverArtID())
	base.SmallImageUrl = small
	base.MediumImageUrl = medium
	base.LargeImageUrl = large
	base.LastFmUrl = artist.ExternalUrl
	base.MusicBrainzID = artist.MbzArtistID

	return &base, &artist.SimilarArtists, nil
}

func (api *Router) GetArtistInfo(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	id, err := p.String("id")
	if err != nil {
		return nil, err
	}
	count := p.IntOr("count", 20)
	includeNotPresent := p.BoolOr("includeNotPresent", false)
	cacheKey := entityResponseCacheKey(r, "artistInfo", id+"|"+strconv.Itoa(count)+"|"+strconv.FormatBool(includeNotPresent))
	return api.cachedSubsonicResponse(r, cacheKey, func(r *http.Request) (*responses.Subsonic, error) {
		return api.loadArtistInfo(r, false)
	})
}

func (api *Router) GetArtistInfo2(r *http.Request) (*responses.Subsonic, error) {
	p := req.Params(r)
	id, err := p.String("id")
	if err != nil {
		return nil, err
	}
	count := p.IntOr("count", 20)
	includeNotPresent := p.BoolOr("includeNotPresent", false)
	cacheKey := entityResponseCacheKey(r, "artistInfo2", id+"|"+strconv.Itoa(count)+"|"+strconv.FormatBool(includeNotPresent))
	return api.cachedSubsonicResponse(r, cacheKey, func(r *http.Request) (*responses.Subsonic, error) {
		return api.loadArtistInfo(r, true)
	})
}

func (api *Router) loadArtistInfo(r *http.Request, useID3 bool) (*responses.Subsonic, error) {
	base, similarArtists, err := api.getArtistInfo(r)
	if err != nil {
		return nil, err
	}

	response := newResponse()
	if useID3 {
		response.ArtistInfo2 = &responses.ArtistInfo2{}
		response.ArtistInfo2.ArtistInfoBase = *base
		for _, s := range *similarArtists {
			similar := toArtistID3(r, s)
			if s.ID == "" {
				similar.Id = "-1"
			}
			response.ArtistInfo2.SimilarArtist = append(response.ArtistInfo2.SimilarArtist, similar)
		}
		return response, nil
	}

	response.ArtistInfo = &responses.ArtistInfo{}
	response.ArtistInfo.ArtistInfoBase = *base
	for _, s := range *similarArtists {
		similar := toArtist(r, s)
		if s.ID == "" {
			similar.Id = "-1"
		}
		response.ArtistInfo.SimilarArtist = append(response.ArtistInfo.SimilarArtist, similar)
	}
	return response, nil
}

func (api *Router) GetSimilarSongs(r *http.Request) (*responses.Subsonic, error) {
	ctx := r.Context()
	p := req.Params(r)
	id, err := p.String("id")
	if err != nil {
		return nil, err
	}
	count := p.IntOr("count", 50)

	songs, err := api.provider.SimilarSongs(ctx, id, count)
	if err != nil {
		return nil, err
	}

	response := newResponse()
	response.SimilarSongs = &responses.SimilarSongs{
		Song: slice.MapWithArg(songs, ctx, childFromMediaFile),
	}
	return response, nil
}

func (api *Router) GetSimilarSongs2(r *http.Request) (*responses.Subsonic, error) {
	res, err := api.GetSimilarSongs(r)
	if err != nil {
		return nil, err
	}

	response := newResponse()
	response.SimilarSongs2 = &responses.SimilarSongs2{
		Song: res.SimilarSongs.Song,
	}
	return response, nil
}

func (api *Router) GetTopSongs(r *http.Request) (*responses.Subsonic, error) {
	ctx := r.Context()
	p := req.Params(r)
	id, idErr := p.String("id")
	artist, err := p.String("artist")
	if err != nil && idErr != nil {
		return nil, err
	}
	count := p.IntOr("count", 50)

	songs, err := api.provider.TopSongs(ctx, artist, id, count)
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		return nil, err
	}

	response := newResponse()
	response.TopSongs = &responses.TopSongs{
		Song: slice.MapWithArg(songs, ctx, childFromMediaFile),
	}
	return response, nil
}

func (api *Router) buildArtistDirectory(ctx context.Context, artist *model.Artist) (*responses.Directory, error) {
	dir := &responses.Directory{}
	dir.Id = artist.ID
	dir.Name = artist.Name
	dir.PlayCount = artist.PlayCount
	if artist.PlayCount > 0 {
		dir.Played = artist.PlayDate
	}
	dir.AlbumCount = getArtistAlbumCount(artist)
	dir.UserRating = int32(artist.Rating)
	if conf.Server.Subsonic.EnableAverageRating {
		dir.AverageRating = artist.AverageRating
	}
	if artist.Starred {
		dir.Starred = artist.StarredAt
	}

	albums, err := api.ds.Album(ctx).GetAll(filter.AlbumsByArtistID(artist.ID))
	if err != nil {
		return nil, err
	}

	dir.Child = slice.MapWithArg(albums, ctx, childFromAlbum)
	return dir, nil
}

func (api *Router) buildArtist(r *http.Request, artist *model.Artist) (*responses.ArtistWithAlbumsID3, error) {
	ctx := r.Context()
	a := &responses.ArtistWithAlbumsID3{}
	a.ArtistID3 = toArtistID3(r, *artist)

	albums, err := api.ds.Album(ctx).GetAll(filter.AlbumsByArtistID(artist.ID))
	if err != nil {
		return nil, err
	}

	a.Album = slice.MapWithArg(albums, ctx, buildAlbumID3)
	return a, nil
}

func (api *Router) buildAlbumDirectory(ctx context.Context, album *model.Album) (*responses.Directory, error) {
	dir := &responses.Directory{}
	dir.Id = album.ID
	dir.Name = album.FullName()
	dir.Parent = album.AlbumArtistID
	dir.PlayCount = album.PlayCount
	if album.PlayCount > 0 {
		dir.Played = album.PlayDate
	}
	dir.UserRating = int32(album.Rating)
	if conf.Server.Subsonic.EnableAverageRating {
		dir.AverageRating = album.AverageRating
	}
	dir.SongCount = int32(album.SongCount)
	dir.CoverArt = album.CoverArtID().String()
	if album.Starred {
		dir.Starred = album.StarredAt
	}

	mfs, err := api.ds.MediaFile(ctx).GetAll(filter.SongsByAlbum(album.ID))
	if err != nil {
		return nil, err
	}

	dir.Child = slice.MapWithArg(mfs, ctx, childFromMediaFile)
	return dir, nil
}

func (api *Router) buildAlbum(ctx context.Context, album *model.Album, mfs model.MediaFiles) *responses.AlbumWithSongsID3 {
	dir := &responses.AlbumWithSongsID3{}
	dir.AlbumID3 = buildAlbumID3(ctx, *album)
	dir.Song = slice.MapWithArg(mfs, ctx, childFromMediaFile)
	return dir
}
