package publicgrpc

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/navidrome/navidrome/server/publicgrpc/gen"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type streamWriter struct {
	stream   gen.Public_OpenServer
	status   int
	header   http.Header
	sentHead bool
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
	chunk := &gen.OpenChunk{Data: append([]byte(nil), p...)}
	if !w.sentHead {
		w.sentHead = true
		if w.status == 0 {
			w.status = http.StatusOK
		}
		chunk.Status = int32(w.status)
		chunk.ContentType = w.header.Get("Content-Type")
	}
	if err := w.stream.Send(chunk); err != nil {
		return 0, err
	}
	return len(p), nil
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

func (s *Service) openSubsonic(ctx context.Context, req *gen.OpenRequest, stream gen.Public_OpenServer, username string) error {
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
		return status.Errorf(codes.Internal, "open %s: %v", endpoint, err)
	}
	if !sw.sentHead {
		st := sw.status
		if st == 0 {
			st = http.StatusOK
		}
		return stream.Send(&gen.OpenChunk{Status: int32(st), Final: true})
	}
	return stream.Send(&gen.OpenChunk{Final: true})
}

func (s *Service) openNative(ctx context.Context, req *gen.OpenRequest, stream gen.Public_OpenServer, token string) error {
	if s.native == nil {
		return status.Error(codes.Unavailable, "Native API invoker is not configured")
	}
	query := mapToURLValues(req.GetParams())
	if opener, ok := s.native.(NativeOpener); ok {
		sw := &streamWriter{stream: stream}
		if err := opener.Open(ctx, req.GetMethod(), req.GetPath(), query, req.GetContentType(), req.GetBody(), token, sw); err != nil {
			return status.Errorf(codes.Internal, "native %s: %v", req.GetPath(), err)
		}
		if !sw.sentHead {
			st := sw.status
			if st == 0 {
				st = http.StatusOK
			}
			return stream.Send(&gen.OpenChunk{Status: int32(st), Final: true})
		}
		return stream.Send(&gen.OpenChunk{Final: true})
	}
	statusCode, header, body, err := s.native.Invoke(ctx, req.GetMethod(), req.GetPath(), query, req.GetContentType(), req.GetBody(), token)
	if err != nil {
		return status.Errorf(codes.Internal, "native %s: %v", req.GetPath(), err)
	}
	ct := header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	if len(body) == 0 {
		return stream.Send(&gen.OpenChunk{Status: int32(statusCode), ContentType: ct, Final: true})
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
