package public

import (
	"cmp"
	"mime"
	"net/http"
	"strings"

	"github.com/navidrome/navidrome/core"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/utils/req"
	"github.com/navidrome/navidrome/utils/str"
)

func (pub *Router) handleDownloads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := req.Params(r).String(":id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Resolve all errors before the archive writes its first byte. Otherwise the
	// response status is already committed as 200 and expired, missing or
	// non-downloadable shares are reported incorrectly.
	s, err := pub.share.Load(ctx, id)
	if err != nil {
		checkShareError(ctx, w, err, id)
		return
	}
	if !s.Downloadable {
		checkShareError(ctx, w, model.ErrNotAuthorized, id)
		return
	}

	name := str.SanitizeFilename(cmp.Or(s.Description, s.ID))
	name = strings.ReplaceAll(name, ",", "_")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": name + ".zip",
	}))
	w.Header().Set("Content-Type", "application/zip")

	// Share.Load records the visit. Pass the loaded object through the context so
	// the archiver does not load it again and double-count the download.
	ctx = core.WithLoadedShare(ctx, s)
	err = pub.archiver.ZipShare(ctx, id, w)
	checkShareError(ctx, w, err, id)
}
