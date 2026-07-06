package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

const (
	generalAPICompressedMinSize     = 1024
	largeAPICompressedMinSize       = 2048
	hugeAPICompressedMinSize        = 4096
	lyricsCompressedMinSize         = 256
	webUICompressedMinSize          = 1024
	largeCompressedResponseSize     = 16 << 10
	hugeCompressedResponseSize      = 256 << 10
	compressionDecisionBufferTarget = largeCompressedResponseSize
	brotliLargeLevel                = 5
	brotliHugeLevel                 = 6
	zstdGeneralLevel                = 3
	gzipFallbackLevel               = 4
)

type compressionEncoding string

const (
	compressionBrotli compressionEncoding = "br"
	compressionZstd   compressionEncoding = "zstd"
	compressionGzip   compressionEncoding = "gzip"
)

var (
	compressionBufferPool = sync.Pool{
		New: func() any {
			return make([]byte, 0, compressionDecisionBufferTarget)
		},
	}
	brotliLargeWriterPool sync.Pool
	brotliHugeWriterPool  sync.Pool
	zstdGeneralWriterPool sync.Pool
	gzipFallbackPool      sync.Pool
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
				path:           r.URL.Path,
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

type compressionProfile struct {
	encoding compressionEncoding
	minSize  int
	level    int
}

func (a acceptedCompressions) hasAny() bool {
	return a.brotli || a.zstd || a.gzip
}

func acceptedCompressionEncodings(acceptEncoding string) acceptedCompressions {
	if !strings.ContainsAny(acceptEncoding, ";*") {
		return acceptedCompressionEncodingsFast(acceptEncoding)
	}
	return acceptedCompressionEncodingsSlow(acceptEncoding)
}

func acceptedCompressionEncodingsFast(acceptEncoding string) acceptedCompressions {
	var accepted acceptedCompressions
	for part := range strings.SplitSeq(acceptEncoding, ",") {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case string(compressionBrotli):
			accepted.brotli = true
		case string(compressionZstd):
			accepted.zstd = true
		case string(compressionGzip):
			accepted.gzip = true
		}
	}
	return accepted
}

func acceptedCompressionEncodingsSlow(acceptEncoding string) acceptedCompressions {
	var accepted acceptedCompressions
	var brotliSet, gzipSet bool
	var wildcardQuality float64
	var wildcardSet bool

	for part := range strings.SplitSeq(acceptEncoding, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		token = strings.TrimSpace(strings.ToLower(token))
		quality := encodingQuality(params)
		switch token {
		case string(compressionBrotli):
			accepted.brotli = quality > 0
			brotliSet = true
		case string(compressionZstd):
			accepted.zstd = quality > 0
		case string(compressionGzip):
			accepted.gzip = quality > 0
			gzipSet = true
		case "*":
			wildcardQuality = quality
			wildcardSet = true
		}
	}

	if wildcardSet && wildcardQuality > 0 {
		if !brotliSet {
			accepted.brotli = true
		}
		if !gzipSet {
			accepted.gzip = true
		}
	}

	return accepted
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
	path     string
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

	if w.buffer == nil {
		w.buffer = getCompressionBuffer()
	}
	w.buffer = append(w.buffer, p...)
	if len(w.buffer) < compressionDecisionBufferTarget {
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

	contentType := responseContentType(w.Header(), w.buffer)
	if !isCompressibleResponse(status, w.Header(), contentType) {
		w.raw = true
		w.ResponseWriter.WriteHeader(status)
		if len(w.buffer) == 0 {
			return nil
		}
		buf := w.buffer
		_, err := w.ResponseWriter.Write(buf)
		w.releaseBuffer()
		return err
	}

	profile := selectCompressionProfile(w.accepted, w.path, w.Header(), contentType, len(w.buffer))
	if profile.encoding == "" || len(w.buffer) < profile.minSize {
		w.raw = true
		w.ResponseWriter.WriteHeader(status)
		if len(w.buffer) == 0 {
			return nil
		}
		buf := w.buffer
		_, err := w.ResponseWriter.Write(buf)
		w.releaseBuffer()
		return err
	}

	w.encoding = profile.encoding
	setCompressionHeaders(w.Header(), w.encoding)
	w.ResponseWriter.WriteHeader(status)

	writer, err := newCompressionWriter(w.ResponseWriter, profile)
	if err != nil {
		return err
	}
	w.writer = writer
	if len(w.buffer) == 0 {
		return nil
	}
	buf := w.buffer
	_, err = w.writer.Write(buf)
	w.releaseBuffer()
	return err
}

func getCompressionBuffer() []byte {
	return compressionBufferPool.Get().([]byte)[:0]
}

func (w *compressResponseWriter) releaseBuffer() {
	if w.buffer == nil {
		return
	}
	if cap(w.buffer) <= compressionDecisionBufferTarget*2 {
		compressionBufferPool.Put(w.buffer[:0])
	}
	w.buffer = nil
}

func isCompressibleResponse(status int, h http.Header, contentType string) bool {
	if status < http.StatusOK || status == http.StatusNoContent || status == http.StatusNotModified {
		return false
	}
	if h.Get("Content-Encoding") != "" || strings.Contains(strings.ToLower(h.Get("Cache-Control")), "no-transform") {
		return false
	}
	return isCompressibleContentType(contentType)
}

func responseContentType(h http.Header, body []byte) string {
	contentType := h.Get("Content-Type")
	if contentType == "" && len(body) > 0 {
		contentType = http.DetectContentType(body)
		h.Set("Content-Type", contentType)
	}
	return contentType
}

func selectCompressionProfile(accepted acceptedCompressions, path string, h http.Header, contentType string, bodySize int) compressionProfile {
	minSize := generalAPICompressedMinSize
	level := zstdGeneralLevel
	preferred := compressionZstd

	switch {
	case isLyricsResponsePath(path):
		minSize = lyricsCompressedMinSize
		level = brotliLargeLevel
		preferred = compressionBrotli
	case isWebUIResponsePath(path, contentType):
		minSize = webUICompressedMinSize
		level = brotliLargeLevel
		preferred = compressionBrotli
	case responseSizeAtLeast(h, bodySize, hugeCompressedResponseSize):
		minSize = hugeAPICompressedMinSize
		level = brotliHugeLevel
		preferred = compressionBrotli
	case responseSizeAtLeast(h, bodySize, largeCompressedResponseSize):
		minSize = largeAPICompressedMinSize
		level = brotliLargeLevel
		preferred = compressionBrotli
	}

	if preferred == compressionBrotli && accepted.brotli {
		return compressionProfile{encoding: compressionBrotli, minSize: minSize, level: level}
	}
	if preferred == compressionZstd && accepted.zstd {
		return compressionProfile{encoding: compressionZstd, minSize: minSize, level: zstdGeneralLevel}
	}
	if accepted.zstd {
		return compressionProfile{encoding: compressionZstd, minSize: minSize, level: zstdGeneralLevel}
	}
	if accepted.brotli {
		if level == 0 {
			level = brotliLargeLevel
		}
		return compressionProfile{encoding: compressionBrotli, minSize: minSize, level: level}
	}
	if accepted.gzip {
		return compressionProfile{encoding: compressionGzip, minSize: minSize, level: gzipFallbackLevel}
	}
	return compressionProfile{}
}

func responseSizeAtLeast(h http.Header, bodySize, threshold int) bool {
	contentLength := h.Get("Content-Length")
	if contentLength == "" {
		return bodySize >= threshold
	}
	n, err := strconv.Atoi(contentLength)
	return err == nil && n >= threshold
}

func isLyricsResponsePath(path string) bool {
	path = strings.ToLower(strings.TrimSuffix(path, ".view"))
	return strings.Contains(path, "lyrics")
}

func isWebUIResponsePath(path, contentType string) bool {
	if strings.HasPrefix(path, "/app/") || path == "/app" {
		return true
	}
	mediaType, _, _ := strings.Cut(contentType, ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "text/html" ||
		mediaType == "text/css" ||
		mediaType == "application/javascript" ||
		mediaType == "application/x-javascript" ||
		mediaType == "application/manifest+json"
}

func isCompressibleContentType(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
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

func newCompressionWriter(w io.Writer, profile compressionProfile) (io.WriteCloser, error) {
	switch profile.encoding {
	case compressionBrotli:
		return newPooledBrotliWriter(w, profile.level), nil
	case compressionZstd:
		return newPooledZstdWriter(w, profile.level)
	case compressionGzip:
		return newPooledGzipWriter(w, profile.level)
	default:
		return nil, http.ErrNotSupported
	}
}

type pooledBrotliWriter struct {
	writer *brotli.Writer
	pool   *sync.Pool
}

func newPooledBrotliWriter(w io.Writer, level int) io.WriteCloser {
	pool := brotliPool(level)
	if writer, ok := pool.Get().(*brotli.Writer); ok {
		writer.Reset(w)
		return &pooledBrotliWriter{writer: writer, pool: pool}
	}
	return &pooledBrotliWriter{writer: brotli.NewWriterLevel(w, level), pool: pool}
}

func brotliPool(level int) *sync.Pool {
	if level >= brotliHugeLevel {
		return &brotliHugeWriterPool
	}
	return &brotliLargeWriterPool
}

func (w *pooledBrotliWriter) Write(p []byte) (int, error) {
	return w.writer.Write(p)
}

func (w *pooledBrotliWriter) Close() error {
	err := w.writer.Close()
	w.writer.Reset(io.Discard)
	w.pool.Put(w.writer)
	return err
}

type pooledZstdWriter struct {
	writer *zstd.Encoder
	pool   *sync.Pool
}

func newPooledZstdWriter(w io.Writer, level int) (io.WriteCloser, error) {
	if writer, ok := zstdGeneralWriterPool.Get().(*zstd.Encoder); ok {
		writer.Reset(w)
		return &pooledZstdWriter{writer: writer, pool: &zstdGeneralWriterPool}, nil
	}
	writer, err := zstd.NewWriter(w,
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
		zstd.WithEncoderConcurrency(1),
	)
	if err != nil {
		return nil, err
	}
	return &pooledZstdWriter{writer: writer, pool: &zstdGeneralWriterPool}, nil
}

func (w *pooledZstdWriter) Write(p []byte) (int, error) {
	return w.writer.Write(p)
}

func (w *pooledZstdWriter) Close() error {
	err := w.writer.Close()
	w.writer.Reset(io.Discard)
	w.pool.Put(w.writer)
	return err
}

type pooledGzipWriter struct {
	writer *gzip.Writer
	pool   *sync.Pool
}

func newPooledGzipWriter(w io.Writer, level int) (io.WriteCloser, error) {
	if writer, ok := gzipFallbackPool.Get().(*gzip.Writer); ok {
		writer.Reset(w)
		return &pooledGzipWriter{writer: writer, pool: &gzipFallbackPool}, nil
	}
	writer, err := gzip.NewWriterLevel(w, level)
	if err != nil {
		return nil, err
	}
	return &pooledGzipWriter{writer: writer, pool: &gzipFallbackPool}, nil
}

func (w *pooledGzipWriter) Write(p []byte) (int, error) {
	return w.writer.Write(p)
}

func (w *pooledGzipWriter) Close() error {
	err := w.writer.Close()
	w.writer.Reset(io.Discard)
	w.pool.Put(w.writer)
	return err
}
