package nativeapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/deluan/rest"
	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/core/apikeys"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
)

type createAPIKeyRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

type createAPIKeyResponse struct {
	Key   model.UserAPIKey `json:"key"`
	Token string           `json:"token"`
}

func (api *Router) addAPIKeyRoute(r chi.Router) {
	service := apikeys.New(api.ds)
	r.Route("/apikey", func(r chi.Router) {
		r.Get("/", listAPIKeys(service))
		r.Post("/", createAPIKey(service))
		r.Delete("/{id}", deleteAPIKey(service))
	})
}

func listAPIKeys(service *apikeys.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := request.UserFrom(r.Context())
		if !ok {
			_ = rest.RespondWithError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		keys, err := service.List(r.Context(), user.ID)
		if err != nil {
			_ = rest.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = rest.RespondWithJSON(w, http.StatusOK, keys)
	}
}

func createAPIKey(service *apikeys.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := request.UserFrom(r.Context())
		if !ok {
			_ = rest.RespondWithError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		var payload createAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			_ = rest.RespondWithError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		key, token, err := service.Create(r.Context(), user.ID, apikeys.CreateInput{
			Name:      payload.Name,
			ExpiresAt: payload.ExpiresAt,
		})
		if err != nil {
			_ = rest.RespondWithError(w, http.StatusBadRequest, err.Error())
			return
		}
		_ = rest.RespondWithJSON(w, http.StatusOK, createAPIKeyResponse{Key: *key, Token: token})
	}
}

func deleteAPIKey(service *apikeys.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := request.UserFrom(r.Context())
		if !ok {
			_ = rest.RespondWithError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		id := chi.URLParam(r, "id")
		if err := service.Delete(r.Context(), user.ID, id); err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, model.ErrNotAuthorized):
				status = http.StatusForbidden
			case errors.Is(err, model.ErrNotFound):
				status = http.StatusNotFound
			}
			_ = rest.RespondWithError(w, status, err.Error())
			return
		}
		_ = rest.RespondWithJSON(w, http.StatusOK, map[string]string{"id": id})
	}
}
