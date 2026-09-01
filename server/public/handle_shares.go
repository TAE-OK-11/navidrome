package public

import (
	"context"
	"errors"
	"net/http"
	"path"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/core/publicurl"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server"
	"github.com/navidrome/navidrome/ui"
	. "github.com/navidrome/navidrome/utils/gg"
	"github.com/navidrome/navidrome/utils/req"
)

func (pub *Router) handleShares(w http.ResponseWriter, r *http.Request) {
	id, err := req.Params(r).String(":id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If requested file is a UI asset, just serve it
	_, err = ui.BuildAssets().Open(id)
	if err == nil {
		pub.assetsHandler.ServeHTTP(w, r)
		return
	}

	// If it is not, consider it a share ID
	s, err := pub.share.Load(r.Context(), id)
	if err != nil {
		checkShareError(r.Context(), w, err, id)
		return
	}

	s, err = pub.mapShareInfo(r, *s)
	if err != nil {
		log.Error(r.Context(), "Error preparing share", "id", id, err)
		http.Error(w, "Error preparing share", http.StatusInternalServerError)
		return
	}
	server.IndexWithShare(pub.ds, ui.BuildAssets(), s)(w, r)
}

func (pub *Router) handleM3U(w http.ResponseWriter, r *http.Request) {
	id, err := req.Params(r).String(":id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If it is not, consider it a share ID
	s, err := pub.share.Load(r.Context(), id)
	if err != nil {
		checkShareError(r.Context(), w, err, id)
		return
	}

	s, err = pub.mapShareToM3U(r, *s)
	if err != nil {
		log.Error(r.Context(), "Error preparing share playlist", "id", id, err)
		http.Error(w, "Error preparing share playlist", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "audio/x-mpegurl")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(s.ToM3U8())) //nolint:gosec
}

func checkShareError(ctx context.Context, w http.ResponseWriter, err error, id string) {
	switch {
	case errors.Is(err, model.ErrExpired):
		log.Error(ctx, "Share expired", "id", id, err)
		http.Error(w, "Share not available anymore", http.StatusGone)
	case errors.Is(err, model.ErrNotFound):
		log.Error(ctx, "Share not found", "id", id, err)
		http.Error(w, "Share not found", http.StatusNotFound)
	case errors.Is(err, model.ErrNotAuthorized):
		log.Error(ctx, "Share is not downloadable", "id", id, err)
		http.Error(w, "This share is not downloadable", http.StatusForbidden)
	case err != nil:
		log.Error(ctx, "Error retrieving share", "id", id, err)
		http.Error(w, "Error retrieving share", http.StatusInternalServerError)
	}
}

func (pub *Router) mapShareInfo(r *http.Request, s model.Share) (*model.Share, error) {
	s.URL = ShareURL(r, s.ID)
	s.ImageURL = publicurl.ImageURL(r, s.CoverArtID(), conf.Server.UICoverArtSize)
	for i := range s.Tracks {
		id, err := encodeMediafileShare(s, s.Tracks[i].ID)
		if err != nil {
			return nil, err
		}
		s.Tracks[i].ID = id
	}
	return &s, nil
}

func (pub *Router) mapShareToM3U(r *http.Request, s model.Share) (*model.Share, error) {
	for i := range s.Tracks {
		id, err := encodeMediafileShare(s, s.Tracks[i].ID)
		if err != nil {
			return nil, err
		}
		s.Tracks[i].Path = publicurl.PublicURL(r, path.Join(consts.URLPathPublic, "s", id), nil)
	}
	return &s, nil
}

// encodeMediafileShare builds the signed token embedded in a public share link
// for a single track.
//
// NOTE ON JWT USAGE: This is deliberately NOT part of Navidrome's authentication.
// The token is a signed, opaque capability that identifies one shared track
// (plus its transcode format/bitrate and the parent share id). We use a JWT here
// (reusing the library we already have) because it is a simple way to get three
// properties for a public link: the embedded ids can't be enumerated by guessing,
// the signature
// makes the claims tamper-evident, and the self-contained exp lets us reject
// stale links without a DB lookup. It carries no user identity (no subject, no
// admin flag) and grants access to nothing beyond the share it belongs to; the
// stream handler still verifies the share exists, is unexpired, and that the
// track is actually a member of it. An attacker who can forge these tokens
// necessarily already holds the signing secret, which also signs real user
// sessions, so that scenario is out of scope for the share boundary specifically.
func encodeMediafileShare(s model.Share, id string) (string, error) {
	claims := auth.Claims{
		ID:      id,
		Format:  s.Format,
		BitRate: s.MaxBitRate,
		ShareID: s.ID,
	}
	return auth.CreateExpiringPublicToken(V(s.ExpiresAt), claims)
}
