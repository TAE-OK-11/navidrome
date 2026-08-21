package artwork

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/external"
	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/resources"
)

func selectImageReader(ctx context.Context, artID model.ArtworkID, extractFuncs ...sourceFunc) (io.ReadCloser, string, error) {
	for _, f := range extractFuncs {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		start := time.Now()
		r, path, err := f()
		if r != nil {
			msg := fmt.Sprintf("Found %s artwork", artID.Kind)
			log.Debug(ctx, msg, "artID", artID, "path", path, "source", f, "elapsed", time.Since(start))
			return capImageReader(r), path, nil
		}
		log.Trace(ctx, "Failed trying to extract artwork", "artID", artID, "source", f, "elapsed", time.Since(start), err)
	}
	return nil, "", fmt.Errorf("could not get `%s` cover art for %s: %w", artID.Kind, artID, ErrUnavailable)
}

type sourceFunc func() (r io.ReadCloser, path string, err error)

func (f sourceFunc) String() string {
	name := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	name = strings.TrimPrefix(name, "github.com/navidrome/navidrome/core/artwork.")
	if _, after, found := strings.Cut(name, ")."); found {
		name = after
	}
	name = strings.TrimSuffix(name, ".func1")
	return name
}

func fromExternalFile(ctx context.Context, lib libraryView, files []string, pattern string) sourceFunc {
	return func() (io.ReadCloser, string, error) {
		for _, file := range files {
			_, name := filepath.Split(file)
			match, err := filepath.Match(pattern, strings.ToLower(name))
			if err != nil {
				log.Warn(ctx, "Error matching cover art file to pattern", "pattern", pattern, "file", file)
				continue
			}
			if !match {
				continue
			}
			f, err := lib.OpenArtwork(file)
			if err != nil {
				log.Warn(ctx, "Could not open cover art file", "file", file, err)
				continue
			}
			return f, file, nil
		}
		return nil, "", fmt.Errorf("pattern '%s' not matched by files %v", pattern, files)
	}
}

// fromFFmpegTag is intentionally absolute-path-based. ffmpeg is a subprocess
// and cannot read from arbitrary fs.FS implementations; piping via stdin is a
// non-trivial refactor with stream/seek implications.
//
// TODO(artwork-musicfs): when the storage backing the library is not local
// (e.g. a future S3 backend, or FakeFS in tests), short-circuit this source
// func to return (nil, "", nil) so callers fall through cleanly.
func fromFFmpegTag(ctx context.Context, ffmpeg ffmpeg.FFmpeg, path string) sourceFunc {
	return fromFFmpegTagNamed(ctx, ffmpeg, path, path)
}

// fromFFmpegTagNamed lets callers use an absolute path for the ffmpeg process
// while preserving the library-relative source identifier used by artwork
// caching, diagnostics and callers.
func fromFFmpegTagNamed(ctx context.Context, ffmpeg ffmpeg.FFmpeg, path, sourcePath string) sourceFunc {
	return func() (io.ReadCloser, string, error) {
		if path == "" {
			return nil, "", nil
		}
		r, err := ffmpeg.ExtractImage(ctx, path)
		if err != nil {
			return nil, "", err
		}
		// Validate that the stream actually contains image data by reading the first byte.
		// ffmpeg.ExtractImage returns a pipe reader that may fail asynchronously if the
		// file has no video/image stream (e.g., an MP3 without embedded art).
		buf := make([]byte, 1)
		n, err := r.Read(buf)
		if n == 0 || err != nil {
			r.Close()
			return nil, "", fmt.Errorf("ffmpeg produced no image data for %s: %w", path, err)
		}
		return readCloser{Reader: io.MultiReader(bytes.NewReader(buf[:n]), r), Closer: r}, sourcePath, nil
	}
}

// readCloser combines a Reader and a Closer into an io.ReadCloser.
type readCloser struct {
	io.Reader
	io.Closer
}

func fromAlbum(ctx context.Context, a *artwork, id model.ArtworkID) sourceFunc {
	return func() (io.ReadCloser, string, error) {
		r, _, err := a.Get(ctx, id, 0, false)
		if err != nil {
			return nil, "", err
		}
		return r, id.String(), nil
	}
}

func fromAlbumPlaceholder() sourceFunc {
	return func() (io.ReadCloser, string, error) {
		r, _ := resources.FS().Open(consts.PlaceholderAlbumArt)
		return r, consts.PlaceholderAlbumArt, nil
	}
}
func fromArtistExternalSource(ctx context.Context, ar model.Artist, provider external.Provider) sourceFunc {
	return func() (io.ReadCloser, string, error) {
		imageUrl, err := provider.ArtistImage(ctx, ar.ID)
		if err != nil {
			return nil, "", err
		}

		return fromURL(ctx, imageUrl)
	}
}

func fromAlbumExternalSource(ctx context.Context, al model.Album, provider external.Provider) sourceFunc {
	return func() (io.ReadCloser, string, error) {
		imageUrl, err := provider.AlbumImage(ctx, al.ID)
		if err != nil {
			return nil, "", err
		}

		return fromURL(ctx, imageUrl)
	}
}

func fromURL(ctx context.Context, imageUrl *url.URL) (io.ReadCloser, string, error) {
	hc := newArtworkHTTPClient()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, imageUrl.String(), nil)
	req.Header.Set("User-Agent", consts.HTTPUserAgent)
	resp, err := hc.Do(req) //nolint:gosec
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("error retrieving artwork from %s: %s", imageUrl, resp.Status)
	}
	body, err := boundedArtworkResponse(resp)
	if err != nil {
		resp.Body.Close()
		return nil, "", err
	}
	return body, imageUrl.String(), nil
}
