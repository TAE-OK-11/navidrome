package subsonic

import (
	"context"
	"github.com/navidrome/navidrome/core/publicurl"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server/subsonic/responses"
)

const (
	entityResponseCacheTTL   = 45 * time.Second
	entityResponseCacheLimit = 256
)

type entityResponseCache struct {
	catalogCache[*responses.Subsonic]
}

func (c *entityResponseCache) put(key string, now time.Time, value *responses.Subsonic) {
	c.store(key, now, value, entityResponseCacheLimit, entityResponseCacheTTL)
}
func (c *entityResponseCache) deleteBySuffix(suffix string) {
	c.invalidate(func(key string) bool { return strings.HasSuffix(key, suffix) })
}

func (c *entityResponseCache) deleteByEntityID(id string) {
	c.deleteBySuffix("|" + id)
}

// Library scope alone cannot key annotated media or private playlists.
func catalogUserKey(ctx context.Context) string {
	user, ok := request.UserFrom(ctx)
	if !ok {
		return "anonymous"
	}
	return strconv.Quote(user.ID) + ":" + genreResponseCacheKey(user)
}

func entityResponseCacheKey(r *http.Request, kind, id string) string {
	format, bitrate := getTranscoding(r.Context())
	player, _ := request.PlayerFrom(r.Context())
	// Rendered responses also vary by client compatibility, player settings and
	// public origin. Quote variable segments to keep delimiters unambiguous.
	return catalogUserKey(r.Context()) + "|" + strconv.Quote(clientNameFrom(r.Context())) +
		"|" + strconv.Quote(format) + ":" + strconv.Itoa(bitrate) + ":" + strconv.FormatBool(player.ReportRealPath) +
		"|" + strconv.Quote(publicurl.PublicURL(r, "/", nil)) + "|" + kind + "|" + id
}

func (api *Router) cachedSubsonicResponse(r *http.Request, cacheKey string, loader func(*http.Request) (*responses.Subsonic, error)) (*responses.Subsonic, error) {
	return api.entityCache.load(r.Context(), cacheKey, entityResponseCacheLimit, entityResponseCacheTTL, func(ctx context.Context) (*responses.Subsonic, error) {
		return loader(r.WithContext(ctx))
	})
}
