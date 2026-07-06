package subsonic

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	. "github.com/Masterminds/squirrel"
	"github.com/deluan/sanitize"
	"github.com/navidrome/navidrome/core/publicurl"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	"github.com/navidrome/navidrome/utils/req"
	"golang.org/x/sync/errgroup"
)

type searchParams struct {
	query        string
	artistCount  int
	artistOffset int
	albumCount   int
	albumOffset  int
	songCount    int
	songOffset   int
}

func (api *Router) getSearchParams(r *http.Request) (*searchParams, error) {
	p := req.Params(r)
	sp := &searchParams{}
	sp.query = p.StringOr("query", `""`)
	sp.artistCount = p.IntOr("artistCount", 20)
	sp.artistOffset = p.IntOr("artistOffset", 0)
	sp.albumCount = p.IntOr("albumCount", 20)
	sp.albumOffset = p.IntOr("albumOffset", 0)
	sp.songCount = p.IntOr("songCount", 20)
	sp.songOffset = p.IntOr("songOffset", 0)
	return sp, nil
}

type searchFunc[T any] func(q string, options ...model.QueryOptions) (T, error)

func callSearch[T any](ctx context.Context, s searchFunc[T], q string, options model.QueryOptions, result *T) func() error {
	return func() error {
		if options.Max == 0 {
			return nil
		}
		var err error
		if log.IsGreaterOrEqualTo(log.LevelTrace) {
			typ := strings.TrimPrefix(reflect.TypeOf(*result).String(), "model.")
			start := time.Now()
			*result, err = s(q, options)
			if err != nil {
				log.Error(ctx, "Error searching "+typ, "query", q, "elapsed", time.Since(start), err)
			} else {
				log.Trace(ctx, "Search for "+typ+" completed", "query", q, "elapsed", time.Since(start))
			}
			return nil
		}
		*result, err = s(q, options)
		if err != nil {
			log.Error(ctx, "Error searching", "query", q, err)
		}
		return nil
	}
}

func (api *Router) searchAll(ctx context.Context, sp *searchParams, musicFolderIds []int) (mediaFiles model.MediaFiles, albums model.Albums, artists model.Artists) {
	start := time.Now()
	q := sanitize.Accents(strings.ToLower(strings.TrimSuffix(sp.query, "*")))

	// Build options with offset/size/filters packed in
	songOpts := model.QueryOptions{Max: sp.songCount, Offset: sp.songOffset}
	albumOpts := model.QueryOptions{Max: sp.albumCount, Offset: sp.albumOffset}
	artistOpts := model.QueryOptions{Max: sp.artistCount, Offset: sp.artistOffset}

	if len(musicFolderIds) > 0 {
		songOpts.Filters = Eq{"library_id": musicFolderIds}
		albumOpts.Filters = Eq{"library_id": musicFolderIds}
		artistOpts.Filters = Eq{"library_id": musicFolderIds}
	}

	// Run searches in parallel
	g, ctx := errgroup.WithContext(ctx)
	g.Go(callSearch(ctx, api.ds.MediaFile(ctx).Search, q, songOpts, &mediaFiles))
	g.Go(callSearch(ctx, api.ds.Album(ctx).Search, q, albumOpts, &albums))
	g.Go(callSearch(ctx, api.ds.Artist(ctx).Search, q, artistOpts, &artists))
	err := g.Wait()
	if err == nil {
		if log.IsGreaterOrEqualTo(log.LevelDebug) {
			log.Debug(ctx, "Search completed", "songs", len(mediaFiles), "albums", len(albums), "artists", len(artists),
				"query", sp.query, "elapsedTime", time.Since(start))
		}
	} else {
		log.Warn(ctx, "Search was interrupted", "query", sp.query, "elapsedTime", time.Since(start), err)
	}
	return mediaFiles, albums, artists
}

func (api *Router) Search2(r *http.Request) (*responses.Subsonic, error) {
	ctx := r.Context()
	sp, err := api.getSearchParams(r)
	if err != nil {
		return nil, err
	}

	// Get optional library IDs from musicFolderId parameter
	musicFolderIds, err := selectedMusicFolderFilterIds(r)
	if err != nil {
		return nil, err
	}
	mfs, als, as := api.searchAll(ctx, sp, musicFolderIds)

	response := newResponse()
	searchResult2 := &responses.SearchResult2{}
	searchResult2.Artist = make([]responses.Artist, len(as))
	for i, artist := range as {
		a := responses.Artist{
			Id:             artist.ID,
			Name:           artist.Name,
			UserRating:     int32(artist.Rating),
			CoverArt:       artist.CoverArtID().String(),
			ArtistImageUrl: publicurl.ImageURL(r, artist.CoverArtID(), 600),
		}
		if artist.Starred {
			a.Starred = artist.StarredAt
		}
		searchResult2.Artist[i] = a
	}
	searchResult2.Album = albumChildren(ctx, als)
	searchResult2.Song = mediaFileChildren(ctx, mfs)
	response.SearchResult2 = searchResult2
	return response, nil
}

func (api *Router) Search3(r *http.Request) (*responses.Subsonic, error) {
	ctx := r.Context()
	sp, err := api.getSearchParams(r)
	if err != nil {
		return nil, err
	}

	// Get optional library IDs from musicFolderId parameter
	musicFolderIds, err := selectedMusicFolderFilterIds(r)
	if err != nil {
		return nil, err
	}
	mfs, als, as := api.searchAll(ctx, sp, musicFolderIds)

	response := newResponse()
	searchResult3 := &responses.SearchResult3{}
	searchResult3.Artist = artistID3s(r, as)
	searchResult3.Album = albumID3s(ctx, als)
	searchResult3.Song = mediaFileChildren(ctx, mfs)
	response.SearchResult3 = searchResult3
	return response, nil
}

func selectedMusicFolderFilterIds(r *http.Request) ([]int, error) {
	p := req.Params(r)
	musicFolderIds, err := p.Ints("musicFolderId")
	if errors.Is(err, req.ErrMissingParam) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	accessibleLibraryIds := getUserAccessibleLibraries(r.Context()).IDs()
	for _, id := range musicFolderIds {
		if !slices.Contains(accessibleLibraryIds, id) {
			return nil, newError(responses.ErrorDataNotFound, "Library %d not found or not accessible", id)
		}
	}
	return musicFolderIds, nil
}

func albumChildren(ctx context.Context, albums model.Albums) []responses.Child {
	response := make([]responses.Child, len(albums))
	for i, album := range albums {
		response[i] = childFromAlbum(ctx, album)
	}
	return response
}

func mediaFileChildren(ctx context.Context, mediaFiles model.MediaFiles) []responses.Child {
	response := make([]responses.Child, len(mediaFiles))
	for i, mf := range mediaFiles {
		response[i] = childFromMediaFile(ctx, mf)
	}
	return response
}

func artistID3s(r *http.Request, artists model.Artists) []responses.ArtistID3 {
	response := make([]responses.ArtistID3, len(artists))
	for i, artist := range artists {
		response[i] = toArtistID3(r, artist)
	}
	return response
}

func albumID3s(ctx context.Context, albums model.Albums) []responses.AlbumID3 {
	response := make([]responses.AlbumID3, len(albums))
	for i, album := range albums {
		response[i] = buildAlbumID3(ctx, album)
	}
	return response
}
