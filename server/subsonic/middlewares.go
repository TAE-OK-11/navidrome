package subsonic

import (
	"cmp"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	ua "github.com/mileusna/useragent"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/core/metrics"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	. "github.com/navidrome/navidrome/utils/gg"
	"github.com/navidrome/navidrome/utils/req"
)

func postFormToQueryParams(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hasFormBody(r) {
			next.ServeHTTP(w, r)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB
		err := r.ParseForm()
		if err != nil {
			sendError(w, r, newError(responses.ErrorGeneric, err.Error()))
			return
		}
		r.URL.RawQuery = r.Form.Encode()

		next.ServeHTTP(w, r)
	})
}

func hasFormBody(r *http.Request) bool {
	if r.Body == nil || r.Body == http.NoBody || r.ContentLength == 0 {
		return false
	}

	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return false
	}

	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	return strings.HasPrefix(contentType, "application/x-www-form-urlencoded")
}

func fromInternalOrProxyAuth(r *http.Request) (string, bool) {
	username := server.InternalAuth(r)

	// If the username comes from internal auth, do not also do reverse proxy auth, as
	// the request will have no reverse proxy IP
	if username != "" {
		return username, true
	}

	return server.UsernameFromExtAuthHeader(r), false
}

func checkRequiredParameters(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, p := req.WithParams(r)

		username, _ := fromInternalOrProxyAuth(r)
		if username == "" {
			var err error
			username, err = p.String("u")
			if err != nil {
				log.Warn(r, err)
				sendError(w, r, err)
				return
			}
		}

		version, err := p.String("v")
		if err != nil {
			log.Warn(r, err)
			sendError(w, r, err)
			return
		}
		client, err := p.String("c")
		if err != nil {
			log.Warn(r, err)
			sendError(w, r, err)
			return
		}

		ctx := r.Context()
		ctx = request.WithUsername(ctx, username)
		ctx = request.WithClient(ctx, client)
		ctx = request.WithVersion(ctx, version)
		if log.IsGreaterOrEqualTo(log.LevelDebug) {
			log.Debug(ctx, "API: New request", "path", r.URL.Path, "username", username, "client", client, "version", version)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func authenticate(ds model.DataStore) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			var usr *model.User
			var err error

			username, isInternalAuth := fromInternalOrProxyAuth(r)
			if username != "" {
				authType := If(isInternalAuth, "internal", "reverse-proxy")
				usr, err = ds.User(ctx).FindByUsername(username)
				if errors.Is(err, context.Canceled) {
					log.Debug(ctx, "API: Request canceled when authenticating", "auth", authType, "username", username, "remoteAddr", r.RemoteAddr, err)
					return
				}
				if errors.Is(err, model.ErrNotFound) {
					log.Warn(ctx, "API: Invalid login", "auth", authType, "username", username, "remoteAddr", r.RemoteAddr, err)
				} else if err != nil {
					log.Error(ctx, "API: Error authenticating username", "auth", authType, "username", username, "remoteAddr", r.RemoteAddr, err)
				}
			} else {
				p := req.Params(r)
				username, _ := request.UsernameFrom(ctx)
				if username == "" {
					username = p.StringOr("u", "")
				}
				pass := p.StringOr("p", "")
				token := p.StringOr("t", "")
				salt := p.StringOr("s", "")
				jwt := p.StringOr("jwt", "")

				usr, err = ds.User(ctx).FindByUsernameWithPassword(username)
				if errors.Is(err, context.Canceled) {
					log.Debug(ctx, "API: Request canceled when authenticating", "auth", "subsonic", "username", username, "remoteAddr", r.RemoteAddr, err)
					return
				}
				switch {
				case errors.Is(err, model.ErrNotFound):
					log.Warn(ctx, "API: Invalid login", "auth", "subsonic", "username", username, "remoteAddr", r.RemoteAddr, err)
				case err != nil:
					log.Error(ctx, "API: Error authenticating username", "auth", "subsonic", "username", username, "remoteAddr", r.RemoteAddr, err)
				default:
					err = validateCredentials(usr, pass, token, salt, jwt)
					if err != nil {
						log.Warn(ctx, "API: Invalid login", "auth", "subsonic", "username", username, "remoteAddr", r.RemoteAddr, err)
					}
				}
			}

			if err != nil {
				sendError(w, r, newError(responses.ErrorAuthenticationFail))
				return
			}

			ctx = request.WithUser(ctx, *usr)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loggedUser, ok := request.UserFrom(r.Context())
		if !ok {
			sendError(w, r, newError(responses.ErrorGeneric, "Internal error"))
			return
		}

		if !loggedUser.IsAdmin {
			sendError(w, r, newError(responses.ErrorAuthorizationFail))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func validateCredentials(user *model.User, pass, token, salt, jwt string) error {
	valid := false

	switch {
	case jwt != "":
		claims, err := auth.Validate(jwt)
		valid = err == nil && claims.Subject == user.UserName
	case pass != "":
		if strings.HasPrefix(pass, "enc:") {
			if dec, err := hex.DecodeString(pass[4:]); err == nil {
				pass = string(dec)
			}
		}
		valid = pass == user.Password
	case token != "":
		sum := md5.Sum([]byte(user.Password + salt))
		valid = equalMD5Hex(token, sum)
	}

	if !valid {
		return model.ErrInvalidAuth
	}
	return nil
}

func equalMD5Hex(token string, sum [md5.Size]byte) bool {
	if len(token) != md5.Size*2 {
		return false
	}

	var encoded [md5.Size * 2]byte
	hex.Encode(encoded[:], sum[:])
	var diff byte
	for i := range encoded {
		diff |= token[i] ^ encoded[i]
	}
	return diff == 0
}

func getPlayer(players core.Players) func(next http.Handler) http.Handler {
	return getPlayerWithLookupMode(players, false, false)
}

func getFreshPlayer(players core.Players) func(next http.Handler) http.Handler {
	return getPlayerWithLookupMode(players, true, false)
}

func getStreamPlayer(players core.Players) func(next http.Handler) http.Handler {
	return getPlayerWithLookupMode(players, true, true)
}

func getPlayerWithLookupMode(players core.Players, fresh, cacheRawStream bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			userName, _ := request.UsernameFrom(ctx)
			client, _ := request.ClientFrom(ctx)
			cookieName := playerIDCookieName(userName)
			playerId := playerIDFromCookie(r, cookieName)
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			userAgent := canonicalUserAgent(r)
			var player *model.Player
			var trc *model.Transcoding
			var err error
			useFresh := fresh
			if cacheRawStream && req.Params(r).StringOr("format", "") == "raw" {
				useFresh = false
			}
			if useFresh {
				player, trc, err = players.RegisterFresh(ctx, playerId, client, userAgent, ip)
			} else {
				player, trc, err = players.Register(ctx, playerId, client, userAgent, ip)
			}
			if err != nil {
				log.Error(ctx, "Could not register player", "username", userName, "client", client, err)
			} else {
				ctx = request.WithPlayer(ctx, *player)
				if trc != nil {
					ctx = request.WithTranscoding(ctx, *trc)
				}
				r = r.WithContext(ctx)

				if shouldSetPlayerCookie(r, cookieName, player.ID) {
					cookie := &http.Cookie{ //nolint:gosec // Secure omitted: Navidrome may run over plain HTTP
						Name:     cookieName,
						Value:    player.ID,
						MaxAge:   consts.CookieExpiry,
						HttpOnly: true,
						SameSite: http.SameSiteStrictMode,
						Path:     cmp.Or(conf.Server.BasePath, "/"),
					}
					http.SetCookie(w, cookie)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

const canonicalUserAgentCacheLimit = 128

var (
	canonicalUserAgentCacheMu sync.RWMutex
	canonicalUserAgentCache   = make(map[string]string, canonicalUserAgentCacheLimit)
)

func canonicalUserAgent(r *http.Request) string {
	raw := r.Header.Get("user-agent")
	if raw == "" {
		return ""
	}
	canonicalUserAgentCacheMu.RLock()
	cached, ok := canonicalUserAgentCache[raw]
	canonicalUserAgentCacheMu.RUnlock()
	if ok {
		return cached
	}

	u := ua.Parse(raw)
	userAgent := u.Name
	if u.OS != "" {
		userAgent = userAgent + "/" + u.OS
	}

	canonicalUserAgentCacheMu.Lock()
	if cached, ok := canonicalUserAgentCache[raw]; ok {
		canonicalUserAgentCacheMu.Unlock()
		return cached
	}
	if len(canonicalUserAgentCache) >= canonicalUserAgentCacheLimit {
		canonicalUserAgentCache = make(map[string]string, canonicalUserAgentCacheLimit)
	}
	canonicalUserAgentCache[raw] = userAgent
	canonicalUserAgentCacheMu.Unlock()
	return userAgent
}

func playerIDFromCookie(r *http.Request, cookieName string) string {
	if r.Header.Get("Cookie") == "" {
		return ""
	}
	var playerId string
	if c, err := r.Cookie(cookieName); err == nil {
		playerId = c.Value
		if log.IsGreaterOrEqualTo(log.LevelTrace) {
			log.Trace(r, "playerId found in cookies", "playerId", playerId)
		}
	}
	return playerId
}

func shouldSetPlayerCookie(r *http.Request, cookieName, playerID string) bool {
	if r.Header.Get("Cookie") == "" {
		return true
	}
	c, err := r.Cookie(cookieName)
	return err != nil || c.Value != playerID
}

func playerIDCookieName(userName string) string {
	const prefix = "nd-player-"
	const hextable = "0123456789abcdef"
	cookieName := make([]byte, len(prefix)+len(userName)*2)
	copy(cookieName, prefix)
	pos := len(prefix)
	for i := 0; i < len(userName); i++ {
		c := userName[i]
		cookieName[pos] = hextable[c>>4]
		cookieName[pos+1] = hextable[c&0x0f]
		pos += 2
	}
	return string(cookieName)
}

type contextKey string

const subsonicErrorPointer contextKey = "subsonicErrorPointer"

func recordStats(metrics metrics.Metrics) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			status := int32(-1)
			contextWithStatus := context.WithValue(r.Context(), subsonicErrorPointer, &status)

			start := time.Now()
			defer func() {
				elapsed := time.Since(start).Milliseconds()

				// We want to get the client name (even if not present for certain endpoints)
				p := req.Params(r)
				client, _ := p.String("c")

				// If there is no Subsonic status (e.g., HTTP 501 not implemented), fallback to HTTP
				if status == -1 {
					status = int32(ww.Status())
				}

				shortPath := strings.Replace(r.URL.Path, ".view", "", 1)

				metrics.RecordRequest(r.Context(), shortPath, r.Method, client, status, elapsed)
			}()

			next.ServeHTTP(ww, r.WithContext(contextWithStatus))
		}
		return http.HandlerFunc(fn)
	}
}
