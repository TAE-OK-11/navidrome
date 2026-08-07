package subsonic

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/stream"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server/subsonic/responses"
	"github.com/navidrome/navidrome/utils/req"
)

func (api *Router) Stream(w http.ResponseWriter, r *http.Request) (*responses.Subsonic, error) {
	ctx := r.Context()
	p := req.Params(r)
	id, err := p.String("id")
	if err != nil {
		return nil, err
	}
	maxBitRate := p.IntOr("maxBitRate", 0)
	format, _ := p.String("format")
	timeOffset := p.IntOr("timeOffset", 0)

	mf, err := api.mediaFileForStreaming(ctx, id)
	if err != nil {
		return nil, err
	}

	streamReq := api.transcodeDecision.ResolveRequest(ctx, mf, format, maxBitRate, timeOffset)
	stream, err := api.streamer.NewStream(ctx, mf, streamReq)
	if err != nil {
		return nil, err
	}
	stream.TrackPlayback()

	// Make sure the stream will be closed at the end, to avoid leakage
	defer func() {
		if err := stream.Close(); err != nil && log.IsGreaterOrEqualTo(log.LevelDebug) {
			log.Error("Error closing stream", "id", id, "file", stream.Name(), err)
		}
	}()

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Content-Duration", strconv.FormatFloat(float64(stream.Duration()), 'G', -1, 32))

	_, err = stream.Serve(ctx, w, r)
	return nil, err
}

func (api *Router) Download(w http.ResponseWriter, r *http.Request) (*responses.Subsonic, error) {
	ctx := r.Context()
	username, _ := request.UsernameFrom(ctx)
	p := req.Params(r)
	id, err := p.String("id")
	if err != nil {
		return nil, err
	}

	if !conf.Server.EnableDownloads {
		log.Warn(ctx, "Downloads are disabled", "user", username, "id", id)
		return nil, newError(responses.ErrorAuthorizationFail, "downloads are disabled")
	}

	entity, err := model.GetEntityByID(ctx, api.ds, id)
	if err != nil {
		return nil, err
	}

	maxBitRate := p.IntOr("bitrate", 0)
	format, _ := p.String("format")

	if format == "" {
		if conf.Server.AutoTranscodeDownload {
			// if we are not provided a format, see if we have requested transcoding for this client
			// This must be enabled via a config option. For the UI, we are always given an option.
			// This will impact other clients which do not use the UI
			transcoding, ok := request.TranscodingFrom(ctx)

			if !ok {
				format = "raw"
			} else {
				format = transcoding.TargetFormat
				maxBitRate = transcoding.DefaultBitRate
			}
		} else {
			format = "raw"
		}
	}

	setHeaders := func(name string) {
		w.Header().Set("Content-Disposition", attachmentDisposition(name+".zip"))
		w.Header().Set("Content-Type", "application/zip")
	}

	writeArchive := func(fn func(http.ResponseWriter) error) error {
		tracked := &archiveResponseWriter{ResponseWriter: w}
		err := fn(tracked)
		return handleArchiveErr(ctx, id, tracked.wrote, err)
	}

	switch v := entity.(type) {
	case *model.MediaFile:
		streamReq := api.transcodeDecision.ResolveRequest(ctx, v, format, maxBitRate, 0)
		stream, err := api.streamer.NewStream(ctx, v, streamReq)
		if err != nil {
			return nil, err
		}

		// Make sure the stream will be closed at the end, to avoid leakage
		defer func() {
			if err := stream.Close(); err != nil && log.IsGreaterOrEqualTo(log.LevelDebug) {
				log.Error("Error closing stream", "id", id, "file", stream.Name(), err)
			}
		}()

		w.Header().Set("Content-Disposition", attachmentDisposition(stream.Name()))

		_, err = stream.Serve(ctx, w, r)
		return nil, err
	case *model.Album:
		setHeaders(v.Name)
		return nil, writeArchive(func(out http.ResponseWriter) error {
			return api.archiver.ZipAlbum(ctx, id, format, maxBitRate, out)
		})
	case *model.Artist:
		setHeaders(v.Name)
		return nil, writeArchive(func(out http.ResponseWriter) error {
			return api.archiver.ZipArtist(ctx, id, format, maxBitRate, out)
		})
	case *model.Playlist:
		setHeaders(v.Name)
		return nil, writeArchive(func(out http.ResponseWriter) error {
			return api.archiver.ZipPlaylist(ctx, id, format, maxBitRate, out)
		})
	default:
		return nil, model.ErrNotFound
	}
}

func attachmentDisposition(name string) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', 0:
			return '_'
		default:
			return r
		}
	}, name)
	if disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name}); disposition != "" {
		return disposition
	}
	return `attachment; filename="download"`
}

type archiveResponseWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *archiveResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if n > 0 {
		w.wrote = true
	}
	return n, err
}

// handleArchiveErr only forwards an archive error while the outer HTTP error
// handler can still write a coherent status/body. Once archive bytes have been
// sent, appending a JSON/XML error would corrupt the ZIP even further, so log the
// failure and leave the already-started response alone.
func handleArchiveErr(ctx context.Context, id string, wrote bool, err error) error {
	if err == nil {
		return nil
	}
	if !wrote {
		return err
	}
	if errors.Is(err, stream.ErrTooManyTranscodes) {
		log.Warn(ctx, "Archive download finalized early: transcode cap reached", "id", id, err)
	} else {
		log.Error(ctx, "Archive download failed after response started", "id", id, err)
	}
	return nil
}
