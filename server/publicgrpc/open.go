package publicgrpc

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/navidrome/navidrome/server/publicgrpc/gen"
	"github.com/navidrome/navidrome/server/subsonic/errmap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var openChunkBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, openChunkSize)
		return &buf
	},
}

type streamWriter struct {
	stream   gen.Public_OpenServer
	status   int
	header   http.Header
	sentHead bool
	flushErr error
}

func (w *streamWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *streamWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *streamWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return w.sendBody(p)
}

// ReadFrom streams in 64 KiB chunks. Send completes before the next Read, so
// the pooled buffer can be passed through without a second copy.
func (w *streamWriter) ReadFrom(source io.Reader) (int64, error) {
	bufPtr := openChunkBufPool.Get().(*[]byte)
	defer openChunkBufPool.Put(bufPtr)
	buf := *bufPtr
	var total int64
	for {
		n, err := source.Read(buf)
		if n > 0 {
			written, writeErr := w.sendBody(buf[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			return total, err
		}
	}
}

// Flush implements http.Flusher so Stream.Serve can push response headers
// (status / Content-Type) before ffmpeg emits the first audio byte.
func (w *streamWriter) Flush() {
	if w.sentHead {
		return
	}
	w.sentHead = true
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	if err := w.stream.Send(&gen.OpenChunk{
		Status:      int32(status),
		ContentType: w.Header().Get("Content-Type"),
		Headers:     headerMap(w.header),
	}); err != nil {
		// Remember the failure so the next Write surfaces a transport error
		// instead of pretending headers were delivered.
		w.flushErr = err
	}
}

func (w *streamWriter) sendBody(p []byte) (int, error) {
	// Handlers may Write an entire cover or API response at once. Bound every
	// message, not just ReadFrom, so a large write cannot exceed gRPC limits or
	// monopolize a connection with one oversized message.
	var total int
	for len(p) > 0 {
		size := min(len(p), openChunkSize)
		n, err := w.sendChunk(p[:size])
		total += n
		if err != nil {
			return total, err
		}
		p = p[size:]
	}
	return total, nil
}

func (w *streamWriter) sendChunk(p []byte) (int, error) {
	if w.flushErr != nil {
		return 0, w.flushErr
	}
	// gRPC Send marshals synchronously before returning, so p need not be
	// copied into a second pooled buffer on the Open hot path.
	chunk := &gen.OpenChunk{Data: p}
	if !w.sentHead {
		w.sentHead = true
		if w.status == 0 {
			w.status = http.StatusOK
		}
		chunk.Status = int32(w.status)
		chunk.ContentType = w.Header().Get("Content-Type")
		chunk.Headers = headerMap(w.header)
	}
	if err := w.stream.Send(chunk); err != nil {
		return 0, err
	}
	return len(p), nil
}

func finalOpenChunk(sw *streamWriter) *gen.OpenChunk {
	st := sw.status
	if st == 0 {
		st = http.StatusOK
	}
	chunk := &gen.OpenChunk{Status: int32(st), Final: true}
	if !sw.sentHead {
		chunk.ContentType = sw.Header().Get("Content-Type")
		chunk.Headers = headerMap(sw.header)
	}
	return chunk
}

func headerMap(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, vals := range h {
		if len(vals) > 0 {
			out[k] = vals[0]
		}
	}
	return out
}

// SubsonicOpener streams Subsonic handler output.
type SubsonicOpener interface {
	Open(ctx context.Context, endpoint string, query url.Values, username string, asJSON bool, w http.ResponseWriter) error
}

func (s *Service) Open(req *gen.OpenRequest, stream gen.Public_OpenServer) error {
	user, err := s.authenticate(stream.Context())
	if err != nil {
		return err
	}
	ctx := withUser(stream.Context(), user)
	token := bearerToken(stream.Context())
	api := strings.ToLower(strings.TrimSpace(req.GetApi()))
	if api == "" || api == "subsonic" {
		return s.openSubsonic(ctx, req, stream, user.UserName)
	}
	if api == "native" {
		return s.openNative(ctx, req, stream, token)
	}
	return status.Error(codes.InvalidArgument, "api must be subsonic or native")
}

func (s *Service) openSubsonic(ctx context.Context, req *gen.OpenRequest, stream gen.Public_OpenServer, username string) (err error) {
	defer recoverOpenAbort(&err)
	opener, ok := s.invoker.(SubsonicOpener)
	if !ok || opener == nil {
		return status.Error(codes.Unavailable, "Subsonic streaming is not configured")
	}
	endpoint := strings.TrimSpace(req.GetPath())
	if endpoint == "" {
		return status.Error(codes.InvalidArgument, "path is required")
	}
	query := mapToURLValues(req.GetParams())
	sw := &streamWriter{stream: stream}
	if err := opener.Open(ctx, endpoint, query, username, req.GetJson(), sw); err != nil {
		code := errmap.GRPCCode(err)
		return status.Errorf(code, "open %s: %v", endpoint, err)
	}
	return stream.Send(finalOpenChunk(sw))
}

func (s *Service) openNative(ctx context.Context, req *gen.OpenRequest, stream gen.Public_OpenServer, token string) (err error) {
	defer recoverOpenAbort(&err)
	if s.native == nil {
		return status.Error(codes.Unavailable, "Native API invoker is not configured")
	}
	query := mapToURLValues(req.GetParams())
	if opener, ok := s.native.(NativeOpener); ok {
		sw := &streamWriter{stream: stream}
		if err := opener.Open(ctx, req.GetMethod(), req.GetPath(), query, req.GetContentType(), req.GetBody(), token, sw); err != nil {
			return status.Errorf(codes.Internal, "native %s: %v", req.GetPath(), err)
		}
		return stream.Send(finalOpenChunk(sw))
	}
	statusCode, header, body, err := s.native.Invoke(ctx, req.GetMethod(), req.GetPath(), query, req.GetContentType(), req.GetBody(), token)
	if err != nil {
		return status.Errorf(codes.Internal, "native %s: %v", req.GetPath(), err)
	}
	ct := header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	headers := headerMap(header)
	if len(body) == 0 {
		return stream.Send(&gen.OpenChunk{Status: int32(statusCode), ContentType: ct, Headers: headers, Final: true})
	}
	offset := 0
	for offset < len(body) {
		end := offset + openChunkSize
		if end > len(body) {
			end = len(body)
		}
		chunk := &gen.OpenChunk{Data: body[offset:end]}
		if offset == 0 {
			chunk.Status = int32(statusCode)
			chunk.ContentType = ct
			chunk.Headers = headers
		}
		offset = end
		chunk.Final = offset >= len(body)
		if err := stream.Send(chunk); err != nil {
			return err
		}
	}
	return nil
}

const openChunkSize = 64 * 1024

// HTTP handlers use ErrAbortHandler to truncate a committed media response.
// Open calls those handlers directly, outside net/http's recovery boundary.
// Translate only this control signal into a gRPC failure; never send Final.
func recoverOpenAbort(err *error) {
	if recovered := recover(); recovered != nil {
		if cause, ok := recovered.(error); ok && errors.Is(cause, http.ErrAbortHandler) {
			*err = status.Error(codes.Internal, "media stream aborted")
			return
		}
		panic(recovered)
	}
}
