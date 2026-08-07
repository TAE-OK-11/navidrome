package server

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

func PrecompressedFileServer(fileSystem fs.FS) http.Handler {
	fallback := http.FileServer(http.FS(fileSystem))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Range") != "" {
			fallback.ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".br") ||
			strings.HasSuffix(r.URL.Path, ".zst") ||
			strings.HasSuffix(r.URL.Path, ".gz") {
			fallback.ServeHTTP(w, r)
			return
		}

		accepted := acceptedCompressionEncodings(r.Header.Get("Accept-Encoding"))
		for _, encoding := range orderedPrecompressedEncodings(accepted) {
			suffix := precompressedSuffix(encoding)
			if suffix != "" && servePrecompressedAsset(w, r, fileSystem, suffix, string(encoding)) {
				return
			}
		}
		fallback.ServeHTTP(w, r)
	})
}

func orderedPrecompressedEncodings(accepted acceptedCompressions) []compressionEncoding {
	order := make([]compressionEncoding, 0, 3)
	for len(order) < 3 && accepted.hasAny() {
		encoding := selectAcceptedCompression(accepted, compressionBrotli)
		if encoding == "" {
			break
		}
		order = append(order, encoding)
		disableAcceptedCompression(&accepted, encoding)
	}
	return order
}

func disableAcceptedCompression(accepted *acceptedCompressions, encoding compressionEncoding) {
	switch encoding {
	case compressionBrotli:
		accepted.brotli = false
		accepted.brotliQuality = 0
	case compressionZstd:
		accepted.zstd = false
		accepted.zstdQuality = 0
	case compressionGzip:
		accepted.gzip = false
		accepted.gzipQuality = 0
	}
}

func precompressedSuffix(encoding compressionEncoding) string {
	switch encoding {
	case compressionBrotli:
		return ".br"
	case compressionZstd:
		return ".zst"
	case compressionGzip:
		return ".gz"
	default:
		return ""
	}
}

func servePrecompressedAsset(w http.ResponseWriter, r *http.Request, fileSystem fs.FS, suffix, encoding string) bool {
	assetPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if assetPath == "." || assetPath == "" {
		assetPath = "index.html"
	}
	if strings.HasSuffix(r.URL.Path, "/") {
		assetPath = path.Join(assetPath, "index.html")
	}

	original, err := fileSystem.Open(assetPath)
	if err != nil {
		return false
	}
	originalInfo, err := original.Stat()
	_ = original.Close()
	if err != nil || originalInfo.IsDir() {
		return false
	}

	file, err := fileSystem.Open(assetPath + suffix)
	if err != nil {
		return false
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		return false
	}

	if contentType := mime.TypeByExtension(path.Ext(assetPath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Content-Encoding", encoding)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	addVaryAcceptEncoding(w.Header())
	http.ServeContent(w, r, path.Base(assetPath), info.ModTime(), seeker)
	return true
}
