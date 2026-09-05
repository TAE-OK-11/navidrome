package scannerworker

import (
	"context"
	"errors"

	"github.com/navidrome/navidrome/core/rustworker"
	"github.com/navidrome/navidrome/core/scannerworker/gen"
	"google.golang.org/grpc"
)

var (
	scannerGRPC = rustworker.NewManagedGRPC(rustworker.ManagedGRPCConfig{
		Name:   "scanner",
		Listen: rustworker.DefaultListenAddr("navidrome-scanner"),
		Resolve: func() (string, error) {
			return Resolve()
		},
		Health: func(ctx context.Context, conn *grpc.ClientConn) error {
			_, err := gen.NewFolderHashClient(conn).Health(ctx, &gen.HealthRequest{})
			return err
		},
	})
	errFolderHashNoGRPC = errors.New("folder-hash gRPC worker unavailable")
	errWalkNoGRPC       = errors.New("scanner walk gRPC worker unavailable")
)

// ErrWalkNoGRPC is returned when the scanner Walk RPC cannot be started.
var ErrWalkNoGRPC = errWalkNoGRPC

func callScanner[T any](ctx context.Context, fn func(context.Context, *grpc.ClientConn) (T, error)) (T, error) {
	result, err := rustworker.CallGRPC(scannerGRPC, ctx, fn)
	if errors.Is(err, rustworker.ErrWorkerUnavailable) {
		var zero T
		return zero, errFolderHashNoGRPC
	}
	return result, err
}

func hashGRPC(ctx context.Context, request FolderHashRequest) (string, error) {
	return callScanner(ctx, func(ctx context.Context, conn *grpc.ClientConn) (string, error) {
		cli := gen.NewFolderHashClient(conn)
		resp, err := cli.Hash(ctx, toProtoHashRequest(request))
		if err != nil {
			return "", err
		}
		if !resp.GetOk() {
			msg := resp.GetError()
			if msg == "" {
				msg = "Rust folder hash worker failed"
			}
			return "", errors.New(msg)
		}
		return resp.GetHash(), nil
	})
}

// ScannerGRPC returns the Walk client, or nil when the worker is unavailable.
func ScannerGRPC() gen.ScannerClient {
	conn, err := scannerGRPC.Conn()
	if err != nil {
		return nil
	}
	return gen.NewScannerClient(conn)
}

// InvalidateGRPC closes the scanner worker so the next RPC starts a fresh process.
func InvalidateGRPC() {
	scannerGRPC.Invalidate()
}

// WarmGRPC starts the scanner worker in the background for a warm first RPC.
func WarmGRPC() {
	scannerGRPC.Warm()
}

func toProtoHashRequest(request FolderHashRequest) *gen.HashRequest {
	return &gen.HashRequest{
		Path:              request.Path,
		ModTimeNs:         request.ModTimeNS,
		ImagesUpdatedAtNs: request.ImagesUpdatedAtNS,
		NumPlaylists:      int32(request.NumPlaylists),
		NumSubfolders:     int32(request.NumSubfolders),
		AudioFiles:        toProtoFiles(request.AudioFiles),
		ImageFiles:        toProtoFiles(request.ImageFiles),
	}
}

func toProtoFiles(files map[string]FolderHashFile) map[string]*gen.FileMeta {
	if len(files) == 0 {
		return nil
	}
	out := make(map[string]*gen.FileMeta, len(files))
	for name, file := range files {
		out[name] = &gen.FileMeta{Name: file.Name, Size: file.Size, ModTimeNs: file.ModTimeNS}
	}
	return out
}
