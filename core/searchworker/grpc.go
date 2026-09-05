package searchworker

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/rustworker"
	"github.com/navidrome/navidrome/core/searchworker/gen"
	"google.golang.org/grpc"
)

var (
	searchGRPC = rustworker.NewManagedGRPC(rustworker.ManagedGRPCConfig{
		Name:   "search",
		Listen: rustworker.DefaultListenAddr("navidrome-search"),
		Resolve: func() (string, error) {
			return Resolve()
		},
		ExtraEnvFn: searchWorkerEnv,
		Health: func(ctx context.Context, conn *grpc.ClientConn) error {
			resp, err := gen.NewSearchClient(conn).Health(ctx, &gen.HealthRequest{})
			if err != nil {
				return err
			}
			if !resp.GetOk() {
				return errNoGRPC
			}
			return nil
		},
	})
	errNoGRPC = errors.New("search gRPC worker unavailable")
)

// ErrNoGRPC is returned when the search gRPC worker is not running.
var ErrNoGRPC = errNoGRPC

func searchWorkerEnv() []string {
	indexPath := IndexPath()
	if indexPath == "" {
		return nil
	}
	return []string{"NAVIDROME_SEARCH_INDEX_PATH=" + indexPath}
}

// IndexPath returns the on-disk Tantivy index directory for the search worker.
func IndexPath() string {
	dataFolder, err := conf.Server.DataFolder.Path()
	if err != nil || dataFolder == "" {
		return ""
	}
	return filepath.Join(dataFolder, "rust-search-index")
}

// SearchClient returns the search gRPC client, or nil when unavailable.
func SearchClient() gen.SearchClient {
	conn, err := searchGRPC.Conn()
	if err != nil {
		return nil
	}
	return gen.NewSearchClient(conn)
}

// InvalidateGRPC closes the search worker so the next RPC starts a fresh process.
func InvalidateGRPC() {
	searchGRPC.Invalidate()
}

// AdoptGRPC installs a preflight-started worker process into the managed host.
// Returns false when proc is nil or the host already has a live process.
func AdoptGRPC(proc *rustworker.GRPCProcess) bool {
	return searchGRPC.Adopt(proc)
}

// WarmGRPC starts the search worker in the background for a warm first RPC.
func WarmGRPC() {
	searchGRPC.Warm()
}

// CallSearch executes fn with automatic transport-error retry.
func CallSearch[T any](ctx context.Context, fn func(context.Context, gen.SearchClient) (T, error)) (T, error) {
	return rustworker.CallGRPC(searchGRPC, ctx, func(ctx context.Context, conn *grpc.ClientConn) (T, error) {
		result, err := fn(ctx, gen.NewSearchClient(conn))
		if errors.Is(err, rustworker.ErrWorkerUnavailable) {
			var zero T
			return zero, errNoGRPC
		}
		return result, err
	})
}
