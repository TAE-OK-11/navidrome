package subsonic

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"testing"

	"github.com/deluan/rest"
	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/adapters/rustsearch"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core"
	"github.com/navidrome/navidrome/core/artwork"
	"github.com/navidrome/navidrome/core/external"
	lyricssvc "github.com/navidrome/navidrome/core/lyrics"
	"github.com/navidrome/navidrome/core/metrics"
	"github.com/navidrome/navidrome/core/playback"
	playlistsvc "github.com/navidrome/navidrome/core/playlists"
	"github.com/navidrome/navidrome/core/scrobbler"
	sonicsvc "github.com/navidrome/navidrome/core/sonic"
	"github.com/navidrome/navidrome/core/stream"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server"
	"github.com/navidrome/navidrome/server/events"
	"github.com/navidrome/navidrome/server/responsecache"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	"github.com/navidrome/navidrome/utils/req"
)

const Version = "1.16.1"

var validJSIdentifier = regexp.MustCompile(`^[a-zA-Z_$][a-zA-Z0-9_$.]*$`)

var responseBufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

func borrowResponseBuffer() *bytes.Buffer {
	buf := responseBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func recycleResponseBuffer(buf *bytes.Buffer) {
	if buf.Cap() > 256<<10 {
		return
	}
	responseBufferPool.Put(buf)
}

func encodeJSON(buf *bytes.Buffer, value any) error {
	if err := json.NewEncoder(buf).Encode(value); err != nil {
		return err
	}
	// Encoder always appends a newline; keep the historical Marshal payload.
	if n := buf.Len(); n > 0 && buf.Bytes()[n-1] == '\n' {
		buf.Truncate(n - 1)
	}
	return nil
}

type handler = func(*http.Request) (*responses.Subsonic, error)
type handlerRaw = func(http.ResponseWriter, *http.Request) (*responses.Subsonic, error)

type Router struct {
	http.Handler
	ds                model.DataStore
	artwork           artwork.Artwork
	streamer          stream.MediaStreamer
	archiver          core.Archiver
	players           core.Players
	provider          external.Provider
	playlists         playlistsvc.Playlists
	scanner           model.Scanner
	scrobbler         scrobbler.PlayTracker
	share             core.Share
	playback          playback.PlaybackServer
	metrics           metrics.Metrics
	lyrics            lyricssvc.Lyrics
	transcodeDecision stream.TranscodeDecider
	sonic             *sonicsvc.Sonic
	genreCache        genreResponseCache
	musicFoldersCache musicFoldersResponseCache
	artistIndexCache  artistIndexCache
	albumListCache    albumListCache
	entityCache       entityResponseCache
	streamFiles       *streamMediaCache
	rustSearch        *rustsearch.Engine
	internalHandlers  map[string]handlerRaw
}

func New(ds model.DataStore, artwork artwork.Artwork, streamer stream.MediaStreamer, archiver core.Archiver,
	players core.Players, provider external.Provider, scanner model.Scanner, _ events.Broker,
	playlists playlistsvc.Playlists, scrobbler scrobbler.PlayTracker, share core.Share, playback playback.PlaybackServer,
	metrics metrics.Metrics, lyrics lyricssvc.Lyrics, transcodeDecision stream.TranscodeDecider,
	sonic *sonicsvc.Sonic,
) *Router {
	r := &Router{
		ds:                ds,
		artwork:           artwork,
		streamer:          streamer,
		archiver:          archiver,
		players:           players,
		provider:          provider,
		playlists:         playlists,
		scanner:           scanner,
		scrobbler:         scrobbler,
		share:             share,
		playback:          playback,
		metrics:           metrics,
		lyrics:            lyrics,
		transcodeDecision: transcodeDecision,
		sonic:             sonic,
		streamFiles:       newStreamMediaCache(streamMediaCacheLimit, streamMediaCacheTTL),
		internalHandlers:  make(map[string]handlerRaw),
	}
	if rustsearch.Available() {
		r.rustSearch = rustsearch.New()
		r.rustSearch.ListenForScans(ds)
		if !testing.Testing() {
			go func() {
				defer func() {
					if rec := recover(); rec != nil {
						log.Warn("Rust search startup panicked; SQLite search remains active", rec)
					}
				}()
				if err := r.rustSearch.Rebuild(context.Background(), ds); err != nil {
					log.Warn("Rust search startup failed; SQLite search remains active", err)
				}
			}()
		}
	}
	r.Handler = r.routes()
	r.registerResponseCacheHooks()
	return r
}

// RebuildRustSearch rebuilds the Tantivy index from the current library.
// Production starts this from New; tests (including e2e) call it when they
// need rust-backed search instead of SQLite FTS.
func (api *Router) RebuildRustSearch(ctx context.Context) error {
	if api == nil || api.rustSearch == nil {
		return nil
	}
	api.rustSearch.EnableForTests()
	return api.rustSearch.Rebuild(ctx, api.ds)
}

func (api *Router) registerResponseCacheHooks() {
	responsecache.RegisterPlaylistsInvalidator(func() {
		api.entityCache.deleteBySuffix("|playlists")
	})
	responsecache.RegisterEntityInvalidator(api.entityCache.deleteByEntityID)
}

func (api *Router) routes() http.Handler {
	r := chi.NewRouter()

	if conf.Server.Prometheus.Enabled {
		r.Use(recordStats(api.metrics))
	}

	r.Use(postFormToQueryParams)

	// Public
	api.h(r, "getOpenSubsonicExtensions", api.GetOpenSubsonicExtensions)

	// Protected: params → auth → last-access, then a single player-aware
	// branch for catalog/JSON APIs. Binary endpoints keep their own player
	// lookup (fresh vs cached) so stream/download do not share that path.
	r.Group(func(r chi.Router) {
		r.Use(checkRequiredParameters)
		r.Use(authenticate(api.ds))
		r.Use(server.UpdateLastAccessMiddleware(api.ds))

		api.registerSystemRoutes(r)
		api.registerPlayerRoutes(r)
		api.registerMediaRoutes(r)

		if !conf.Server.EnableSharing {
			h501(r, "getShares", "createShare", "updateShare", "deleteShare")
		}
		if !conf.Server.Jukebox.Enabled {
			h501(r, "jukeboxControl")
		}

		// Not Implemented (yet?)
		h501(r, "getPodcasts", "getNewestPodcasts", "refreshPodcasts", "createPodcastChannel", "deletePodcastChannel",
			"deletePodcastEpisode", "downloadPodcastEpisode")
		h501(r, "createUser", "updateUser", "deleteUser", "changePassword")

		// Deprecated/Won't implement/Out of scope endpoints
		h410(r, "search")
		h410(r, "getChatMessages", "addChatMessage")
		h410(r, "getVideos", "getVideoInfo", "getCaptions", "hls")
	})
	return r
}

func (api *Router) registerSystemRoutes(r chi.Router) {
	api.h(r, "ping", api.Ping)
	api.h(r, "getLicense", api.GetLicense)
	api.h(r, "tokenInfo", api.GetTokenInfo)
	api.h(r, "getMusicFolders", api.GetMusicFolders)
	api.h(r, "getGenres", api.GetGenres)
	api.h(r, "getScanStatus", api.GetScanStatus)
	api.h(r.With(adminOnly, rejectCrossSiteProxyMutation), "startScan", api.StartScan)
}

func (api *Router) registerPlayerRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(getPlayer(api.players))

		api.h(r, "getIndexes", api.GetIndexes)
		api.h(r, "getArtists", api.GetArtists)
		api.h(r, "getMusicDirectory", api.GetMusicDirectory)
		api.h(r, "getArtist", api.GetArtist)
		api.h(r, "getAlbum", api.GetAlbum)
		api.h(r, "getSong", api.GetSong)
		api.h(r, "getAlbumInfo", api.GetAlbumInfo)
		api.h(r, "getAlbumInfo2", api.GetAlbumInfo)
		api.h(r, "getArtistInfo", api.GetArtistInfo)
		api.h(r, "getArtistInfo2", api.GetArtistInfo2)
		api.h(r, "getTopSongs", api.GetTopSongs)
		api.h(r, "getSimilarSongs", api.GetSimilarSongs)
		api.h(r, "getSimilarSongs2", api.GetSimilarSongs2)
		api.hr(r, "getSonicSimilarTracks", api.GetSonicSimilarTracks)
		api.hr(r, "findSonicPath", api.FindSonicPath)

		api.hr(r, "getAlbumList", api.GetAlbumList)
		api.hr(r, "getAlbumList2", api.GetAlbumList2)
		api.h(r, "getStarred", api.GetStarred)
		api.h(r, "getStarred2", api.GetStarred2)
		api.h(r, "getNowPlaying", api.GetNowPlaying)
		api.h(r, "getRandomSongs", api.GetRandomSongs)
		api.h(r, "getSongsByGenre", api.GetSongsByGenre)

		api.h(r, "getPlaylists", api.GetPlaylists)
		api.h(r, "getPlaylist", api.GetPlaylist)
		api.h(r.With(rejectCrossSiteProxyMutation), "createPlaylist", api.CreatePlaylist)
		api.h(r.With(rejectCrossSiteProxyMutation), "deletePlaylist", api.DeletePlaylist)
		api.h(r.With(rejectCrossSiteProxyMutation), "updatePlaylist", api.UpdatePlaylist)

		api.h(r, "getBookmarks", api.GetBookmarks)
		api.h(r.With(rejectCrossSiteProxyMutation), "createBookmark", api.CreateBookmark)
		api.h(r.With(rejectCrossSiteProxyMutation), "deleteBookmark", api.DeleteBookmark)
		api.h(r, "getPlayQueue", api.GetPlayQueue)
		api.h(r, "getPlayQueueByIndex", api.GetPlayQueueByIndex)
		api.h(r.With(rejectCrossSiteProxyMutation), "savePlayQueue", api.SavePlayQueue)
		api.h(r.With(rejectCrossSiteProxyMutation), "savePlayQueueByIndex", api.SavePlayQueueByIndex)

		api.h(r, "search2", api.Search2)
		api.h(r, "search3", api.Search3)

		api.h(r, "getUser", api.GetUser)
		api.h(r.With(adminOnly), "getUsers", api.GetUsers)

		api.h(r, "getInternetRadioStations", api.GetInternetRadios)
		r.Group(func(r chi.Router) {
			r.Use(adminOnly)
			r.Use(rejectCrossSiteProxyMutation)
			api.h(r, "createInternetRadioStation", api.CreateInternetRadio)
			api.h(r, "deleteInternetRadioStation", api.DeleteInternetRadio)
			api.h(r, "updateInternetRadioStation", api.UpdateInternetRadio)
		})

		r.Group(func(r chi.Router) {
			r.Use(rejectCrossSiteProxyMutation)
			api.h(r, "setRating", api.SetRating)
			api.h(r, "star", api.Star)
			api.h(r, "unstar", api.Unstar)
			api.h(r, "scrobble", api.Scrobble)
			api.h(r, "reportPlayback", api.ReportPlayback)
		})

		if conf.Server.EnableSharing {
			api.h(r, "getShares", api.GetShares)
			api.h(r.With(rejectCrossSiteProxyMutation), "createShare", api.CreateShare)
			api.h(r.With(rejectCrossSiteProxyMutation), "updateShare", api.UpdateShare)
			api.h(r.With(rejectCrossSiteProxyMutation), "deleteShare", api.DeleteShare)
		}

		if conf.Server.Jukebox.Enabled {
			api.h(r.With(rejectCrossSiteProxyMutation), "jukeboxControl", api.JukeboxControl)
		}
	})
}

func (api *Router) registerMediaRoutes(r chi.Router) {
	api.hr(r, "getAvatar", api.GetAvatar)
	api.h(r, "getLyrics", api.GetLyrics)
	api.h(r, "getLyricsBySongId", api.GetLyricsBySongId)

	r.Group(func(r chi.Router) {
		r.Use(getStreamPlayer(api.players))
		api.hr(r, "stream", api.Stream)
	})
	r.Group(func(r chi.Router) {
		r.Use(getFreshPlayer(api.players))
		api.hr(r, "download", api.Download)
		api.hr(r, "getTranscodeDecision", api.GetTranscodeDecision)
		api.hr(r, "getTranscodeStream", api.GetTranscodeStream)
	})
	r.Group(func(r chi.Router) {
		r.Use(server.ThrottleBacklog(conf.Server.DevArtworkMaxRequests, conf.Server.DevArtworkThrottleBacklogLimit,
			conf.Server.DevArtworkThrottleBacklogTimeout))
		api.hr(r, "getCoverArt", api.GetCoverArt)
	})
}

// Add a Subsonic handler that requires an http.ResponseWriter (ex: stream, getCoverArt...)
func hr(r chi.Router, path string, f handlerRaw) {
	handle := func(w http.ResponseWriter, r *http.Request) {
		res, err := f(w, r)
		if err != nil {
			sendError(w, r, err)
			return
		}
		if r.Context().Err() != nil {
			if log.IsGreaterOrEqualTo(log.LevelDebug) {
				log.Warn(r.Context(), "Request was interrupted", "endpoint", r.URL.Path, r.Context().Err())
			}
			return
		}
		if res != nil {
			sendResponse(w, r, res)
		}
	}
	addHandler(r, path, handle)
}

// Add a handler that returns 501 - Not implemented. Used to signal that an endpoint is not implemented yet
func h501(r chi.Router, paths ...string) {
	for _, path := range paths {
		handle := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte("This endpoint is not implemented, but may be in future releases"))
		}
		addHandler(r, path, handle)
	}
}

// Add a handler that returns 410 - Gone. Used to signal that an endpoint will not be implemented
func h410(r chi.Router, paths ...string) {
	for _, path := range paths {
		handle := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusGone)
			_, _ = w.Write([]byte("This endpoint will not be implemented"))
		}
		addHandler(r, path, handle)
	}
}

func addHandler(r chi.Router, path string, handle func(w http.ResponseWriter, r *http.Request)) {
	r.HandleFunc("/"+path, handle)
	r.HandleFunc("/"+path+".view", handle)
}

func mapToSubsonicError(err error) subError {
	switch {
	case errors.Is(err, errSubsonic): // do nothing
	case errors.Is(err, req.ErrMissingParam):
		err = newError(responses.ErrorMissingParameter, err.Error())
	case errors.Is(err, req.ErrInvalidParam):
		err = newError(responses.ErrorGeneric, err.Error())
	case errors.Is(err, model.ErrNotFound), errors.Is(err, rest.ErrNotFound):
		err = newError(responses.ErrorDataNotFound, "data not found")
	case errors.Is(err, model.ErrNotAuthorized), errors.Is(err, rest.ErrPermissionDenied),
		errors.Is(err, model.ErrPlaylistNotEditable):
		err = newError(responses.ErrorAuthorizationFail)
	case errors.Is(err, stream.ErrTooManyTranscodes):
		err = newError(responses.ErrorGeneric, "too many concurrent transcodes, please retry shortly")
	default:
		err = newError(responses.ErrorGeneric, fmt.Sprintf("Internal Server Error: %s", err))
	}
	var subErr subError
	errors.As(err, &subErr)
	return subErr
}

func sendError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, stream.ErrTooManyTranscodes) {
		w.Header().Set("Retry-After", strconv.Itoa(stream.RetryAfterSeconds))
		sendResponseWithStatus(w, r, errorResponse(err), http.StatusTooManyRequests)
		return
	}
	sendResponse(w, r, errorResponse(err))
}

func errorResponse(err error) *responses.Subsonic {
	subErr := mapToSubsonicError(err)
	response := newResponse()
	response.Status = responses.StatusFailed
	response.Error = &responses.Error{Code: subErr.code, Message: subErr.Error(), HelpUrl: subErr.HelpUrl()}
	return response
}

func sendResponse(w http.ResponseWriter, r *http.Request, payload *responses.Subsonic) {
	sendResponseWithStatus(w, r, payload, 0)
}

// sendResponseWithStatus writes the response body in the format requested by
// the client. When status is non-zero, WriteHeader is called with that code
// before the body is written; callers that need to set additional headers
// (e.g. Retry-After) must set them before calling.
func sendResponseWithStatus(w http.ResponseWriter, r *http.Request, payload *responses.Subsonic, status int) {
	p := req.Params(r)
	f := p.StringOr("f", "")
	buf := borrowResponseBuffer()
	defer recycleResponseBuffer(buf)
	var err error
	switch f {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		err = encodeJSON(buf, responses.JsonWrapper{Subsonic: *payload})
	case "jsonp":
		callback := p.StringOr("callback", "")
		if !validJSIdentifier.MatchString(callback) {
			log.Warn(r.Context(), "Invalid JSONP callback parameter", "callback", callback)
			w.Header().Set("Content-Type", "application/json")
			errResp := newResponse()
			errResp.Status = responses.StatusFailed
			errResp.Error = &responses.Error{Code: responses.ErrorGeneric, Message: "invalid callback parameter"}
			_ = encodeJSON(buf, responses.JsonWrapper{Subsonic: *errResp})
			break
		}
		w.Header().Set("Content-Type", "application/javascript")
		buf.WriteString(callback)
		buf.WriteByte('(')
		err = encodeJSON(buf, responses.JsonWrapper{Subsonic: *payload})
		if err == nil {
			buf.WriteByte(')')
		}
	default:
		w.Header().Set("Content-Type", "application/xml")
		err = xml.NewEncoder(buf).Encode(payload)
	}
	// This should never happen, but if it does, we need to know
	if err != nil {
		log.Error(r.Context(), "Error marshalling response", "format", f, err)
		sendError(w, r, err)
		return
	}
	if status != 0 {
		w.WriteHeader(status)
	}

	response := buf.Bytes()
	if payload.Status == responses.StatusOK {
		if log.IsGreaterOrEqualTo(log.LevelTrace) {
			log.Debug(r.Context(), "API: Successful response", "endpoint", r.URL.Path, "status", "OK", "body", string(response))
		} else if log.IsGreaterOrEqualTo(log.LevelDebug) {
			log.Debug(r.Context(), "API: Successful response", "endpoint", r.URL.Path, "status", "OK")
		}
	} else {
		log.Warn(r.Context(), "API: Failed response", "endpoint", r.URL.Path, "error", payload.Error.Code, "message", payload.Error.Message)
	}

	statusPointer, ok := r.Context().Value(subsonicErrorPointer).(*int32)

	if ok && statusPointer != nil {
		if payload.Status == responses.StatusOK {
			*statusPointer = 0
		} else {
			*statusPointer = payload.Error.Code
		}
	}

	if w.Header().Get("Content-Length") == "" {
		w.Header().Set("Content-Length", strconv.Itoa(len(response)))
	}
	if _, err := w.Write(response); err != nil { //nolint:gosec
		if log.IsGreaterOrEqualTo(log.LevelTrace) {
			log.Error(r, "Error sending response to client", "endpoint", r.URL.Path, "payload", string(response), err)
		} else {
			log.Error(r, "Error sending response to client", "endpoint", r.URL.Path, err)
		}
	}
}
