package server

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	gzip "github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

const (
	generalAPICompressedMinSize      = 256
	largeAPICompressedMinSize        = 2048
	hugeAPICompressedMinSize         = 4096
	lyricsCompressedMinSize          = 256
	webUICompressedMinSize           = 1024
	largeCompressedResponseSize      = 16 << 10
	hugeCompressedResponseSize       = 256 << 10
	compressionDecisionBufferTarget  = largeCompressedResponseSize
	apiCompressionDecisionBufferSize = generalAPICompressedMinSize
	brotliLargeLevel                 = 5
	brotliHugeLevel                  = 6
	zstdGeneralLevel                 = 1
	gzipFallbackLevel                = 4
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
	apiCompressionBufferPool = sync.Pool{
		New: func() any {
			return make([]byte, 0, apiCompressionDecisionBufferSize)
		},
	}
	brotliLargeWriterPool sync.Pool
	brotliHugeWriterPool  sync.Pool
	brotliWrapperPool     sync.Pool
	zstdGeneralWriterPool sync.Pool
	zstdWrapperPool       sync.Pool
	gzipFallbackPool      sync.Pool
	gzipWrapperPool       sync.Pool
)

func compressMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldBypassCompressionRequest(r) {
				next.ServeHTTP(w, r)
				return
			}

			acceptEncoding := r.Header.Get("Accept-Encoding")
			if acceptEncoding == "" {
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
			defer func() {
				_ = cw.Close()
			}()
			next.ServeHTTP(cw, r)
		})
	}
}

func shouldBypassCompressionRequest(r *http.Request) bool {
	if r.Header.Get(rustHTTP3CompressionHeader) != "" {
		r.Header.Del(rustHTTP3CompressionHeader)
		return true
	}
	if r.Method == http.MethodHead || r.Header.Get("Range") != "" || isMediaResponsePath(r.URL.Path) || isSensitiveAuthResponsePath(r.URL.Path) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("Connection")), "upgrade")
}

func isSensitiveAuthResponsePath(requestPath string) bool {
	requestPath = strings.ToLower(strings.TrimSuffix(requestPath, "/"))
	return requestPath == "/auth" || strings.HasSuffix(requestPath, "/auth") || strings.Contains(requestPath, "/auth/")
}

type acceptedCompressions struct {
	brotli bool
	zstd   bool
	gzip   bool

	brotliQuality   float64
	zstdQuality     float64
	gzipQuality     float64
	identitySet     bool
	identityQuality float64
}

type compressionProfile struct {
	encoding compressionEncoding
	minSize  int
	level    int
}

func (a acceptedCompressions) hasAny() bool {
	return a.brotli || a.zstd || a.gzip
}

func (a acceptedCompressions) forbidsIdentity() bool {
	return a.identitySet && a.identityQuality <= 0
}

func (a acceptedCompressions) quality(encoding compressionEncoding) float64 {
	switch encoding {
	case compressionBrotli:
		if a.brotliQuality > 0 {
			return a.brotliQuality
		}
		if a.brotli {
			return 1
		}
	case compressionZstd:
		if a.zstdQuality > 0 {
			return a.zstdQuality
		}
		if a.zstd {
			return 1
		}
	case compressionGzip:
		if a.gzipQuality > 0 {
			return a.gzipQuality
		}
		if a.gzip {
			return 1
		}
	}
	return 0
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
		part = strings.TrimSpace(part)
		switch {
		case strings.EqualFold(part, string(compressionBrotli)):
			accepted.brotli = true
		case strings.EqualFold(part, string(compressionZstd)):
			accepted.zstd = true
		case strings.EqualFold(part, string(compressionGzip)):
			accepted.gzip = true
		}
	}
	return accepted
}

func acceptedCompressionEncodingsSlow(acceptEncoding string) acceptedCompressions {
	var accepted acceptedCompressions
	var brotliSet, zstdSet, gzipSet bool
	var wildcardQuality float64
	var wildcardSet bool

	for part := range strings.SplitSeq(acceptEncoding, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		token = strings.TrimSpace(strings.ToLower(token))
		quality := encodingQuality(params)
		switch token {
		case string(compressionBrotli):
			accepted.brotli = quality > 0
			accepted.brotliQuality = quality
			brotliSet = true
		case string(compressionZstd):
			accepted.zstd = quality > 0
			accepted.zstdQuality = quality
			zstdSet = true
		case string(compressionGzip):
			accepted.gzip = quality > 0
			accepted.gzipQuality = quality
			gzipSet = true
		case "identity":
			accepted.identitySet = true
			accepted.identityQuality = quality
		case "*":
			wildcardQuality = quality
			wildcardSet = true
		}
	}

	if wildcardSet {
		if !brotliSet {
			accepted.brotli = wildcardQuality > 0
			accepted.brotliQuality = wildcardQuality
		}
		if !zstdSet {
			accepted.zstd = wildcardQuality > 0
			accepted.zstdQuality = wildcardQuality
		}
		if !gzipSet {
			accepted.gzip = wildcardQuality > 0
			accepted.gzipQuality = wildcardQuality
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
		q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || q < 0 || q > 1 {
			return 0
		}
		return q
	}
	return 1
}

func isMediaResponsePath(path string) bool {
	path = strings.ToLower(strings.TrimSuffix(path, ".view"))
	return strings.HasSuffix(path, "/rest/stream") ||
		strings.HasSuffix(path, "/rest/download") ||
		strings.HasSuffix(path, "/rest/gettranscodestream") ||
		strings.HasSuffix(path, "/rest/getcoverart") ||
		strings.HasSuffix(path, "/rest/getavatar") ||
		strings.Contains(path, "/share/s/") ||
		strings.Contains(path, "/share/d/") ||
		strings.Contains(path, "/share/img/")
}

func isAPIResponsePath(path string) bool {
	path = strings.ToLower(strings.TrimSuffix(path, ".view"))
	return strings.HasPrefix(path, "/api/") ||
		strings.HasPrefix(path, "/rest/") ||
		strings.HasPrefix(path, "/auth/") ||
		strings.Contains(path, "/api/") ||
		strings.Contains(path, "/rest/") ||
		strings.Contains(path, "/auth/")
}

func compressionDecisionTarget(path string) int {
	if isAPIResponsePath(path) {
		return apiCompressionDecisionBufferSize
	}
	return compressionDecisionBufferTarget
}

type compressResponseWriter struct {
	http.ResponseWriter
	accepted   acceptedCompressions
	encoding   compressionEncoding
	path       string
	status     int
	writer     io.WriteCloser
	buffer     []byte
	bufferPool *sync.Pool
	raw        bool
	closed     bool
}

func (w *compressResponseWriter) WriteHeader(status int) {
	// Informational responses can precede the final response. Do not let a 103
	// Early Hints response lock the wrapper into a non-final status.
	if status >= 100 && status < 200 && status != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	if status == http.StatusSwitchingProtocols {
		w.raw = true
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *compressResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.writer != nil || w.raw {
		return w.writeStarted(p)
	}

	if w.buffer == nil && w.Header().Get("Content-Type") != "" && w.Header().Get("Content-Length") != "" {
		if err := w.start(nil); err != nil {
			return 0, err
		}
		return w.writeStarted(p)
	}

	decisionTarget := compressionDecisionTarget(w.path)
	if w.buffer == nil && len(p) >= decisionTarget {
		if err := w.start(p); err != nil {
			return 0, err
		}
		return w.writeStarted(p)
	}

	if w.buffer == nil {
		w.buffer, w.bufferPool = getCompressionBuffer(decisionTarget)
	}
	w.buffer = append(w.buffer, p...)
	if len(w.buffer) < decisionTarget {
		return len(p), nil
	}
	if err := w.flushBuffered(); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *compressResponseWriter) ReadFrom(source io.Reader) (int64, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.writer != nil {
		return io.Copy(w.writer, source)
	}
	if w.raw {
		return copyResponseBody(w.ResponseWriter, source)
	}

	contentType := responseContentType(w.Header(), nil)
	if contentType != "" && !isCompressibleResponse(w.status, w.Header(), contentType) {
		w.raw = true
		w.ResponseWriter.WriteHeader(w.status)
		return copyResponseBody(w.ResponseWriter, source)
	}
	if contentType != "" && w.Header().Get("Content-Length") != "" {
		if err := w.start(nil); err != nil {
			return 0, err
		}
		if w.writer != nil {
			return io.Copy(w.writer, source)
		}
		return copyResponseBody(w.ResponseWriter, source)
	}

	return io.Copy(struct{ io.Writer }{w}, source)
}

func copyResponseBody(destination http.ResponseWriter, source io.Reader) (int64, error) {
	if readerFrom, ok := destination.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(source)
	}
	return io.Copy(struct{ io.Writer }{destination}, source)
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
	if err := w.flushBuffered(); err != nil {
		return
	}
	if flusher, ok := w.writer.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			return
		}
	}
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
	buf := w.buffer
	if err := w.start(buf); err != nil {
		return err
	}
	if len(buf) == 0 {
		w.releaseBuffer()
		return nil
	}
	_, err := w.writeStarted(buf)
	w.releaseBuffer()
	return err
}

func (w *compressResponseWriter) start(body []byte) error {
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}

	contentType := responseContentType(w.Header(), body)
	if !isCompressibleResponse(status, w.Header(), contentType) {
		w.raw = true
		w.ResponseWriter.WriteHeader(status)
		return nil
	}

	bodySize := responseBodySize(w.Header(), len(body))
	profile := selectCompressionProfile(w.accepted, w.path, w.Header(), contentType, bodySize)
	if profile.encoding == "" || (bodySize < profile.minSize && !w.accepted.forbidsIdentity()) {
		w.raw = true
		w.ResponseWriter.WriteHeader(status)
		return nil
	}

	writer, err := newCompressionWriter(w.ResponseWriter, profile)
	if err != nil {
		w.raw = true
		w.ResponseWriter.WriteHeader(status)
		return nil //nolint:nilerr // Compression is optional; degrade to the raw response.
	}
	w.encoding = profile.encoding
	setCompressionHeaders(w.Header(), w.encoding)
	w.ResponseWriter.WriteHeader(status)
	w.writer = writer
	return nil
}

func (w *compressResponseWriter) writeStarted(p []byte) (int, error) {
	if w.writer != nil {
		if _, err := w.writer.Write(p); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	return w.ResponseWriter.Write(p)
}

func getCompressionBuffer(decisionTarget int) ([]byte, *sync.Pool) {
	pool := &compressionBufferPool
	if decisionTarget <= apiCompressionDecisionBufferSize {
		pool = &apiCompressionBufferPool
	}
	return pool.Get().([]byte)[:0], pool
}

func (w *compressResponseWriter) releaseBuffer() {
	if w.buffer == nil {
		return
	}
	if w.bufferPool != nil {
		maxCapacity := compressionDecisionBufferTarget * 2
		if w.bufferPool == &apiCompressionBufferPool {
			maxCapacity = apiCompressionDecisionBufferSize * 2
		}
		if cap(w.buffer) <= maxCapacity {
			w.bufferPool.Put(w.buffer[:0])
		}
	}
	w.buffer = nil
	w.bufferPool = nil
}

func isCompressibleResponse(status int, h http.Header, contentType string) bool {
	if status < http.StatusOK || status == http.StatusNoContent || status == http.StatusNotModified || status == http.StatusPartialContent {
		return false
	}
	if h.Get("Content-Range") != "" || h.Get("Content-Encoding") != "" || strings.Contains(strings.ToLower(h.Get("Cache-Control")), "no-transform") {
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

func responseBodySize(h http.Header, bufferedSize int) int {
	contentLength := h.Get("Content-Length")
	if contentLength == "" {
		return bufferedSize
	}
	n, err := strconv.Atoi(contentLength)
	if err != nil || n < 0 {
		return bufferedSize
	}
	return n
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
	case isAPIResponsePath(path):
		minSize = generalAPICompressedMinSize
		level = zstdGeneralLevel
		preferred = compressionZstd
	case responseSizeAtLeast(h, bodySize, hugeCompressedResponseSize):
		minSize = hugeAPICompressedMinSize
		level = brotliHugeLevel
		preferred = compressionBrotli
	case responseSizeAtLeast(h, bodySize, largeCompressedResponseSize):
		minSize = largeAPICompressedMinSize
		level = brotliLargeLevel
		preferred = compressionBrotli
	}

	selected := selectAcceptedCompression(accepted, preferred)
	switch selected {
	case compressionBrotli:
		brotliLevel := brotliLargeLevel
		if preferred == compressionBrotli && level >= brotliHugeLevel {
			brotliLevel = brotliHugeLevel
		}
		return compressionProfile{encoding: compressionBrotli, minSize: minSize, level: brotliLevel}
	case compressionZstd:
		return compressionProfile{encoding: compressionZstd, minSize: minSize, level: zstdGeneralLevel}
	case compressionGzip:
		return compressionProfile{encoding: compressionGzip, minSize: minSize, level: gzipFallbackLevel}
	default:
		return compressionProfile{}
	}
}

func selectAcceptedCompression(accepted acceptedCompressions, preferred compressionEncoding) compressionEncoding {
	best := compressionEncoding("")
	bestQuality := float64(0)
	if quality := accepted.quality(preferred); quality > 0 {
		best = preferred
		bestQuality = quality
	}

	for _, encoding := range []compressionEncoding{compressionZstd, compressionBrotli, compressionGzip} {
		quality := accepted.quality(encoding)
		if quality > bestQuality {
			best = encoding
			bestQuality = quality
		}
	}
	return best
}

func responseSizeAtLeast(h http.Header, bodySize, threshold int) bool {
	return responseBodySize(h, bodySize) >= threshold
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
		mediaType == "text/javascript" ||
		mediaType == "application/javascript" ||
		mediaType == "application/x-javascript" ||
		mediaType == "application/manifest+json"
}

func isCompressibleContentType(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "" {
		return false
	}
	if strings.HasPrefix(mediaType, "text/") {
		return mediaType != "text/event-stream"
	}
	if strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "video/") {
		return isTextPlaylistMediaType(mediaType)
	}
	if strings.HasPrefix(mediaType, "image/") {
		return mediaType == "image/svg+xml"
	}
	if isAlreadyCompressedBinaryMediaType(mediaType) {
		return false
	}

	switch mediaType {
	case "application/json",
		"application/xml",
		"application/javascript",
		"application/x-javascript",
		"application/manifest+json",
		"application/problem+json",
		"application/x-ndjson",
		"application/json-seq",
		"application/yaml",
		"application/x-yaml",
		"application/toml",
		"application/sql",
		"application/graphql-response+json",
		"application/x-www-form-urlencoded",
		"application/wasm",
		"application/vnd.apple.mpegurl",
		"application/x-mpegurl":
		return true
	default:
		return strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml")
	}
}

func isTextPlaylistMediaType(mediaType string) bool {
	switch mediaType {
	case "audio/mpegurl", "audio/x-mpegurl", "audio/vnd.apple.mpegurl":
		return true
	default:
		return false
	}
}

func isAlreadyCompressedBinaryMediaType(mediaType string) bool {
	switch mediaType {
	case "application/zip",
		"application/gzip",
		"application/x-gzip",
		"application/x-7z-compressed",
		"application/x-rar-compressed",
		"application/zstd",
		"application/pdf",
		"font/woff",
		"font/woff2",
		"application/font-woff",
		"application/vnd.ms-fontobject":
		return true
	default:
		return false
	}
}

func setCompressionHeaders(h http.Header, encoding compressionEncoding) {
	h.Set("Content-Encoding", string(encoding))
	h.Del("Content-Length")
	h.Del("Content-MD5")
	h.Del("Content-Digest")
	h.Del("Digest")
	weakenETagAfterCompression(h)
	addVaryAcceptEncoding(h)
}

func weakenETagAfterCompression(h http.Header) {
	etag := strings.TrimSpace(h.Get("ETag"))
	if etag == "" || strings.HasPrefix(etag, "W/\"") {
		return
	}
	if strings.HasPrefix(etag, "\"") && strings.HasSuffix(etag, "\"") {
		h.Set("ETag", "W/"+etag)
		return
	}
	h.Del("ETag")
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
	pooled, _ := brotliWrapperPool.Get().(*pooledBrotliWriter)
	if pooled == nil {
		pooled = &pooledBrotliWriter{}
	}
	pooled.pool = pool
	if writer, ok := pool.Get().(*brotli.Writer); ok {
		writer.Reset(w)
		pooled.writer = writer
		return pooled
	}
	pooled.writer = brotli.NewWriterLevel(w, level)
	return pooled
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

func (w *pooledBrotliWriter) Flush() error {
	return w.writer.Flush()
}

func (w *pooledBrotliWriter) Close() error {
	if w.writer == nil {
		return nil
	}
	err := w.writer.Close()
	w.pool.Put(w.writer)
	w.writer = nil
	w.pool = nil
	brotliWrapperPool.Put(w)
	return err
}

type pooledZstdWriter struct {
	writer *zstd.Encoder
	pool   *sync.Pool
}

func newPooledZstdWriter(w io.Writer, level int) (io.WriteCloser, error) {
	pooled, _ := zstdWrapperPool.Get().(*pooledZstdWriter)
	if pooled == nil {
		pooled = &pooledZstdWriter{pool: &zstdGeneralWriterPool}
	}
	if writer, ok := zstdGeneralWriterPool.Get().(*zstd.Encoder); ok {
		writer.Reset(w)
		pooled.writer = writer
		return pooled, nil
	}
	writer, err := zstd.NewWriter(w,
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
		zstd.WithEncoderConcurrency(1),
	)
	if err != nil {
		zstdWrapperPool.Put(pooled)
		return nil, err
	}
	pooled.writer = writer
	return pooled, nil
}

func (w *pooledZstdWriter) Write(p []byte) (int, error) {
	return w.writer.Write(p)
}

func (w *pooledZstdWriter) Flush() error {
	return w.writer.Flush()
}

func (w *pooledZstdWriter) Close() error {
	if w.writer == nil {
		return nil
	}
	err := w.writer.Close()
	w.pool.Put(w.writer)
	w.writer = nil
	zstdWrapperPool.Put(w)
	return err
}

type pooledGzipWriter struct {
	writer *gzip.Writer
	pool   *sync.Pool
}

func newPooledGzipWriter(w io.Writer, level int) (io.WriteCloser, error) {
	pooled, _ := gzipWrapperPool.Get().(*pooledGzipWriter)
	if pooled == nil {
		pooled = &pooledGzipWriter{pool: &gzipFallbackPool}
	}
	if writer, ok := gzipFallbackPool.Get().(*gzip.Writer); ok {
		writer.Reset(w)
		pooled.writer = writer
		return pooled, nil
	}
	writer, err := gzip.NewWriterLevel(w, level)
	if err != nil {
		gzipWrapperPool.Put(pooled)
		return nil, err
	}
	pooled.writer = writer
	return pooled, nil
}

func (w *pooledGzipWriter) Write(p []byte) (int, error) {
	return w.writer.Write(p)
}

func (w *pooledGzipWriter) Flush() error {
	return w.writer.Flush()
}

func (w *pooledGzipWriter) Close() error {
	if w.writer == nil {
		return nil
	}
	err := w.writer.Close()
	w.pool.Put(w.writer)
	w.writer = nil
	gzipWrapperPool.Put(w)
	return err
}
