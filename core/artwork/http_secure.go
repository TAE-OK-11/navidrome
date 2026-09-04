package artwork

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

func maxExternalArtworkResponseSize() int64 {
	return maxImageReadBytes()
}

var errExternalArtworkTooLarge = errors.New("external artwork response exceeds size limit")

type boundedArtworkReadCloser struct {
	reader    io.ReadCloser
	remaining int64
}

func (r *boundedArtworkReadCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			return 0, errExternalArtworkTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func (r *boundedArtworkReadCloser) Close() error { return r.reader.Close() }

func boundedArtworkResponse(resp *http.Response) (io.ReadCloser, error) {
	if resp.ContentLength > maxExternalArtworkResponseSize() {
		return nil, fmt.Errorf("%w: content length %s", errExternalArtworkTooLarge, strconv.FormatInt(resp.ContentLength, 10))
	}
	return &boundedArtworkReadCloser{reader: resp.Body, remaining: maxExternalArtworkResponseSize()}, nil
}
