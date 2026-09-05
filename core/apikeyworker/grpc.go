package apikeyworker

import (
	"context"
	"errors"

	"github.com/navidrome/navidrome/core/apikeyworker/gen"
	"github.com/navidrome/navidrome/core/rustworker"
	"google.golang.org/grpc"
)

var (
	apikeysGRPC = rustworker.NewManagedGRPC(rustworker.ManagedGRPCConfig{
		Name:   "apikeys",
		Listen: rustworker.DefaultListenAddr("navidrome-apikeys"),
		Resolve: func() (string, error) {
			return Resolve()
		},
		Health: func(ctx context.Context, conn *grpc.ClientConn) error {
			resp, err := gen.NewApiKeysClient(conn).Health(ctx, &gen.HealthRequest{})
			if err != nil {
				return err
			}
			if !resp.GetOk() {
				return errNoGRPC
			}
			return nil
		},
	})
	errNoGRPC = errors.New("apikeys gRPC worker unavailable")
)

// WarmGRPC starts the apikeys worker in the background for a warm first RPC.
func WarmGRPC() {
	apikeysGRPC.Warm()
}

func callAPIKeys[T any](ctx context.Context, fn func(context.Context, *grpc.ClientConn) (T, error)) (T, error) {
	result, err := rustworker.CallGRPC(apikeysGRPC, ctx, fn)
	if errors.Is(err, rustworker.ErrWorkerUnavailable) {
		var zero T
		return zero, errNoGRPC
	}
	return result, err
}

func generateGRPC(ctx context.Context, pepper string) (generateResult, error) {
	return callAPIKeys(ctx, func(ctx context.Context, conn *grpc.ClientConn) (generateResult, error) {
		cli := gen.NewApiKeysClient(conn)
		resp, err := cli.Generate(ctx, &gen.GenerateRequest{Pepper: pepper})
		if err != nil {
			return generateResult{}, err
		}
		if !resp.GetOk() {
			return generateResult{}, errors.New(nonEmpty(resp.GetError(), "Rust apikeys Generate failed"))
		}
		return generateResult{Token: resp.GetToken(), LookupPrefix: resp.GetLookupPrefix(), Hash: resp.GetHash()}, nil
	})
}

func hashGRPC(ctx context.Context, token, pepper string) (string, string, error) {
	type hashResult struct {
		prefix string
		hash   string
	}
	result, err := callAPIKeys(ctx, func(ctx context.Context, conn *grpc.ClientConn) (hashResult, error) {
		cli := gen.NewApiKeysClient(conn)
		resp, err := cli.Hash(ctx, &gen.HashRequest{Token: token, Pepper: pepper})
		if err != nil {
			return hashResult{}, err
		}
		if !resp.GetOk() {
			return hashResult{}, errors.New(nonEmpty(resp.GetError(), "Rust apikeys Hash failed"))
		}
		return hashResult{prefix: resp.GetLookupPrefix(), hash: resp.GetHash()}, nil
	})
	if err != nil {
		return "", "", err
	}
	return result.prefix, result.hash, nil
}

func verifyGRPC(ctx context.Context, token, hash, pepper string) (bool, error) {
	return callAPIKeys(ctx, func(ctx context.Context, conn *grpc.ClientConn) (bool, error) {
		cli := gen.NewApiKeysClient(conn)
		resp, err := cli.Verify(ctx, &gen.VerifyRequest{Token: token, Hash: hash, Pepper: pepper})
		if err != nil {
			return false, err
		}
		if !resp.GetOk() {
			return false, errors.New(nonEmpty(resp.GetError(), "Rust apikeys Verify failed"))
		}
		return resp.GetValid(), nil
	})
}

func nonEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
