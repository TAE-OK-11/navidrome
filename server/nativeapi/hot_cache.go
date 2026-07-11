package nativeapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/core/stream/hotcache"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

const hotCacheAdminTimeout = 30 * time.Second

type hotCacheDashboard struct {
	Status   hotcache.StatusSnapshot      `json:"status"`
	Sessions []hotcache.SessionSnapshot   `json:"sessions"`
	Queue    []hotcache.QueueItemSnapshot `json:"queue"`
	Current  *hotcache.PromotionSnapshot  `json:"current"`
	Formats  []hotcache.FormatSnapshot    `json:"formats"`
	Events   []hotcache.Event             `json:"events"`
	Errors   []hotcache.Event             `json:"errors"`
	Artwork  []hotcache.Event             `json:"artwork"`
}

type hotCacheCandidate struct {
	MediaID    string  `json:"mediaId"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album"`
	Codec      string  `json:"codec"`
	Container  string  `json:"container"`
	Size       int64   `json:"size"`
	Duration   float32 `json:"duration"`
	CacheState string  `json:"cacheState"`
}

type hotCacheCandidatePage struct {
	Items   []hotCacheCandidate `json:"items"`
	HasMore bool                `json:"hasMore"`
}

type hotCachePromoteRequest struct {
	MediaIDs []string `json:"mediaIds"`
}

type hotCachePromoteResult struct {
	Accepted []string          `json:"accepted"`
	Rejected map[string]string `json:"rejected"`
}

func (api *Router) addHotCacheRoutes(r chi.Router) {
	manager := hotcache.GetManager()
	r.Route("/admin/hot-cache", func(r chi.Router) {
		r.Get("/dashboard", func(w http.ResponseWriter, request *http.Request) {
			limit := parseLimit(request.URL.Query().Get("eventLimit"))
			events := nonNilHotCacheSlice(manager.Events(0, limit))
			errorsOnly := nonNilHotCacheSlice(manager.Errors(0, limit))
			artwork := make([]hotcache.Event, 0)
			for _, event := range errorsOnly {
				if event.Category == "artwork" {
					artwork = append(artwork, event)
				}
			}
			writeHotCacheJSON(w, http.StatusOK, hotCacheDashboard{
				Status: manager.Status(), Sessions: nonNilHotCacheSlice(manager.Sessions()),
				Queue: nonNilHotCacheSlice(manager.Queue()), Current: manager.CurrentPromotion(),
				Formats: nonNilHotCacheSlice(manager.Formats()), Events: events,
				Errors: errorsOnly, Artwork: artwork,
			})
		})
		r.Get("/status", func(w http.ResponseWriter, _ *http.Request) {
			writeHotCacheJSON(w, http.StatusOK, manager.Status())
		})
		r.Get("/sessions", func(w http.ResponseWriter, _ *http.Request) {
			writeHotCacheJSON(w, http.StatusOK, manager.Sessions())
		})
		r.Get("/entries", func(w http.ResponseWriter, request *http.Request) {
			query := request.URL.Query()
			page := manager.Entries(hotCacheEntryQuery(query.Get("search"), query.Get("format"),
				query.Get("state"), query.Get("sort"), query.Get("order"), query.Get("offset"), query.Get("limit")))
			for index := range page.Items {
				if page.Items[index].Title != "" {
					continue
				}
				mediaFile, err := api.ds.MediaFile(request.Context()).GetForStreaming(page.Items[index].MediaID)
				if err == nil {
					page.Items[index].Title = mediaFile.Title
					page.Items[index].Artist = mediaFile.Artist
					page.Items[index].Album = mediaFile.Album
					page.Items[index].Codec = mediaFile.Codec
					page.Items[index].Container = mediaFile.Suffix
					page.Items[index].Extension = mediaFile.Suffix
					page.Items[index].BitRate = mediaFile.BitRate
				}
			}
			writeHotCacheJSON(w, http.StatusOK, page)
		})
		r.Get("/queue", func(w http.ResponseWriter, _ *http.Request) {
			writeHotCacheJSON(w, http.StatusOK, manager.Queue())
		})
		r.Get("/current", func(w http.ResponseWriter, _ *http.Request) {
			writeHotCacheJSON(w, http.StatusOK, manager.CurrentPromotion())
		})
		r.Get("/events", func(w http.ResponseWriter, request *http.Request) {
			writeHotCacheJSON(w, http.StatusOK, manager.Events(parseUint(request.URL.Query().Get("after")), parseLimit(request.URL.Query().Get("limit"))))
		})
		r.Get("/errors", func(w http.ResponseWriter, request *http.Request) {
			writeHotCacheJSON(w, http.StatusOK, manager.Errors(parseUint(request.URL.Query().Get("after")), parseLimit(request.URL.Query().Get("limit"))))
		})
		r.Get("/stats", func(w http.ResponseWriter, _ *http.Request) {
			writeHotCacheJSON(w, http.StatusOK, map[string]any{"status": manager.Status(), "formats": manager.Formats()})
		})
		r.Get("/formats", func(w http.ResponseWriter, _ *http.Request) {
			writeHotCacheJSON(w, http.StatusOK, manager.Formats())
		})
		r.Get("/artwork-errors", func(w http.ResponseWriter, request *http.Request) {
			errorsOnly := manager.Errors(parseUint(request.URL.Query().Get("after")), parseLimit(request.URL.Query().Get("limit")))
			result := make([]hotcache.Event, 0, len(errorsOnly))
			for _, event := range errorsOnly {
				if event.Category == "artwork" {
					result = append(result, event)
				}
			}
			writeHotCacheJSON(w, http.StatusOK, result)
		})
		r.Get("/candidates", api.hotCacheCandidates(manager))
		r.Post("/promote", api.hotCachePromoteMany(manager))

		r.Post("/entries/{mediaID}/promote", api.hotCacheMediaAction(manager, manager.Promote))
		r.Post("/entries/{mediaID}/retry", api.hotCacheMediaAction(manager, manager.Retry))
		r.Delete("/entries/{mediaID}", func(w http.ResponseWriter, request *http.Request) {
			mediaID := chi.URLParam(request, "mediaID")
			if mediaID == "" {
				http.Error(w, "media ID is required", http.StatusBadRequest)
				return
			}
			writeHotCacheAction(w, manager.Remove(mediaID))
		})
		r.Post("/queue/{mediaID}/cancel", func(w http.ResponseWriter, request *http.Request) {
			writeHotCacheAction(w, manager.Cancel(chi.URLParam(request, "mediaID")))
		})
		r.Post("/pause", func(w http.ResponseWriter, _ *http.Request) {
			manager.Pause()
			writeHotCacheJSON(w, http.StatusOK, manager.Status())
		})
		r.Post("/resume", func(w http.ResponseWriter, _ *http.Request) {
			manager.Resume()
			writeHotCacheJSON(w, http.StatusOK, manager.Status())
		})
		r.Post("/verify", withHotCacheTimeout(func(ctx context.Context) any { return manager.Verify(ctx) }))
		r.Post("/cleanup", withHotCacheTimeout(func(ctx context.Context) any { return manager.Cleanup(ctx) }))
		r.Post("/purge", func(w http.ResponseWriter, request *http.Request) {
			if request.Header.Get("X-ND-Confirm") != "purge-hot-cache" {
				http.Error(w, "purge confirmation is required", http.StatusPreconditionFailed)
				return
			}
			ctx, cancel := context.WithTimeout(request.Context(), hotCacheAdminTimeout)
			defer cancel()
			writeHotCacheJSON(w, http.StatusOK, manager.Purge(ctx))
		})
		r.Post("/artwork/recheck", func(w http.ResponseWriter, _ *http.Request) {
			hotcache.ResetArtworkFailures()
			writeHotCacheJSON(w, http.StatusOK, map[string]bool{"reset": true})
		})
		r.Delete("/events", func(w http.ResponseWriter, _ *http.Request) {
			manager.ResetEvents()
			writeHotCacheJSON(w, http.StatusOK, map[string]bool{"reset": true})
		})
		r.Delete("/stats", func(w http.ResponseWriter, _ *http.Request) {
			manager.ResetStats()
			writeHotCacheJSON(w, http.StatusOK, map[string]bool{"reset": true})
		})
	})
}

func (api *Router) hotCacheCandidates(manager hotcache.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		search := strings.TrimSpace(request.URL.Query().Get("search"))
		if len([]rune(search)) < 2 {
			writeHotCacheJSON(w, http.StatusOK, hotCacheCandidatePage{Items: []hotCacheCandidate{}})
			return
		}
		offset := max(parseInt(request.URL.Query().Get("offset"), 0), 0)
		limit := min(max(parseInt(request.URL.Query().Get("limit"), 25), 1), 50)
		files, err := api.ds.MediaFile(request.Context()).Search(search, model.QueryOptions{Max: limit + 1, Offset: offset})
		if err != nil {
			writeHotCacheJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		hasMore := len(files) > limit
		if hasMore {
			files = files[:limit]
		}
		ids := make([]string, 0, len(files))
		for i := range files {
			ids = append(ids, files[i].ID)
		}
		states := manager.MediaStates(ids)
		items := make([]hotCacheCandidate, 0, len(files))
		for i := range files {
			file := &files[i]
			state := states[file.ID]
			if state == "" {
				state = "available"
			}
			items = append(items, hotCacheCandidate{
				MediaID: file.ID, Title: file.Title, Artist: file.Artist, Album: file.Album,
				Codec: file.Codec, Container: file.Suffix, Size: file.Size,
				Duration: file.Duration, CacheState: state,
			})
		}
		writeHotCacheJSON(w, http.StatusOK, hotCacheCandidatePage{Items: items, HasMore: hasMore})
	}
}

func (api *Router) hotCachePromoteMany(manager hotcache.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(w, request.Body, 64<<10)
		var payload hotCachePromoteRequest
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			writeHotCacheJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid promotion request"})
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			writeHotCacheJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid promotion request"})
			return
		}
		if len(payload.MediaIDs) == 0 || len(payload.MediaIDs) > 50 {
			writeHotCacheJSON(w, http.StatusBadRequest, map[string]string{"error": "select between 1 and 50 tracks"})
			return
		}

		ctx, cancel := context.WithTimeout(request.Context(), hotCacheAdminTimeout)
		defer cancel()
		result := hotCachePromoteResult{Accepted: []string{}, Rejected: map[string]string{}}
		seen := make(map[string]struct{}, len(payload.MediaIDs))
		for _, mediaID := range payload.MediaIDs {
			mediaID = strings.TrimSpace(mediaID)
			if mediaID == "" {
				result.Rejected[mediaID] = "media ID is required"
				continue
			}
			if _, duplicate := seen[mediaID]; duplicate {
				continue
			}
			seen[mediaID] = struct{}{}
			mediaFile, err := api.ds.MediaFile(ctx).GetForStreaming(mediaID)
			if err == nil {
				err = manager.Promote(ctx, mediaFile)
			}
			if err != nil {
				result.Rejected[mediaID] = err.Error()
				continue
			}
			result.Accepted = append(result.Accepted, mediaID)
		}
		writeHotCacheJSON(w, http.StatusAccepted, result)
	}
}

func nonNilHotCacheSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func (api *Router) hotCacheMediaAction(manager hotcache.Manager, action func(context.Context, *model.MediaFile) error) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		mediaID := chi.URLParam(request, "mediaID")
		if mediaID == "" {
			http.Error(w, "media ID is required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(request.Context(), hotCacheAdminTimeout)
		defer cancel()
		mediaFile, err := api.ds.MediaFile(ctx).GetForStreaming(mediaID)
		if err == nil {
			err = action(ctx, mediaFile)
		}
		writeHotCacheAction(w, err)
	}
}

func hotCacheEntryQuery(search, format, state, sortField, order, offset, limit string) hotcache.EntryQuery {
	return hotcache.EntryQuery{
		Search: search, Format: format, State: state, Sort: sortField, Order: order,
		Offset: parseInt(offset, 0), Limit: min(max(parseInt(limit, 50), 1), 200),
	}
}

func withHotCacheTimeout(action func(context.Context) any) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), hotCacheAdminTimeout)
		defer cancel()
		writeHotCacheJSON(w, http.StatusOK, action(ctx))
	}
}

func writeHotCacheAction(w http.ResponseWriter, err error) {
	if err == nil {
		writeHotCacheJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
		return
	}
	status := http.StatusConflict
	if errors.Is(err, model.ErrNotFound) {
		status = http.StatusNotFound
	} else if strings.Contains(err.Error(), "required") {
		status = http.StatusBadRequest
	}
	writeHotCacheJSON(w, status, map[string]string{"error": err.Error()})
}

func writeHotCacheJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Debug("Could not write Hot Cache administrator response", err)
	}
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseUint(value string) uint64 {
	parsed, _ := strconv.ParseUint(value, 10, 64)
	return parsed
}

func parseLimit(value string) int {
	return min(max(parseInt(value, 200), 1), 1000)
}
