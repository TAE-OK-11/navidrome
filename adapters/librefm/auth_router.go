package librefm

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/deluan/rest"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/agents"
	"github.com/navidrome/navidrome/core/integration"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server"
	"github.com/navidrome/navidrome/utils/req"
)

//go:embed token_received.html
var tokenReceivedPage []byte

const maxLinkRequestBodySize = 8 << 10

type Router struct {
	http.Handler
	ds          model.DataStore
	sessionKeys *agents.SessionKeys
	client      *client
	apiKey      string
	secret      string
	authURL     string
}

func NewRouter(ds model.DataStore) *Router {
	r := &Router{
		ds:          ds,
		apiKey:      effectiveApiKey(),
		secret:      effectiveSecret(),
		authURL:     conf.Server.LibreFM.AuthURL,
		sessionKeys: &agents.SessionKeys{DataStore: ds, KeyName: sessionKeyProperty},
	}
	r.Handler = r.routes()
	hc := integration.HTTPClient(consts.DefaultHttpClientTimeOut)
	r.client = newClient(r.apiKey, r.secret, conf.Server.LibreFM.BaseURL, hc)
	return r
}

func (s *Router) routes() http.Handler {
	r := chi.NewRouter()

	r.Group(func(r chi.Router) {
		r.Use(server.Authenticator(s.ds))
		r.Use(server.JWTRefresher)

		r.Get("/link", s.getLinkStatus)
		r.Put("/link", s.link)
		r.Delete("/link", s.unlink)
	})

	r.Get("/link/callback", s.callback)

	return r
}

func (s *Router) getLinkStatus(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"apiKey":  s.apiKey,
		"authUrl": s.authURL,
	}
	u, _ := request.UserFrom(r.Context())
	key, err := s.sessionKeys.Get(r.Context(), u.ID)
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		resp["error"] = err
		resp["status"] = false
		_ = rest.RespondWithJSON(w, http.StatusInternalServerError, resp)
		return
	}
	resp["status"] = key != ""
	linkToken, err := createLinkToken(u.ID)
	if err != nil {
		log.Error(r.Context(), "Could not create Libre.fm link token", "userId", u.ID, err)
		_ = rest.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp["linkToken"] = linkToken
	_ = rest.RespondWithJSON(w, http.StatusOK, resp)
}

func (s *Router) link(w http.ResponseWriter, r *http.Request) {
	type sessionPayload struct {
		SessionKey string `json:"sessionKey"`
	}
	var payload sessionPayload
	r.Body = http.MaxBytesReader(w, r.Body, maxLinkRequestBodySize)
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			_ = rest.RespondWithError(w, http.StatusRequestEntityTooLarge, "Request body too large")
			return
		}
		_ = rest.RespondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	if payload.SessionKey == "" {
		_ = rest.RespondWithError(w, http.StatusBadRequest, "Session key is required")
		return
	}

	u, _ := request.UserFrom(r.Context())
	username, err := s.client.validateSessionKey(r.Context(), payload.SessionKey)
	if err != nil {
		var retryLater *agents.RetryLaterError
		if errors.As(err, &retryLater) {
			log.Warn(r.Context(), "Libre.fm session key validation rate-limited", "userId", u.ID, err)
			_ = rest.RespondWithError(w, http.StatusServiceUnavailable, "Libre.fm is temporarily unavailable. Please try again later.")
			return
		}
		var lfErr *libreFMError
		if errors.As(err, &lfErr) && (lfErr.Code == 9 || lfErr.Code == 4) {
			_ = rest.RespondWithError(w, http.StatusBadRequest, "Invalid session key")
			return
		}
		log.Error(r.Context(), "Could not validate Libre.fm session key", "userId", u.ID, "requestId", middleware.GetReqID(r.Context()), err)
		_ = rest.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	err = s.sessionKeys.Put(r.Context(), u.ID, payload.SessionKey)
	if err != nil {
		log.Error("Could not save Libre.fm session key", "userId", u.ID, "requestId", middleware.GetReqID(r.Context()), err)
		_ = rest.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_ = rest.RespondWithJSON(w, http.StatusOK, map[string]any{"status": true, "user": username})
}

func (s *Router) unlink(w http.ResponseWriter, r *http.Request) {
	u, _ := request.UserFrom(r.Context())
	err := s.sessionKeys.Delete(r.Context(), u.ID)
	if err != nil {
		_ = rest.RespondWithError(w, http.StatusInternalServerError, err.Error())
	} else {
		_ = rest.RespondWithJSON(w, http.StatusOK, map[string]string{})
	}
}

func (s *Router) callback(w http.ResponseWriter, r *http.Request) {
	p := req.Params(r)
	token, err := p.String("token")
	if err != nil {
		_ = rest.RespondWithError(w, http.StatusBadRequest, "token not received")
		return
	}
	linkToken, err := p.String("uid")
	if err != nil {
		_ = rest.RespondWithError(w, http.StatusBadRequest, "uid not received")
		return
	}
	uid, err := verifyLinkToken(linkToken)
	if err != nil {
		log.Warn(r.Context(), "Rejected Libre.fm callback with invalid link token", "requestId", middleware.GetReqID(r.Context()), err)
		_ = rest.RespondWithError(w, http.StatusBadRequest, "invalid link token")
		return
	}

	ctx := request.WithUser(r.Context(), model.User{ID: uid})
	err = s.fetchSessionKey(ctx, uid, token)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("An error occurred while authorizing with Libre.fm. \n\nRequest ID: " + middleware.GetReqID(ctx))) //nolint:gosec
		return
	}

	http.ServeContent(w, r, "response", time.Now(), bytes.NewReader(tokenReceivedPage))
}

func (s *Router) fetchSessionKey(ctx context.Context, uid, token string) error {
	sessionKey, err := s.client.getSession(ctx, token)
	if err != nil {
		log.Error(ctx, "Could not fetch Libre.fm session key", "userId", uid,
			"requestId", middleware.GetReqID(ctx), err)
		return err
	}
	err = s.sessionKeys.Put(ctx, uid, sessionKey)
	if err != nil {
		log.Error("Could not save Libre.fm session key", "userId", uid, "requestId", middleware.GetReqID(ctx), err)
	}
	return err
}
