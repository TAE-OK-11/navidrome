package nativeapi

import (
	"context"
	"encoding/json"
	"errors"
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

func (api *Router) addHotCacheRoutes(r chi.Router) {
	manager := hotcache.GetManager()
	r.Route("/admin/hot-cache", func(r chi.Router) {
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
