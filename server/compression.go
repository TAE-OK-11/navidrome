package server

import (
	"compress/gzip"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

const (
	minCompressedResponseSize       = 512
	largeCompressedResponseSize     = 16 << 10
	compressionDecisionBufferTarget = largeCompressedResponseSize
)

type compressionEncoding string

const (
	compressionBrotli compressionEncoding = "br"
	compressionZstd   compressionEncoding = "zstd"
	compressionGzip   compressionEncoding = "gzip"
)

func compressMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodHead || r.Header.Get("Range") != "" {
				next.ServeHTTP(w, r)
				return
			}
			acceptEncoding := r.Header.Get("Accept-Encoding")
			if acceptEncoding == "" {
				next.ServeHTTP(w, r)
				return
			}
			if isMediaResponsePath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			accepted := acceptedCompressionEncodings(acceptEncoding)
			if !accepted.hasAny() {
				next.ServeHTTP(w, r)
				return
			}

			cw := &compressResponseWriter{
				ResponseWriter: w,
				accepted:       accepted,
			}
			defer cw.Close()
			next.ServeHTTP(cw, r)
		})
	}
}

type acceptedCompressions struct {
	brotli bool
	zstd   bool
	gzip   bool
}

func (a acceptedCompressions) hasAny() bool {
	return a.brotli || a.zstd || a.gzip
}

func acceptedCompressionEncodings(acceptEncoding string) acceptedCompressions {
	return acceptedCompressions{
		brotli: acceptedEncodingQuality(acceptEncoding, string(compressionBrotli)) > 0,
		zstd:   acceptedEncodingQuality(acceptEncoding, string(compressionZstd)) > 0,
		gzip:   acceptedEncodingQuality(acceptEncoding, string(compressionGzip)) > 0,
	}
}

func acceptedEncodingQuality(header, encoding string) float64 {
	var wildcardQuality float64
	hasWildcard := false

	for part := range strings.SplitSeq(header, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		token = strings.TrimSpace(strings.ToLower(token))
		quality := encodingQuality(params)
		switch token {
		case encoding:
			return quality
		case "*":
			wildcardQuality = quality
			hasWildcard = true
		}
	}
	if hasWildcard {
		return wildcardQuality
	}
	return 0
}

func encodingQuality(params string) float64 {
	if params == "" {
		return 1
	}
	for param := range strings.SplitSeq(params, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if !ok || !strings.EqualFold(key, "q") {
			continue
		}
		q, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0
		}
		return q
	}
	return 1
}

func isMediaResponsePath(path string) bool {
	path = strings.TrimSuffix(path, ".view")
	return strings.HasSuffix(path, "/rest/stream") ||
		strings.HasSuffix(path, "/rest/download") ||
		strings.HasSuffix(path, "/rest/getTranscodeStream") ||
		strings.HasSuffix(path, "/rest/getCoverArt") ||
		strings.HasSuffix(path, "/rest/getAvatar")
}

type compressResponseWriter struct {
	http.ResponseWriter
	accepted acceptedCompressions
	encoding compressionEncoding
	status   int
	writer   io.WriteCloser
	buffer   []byte
	raw      bool
	closed   bool
}

func (w *compressResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
}

func (w *compressResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.writer != nil {
		if _, err := w.writer.Write(p); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	if w.raw {
		n, err := w.ResponseWriter.Write(p)
		if err != nil {
			return n, err
		}
		return len(p), nil
	}

	w.buffer = append(w.buffer, p...)
	if len(w.buffer) < compressionDecisionBufferTarget && !hasSmallContentLength(w.Header()) {
		return len(p), nil
	}
	if err := w.flushBuffered(); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *compressResponseWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if w.writer == nil && !w.raw {
		if err := w.flushBuffered(); err != nil {
			return err
		}
	}
	if w.writer != nil {
		return w.writer.Close()
	}
	return nil
}

func (w *compressResponseWriter) Flush() {
	_ = w.flushBuffered()
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *compressResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *compressResponseWriter) flushBuffered() error {
	if w.writer != nil || w.raw {
		return nil
	}
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}

	if !shouldCompressResponse(status, w.Header(), w.buffer) {
		w.raw = true
		w.ResponseWriter.WriteHeader(status)
		if len(w.buffer) == 0 {
			return nil
		}
		_, err := w.ResponseWriter.Write(w.buffer)
		w.buffer = nil
		return err
	}

	w.encoding = selectCompressionEncoding(w.accepted, len(w.buffer))
	if w.encoding == "" {
		w.raw = true
		w.ResponseWriter.WriteHeader(status)
		if len(w.buffer) == 0 {
			return nil
		}
		_, err := w.ResponseWriter.Write(w.buffer)
		w.buffer = nil
		return err
	}

	setCompressionHeaders(w.Header(), w.encoding)
	w.ResponseWriter.WriteHeader(status)

	writer, err := newCompressionWriter(w.ResponseWriter, w.encoding)
	if err != nil {
		return err
	}
	w.writer = writer
	if len(w.buffer) == 0 {
		return nil
	}
	_, err = w.writer.Write(w.buffer)
	w.buffer = nil
	return err
}

func shouldCompressResponse(status int, h http.Header, body []byte) bool {
	if status < http.StatusOK || status == http.StatusNoContent || status == http.StatusNotModified {
		return false
	}
	if h.Get("Content-Encoding") != "" || strings.Contains(strings.ToLower(h.Get("Cache-Control")), "no-transform") {
		return false
	}
	if hasSmallContentLength(h) || len(body) < minCompressedResponseSize {
		return false
	}
	contentType := h.Get("Content-Type")
	if contentType == "" && len(body) > 0 {
		contentType = http.DetectContentType(body)
		h.Set("Content-Type", contentType)
	}
	return isCompressibleContentType(contentType)
}

func selectCompressionEncoding(accepted acceptedCompressions, bodySize int) compressionEncoding {
	if bodySize >= largeCompressedResponseSize {
		if accepted.brotli {
			return compressionBrotli
		}
		if accepted.zstd {
			return compressionZstd
		}
		if accepted.gzip {
			return compressionGzip
		}
		return ""
	}

	if accepted.zstd {
		return compressionZstd
	}
	if accepted.gzip {
		return compressionGzip
	}
	if accepted.brotli {
		return compressionBrotli
	}
	return ""
}

func hasSmallContentLength(h http.Header) bool {
	contentLength := h.Get("Content-Length")
	if contentLength == "" {
		return false
	}
	n, err := strconv.Atoi(contentLength)
	return err == nil && n < minCompressedResponseSize
}

func isCompressibleContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType, _, _ = strings.Cut(contentType, ";")
		mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	}
	if strings.HasPrefix(mediaType, "text/") {
		return mediaType != "text/event-stream"
	}
	switch mediaType {
	case "application/json",
		"application/xml",
		"application/javascript",
		"application/x-javascript",
		"application/manifest+json",
		"application/problem+json",
		"image/svg+xml":
		return true
	default:
		return strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml")
	}
}

func setCompressionHeaders(h http.Header, encoding compressionEncoding) {
	h.Set("Content-Encoding", string(encoding))
	h.Del("Content-Length")
	addVaryAcceptEncoding(h)
}

func addVaryAcceptEncoding(h http.Header) {
	for part := range strings.SplitSeq(h.Get("Vary"), ",") {
		if strings.EqualFold(strings.TrimSpace(part), "Accept-Encoding") {
			return
		}
	}
	h.Add("Vary", "Accept-Encoding")
}

func newCompressionWriter(w io.Writer, encoding compressionEncoding) (io.WriteCloser, error) {
	switch encoding {
	case compressionBrotli:
		return brotli.NewWriterLevel(w, 3), nil
	case compressionZstd:
		return zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedFastest), zstd.WithEncoderConcurrency(1))
	case compressionGzip:
		return gzip.NewWriterLevel(w, gzip.BestSpeed)
	default:
		return nil, http.ErrNotSupported
	}
}
