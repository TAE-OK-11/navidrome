package apikeyworker

import (
	"context"
	"errors"
	"sync"

	"github.com/navidrome/navidrome/core/apikeyworker/gen"
	"github.com/navidrome/navidrome/core/rustworker"
	"github.com/navidrome/navidrome/log"
)

var (
	grpcOnce sync.Once
	grpcProc *rustworker.GRPCProcess
	grpcCli  gen.ApiKeysClient
)

func grpcClient() gen.ApiKeysClient {
	grpcOnce.Do(func() {
		binary, err := Resolve()
		if err != nil {
			return
		}
		proc, err := rustworker.StartGRPC(context.Background(), binary, rustworker.DefaultListenAddr("navidrome-apikeys"), nil)
		if err != nil {
			log.Debug("Rust apikeys gRPC worker unavailable; using NDJSON fallback", err)
			return
		}
		cli := gen.NewApiKeysClient(proc.Conn)
		healthCtx, cancel := context.WithTimeout(context.Background(), rustworker.DefaultGRPCDialTimeout)
		defer cancel()
		if _, err := cli.Health(healthCtx, &gen.HealthRequest{}); err != nil {
			proc.Close()
			log.Debug("Rust apikeys gRPC health failed; using NDJSON fallback", err)
			return
		}
		grpcProc = proc
		grpcCli = cli
		if grpcProc.Cmd != nil && grpcProc.Cmd.Process != nil {
			log.Info("API key hashing routed through Rust gRPC worker", "pid", grpcProc.Cmd.Process.Pid, "listen", grpcProc.Addr)
		} else {
			log.Info("API key hashing routed through Rust gRPC worker", "listen", grpcProc.Addr)
		}
	})
	return grpcCli
}

func generateGRPC(ctx context.Context, pepper string) (generateResult, error) {
	cli := grpcClient()
	if cli == nil {
		return generateResult{}, errNoGRPC
	}
	resp, err := cli.Generate(ctx, &gen.GenerateRequest{Pepper: pepper})
	if err != nil {
		return generateResult{}, err
	}
	if !resp.GetOk() {
		return generateResult{}, errors.New(nonEmpty(resp.GetError(), "Rust apikeys Generate failed"))
	}
	return generateResult{Token: resp.GetToken(), LookupPrefix: resp.GetLookupPrefix(), Hash: resp.GetHash()}, nil
}

func hashGRPC(ctx context.Context, token, pepper string) (string, string, error) {
	cli := grpcClient()
	if cli == nil {
		return "", "", errNoGRPC
	}
	resp, err := cli.Hash(ctx, &gen.HashRequest{Token: token, Pepper: pepper})
	if err != nil {
		return "", "", err
	}
	if !resp.GetOk() {
		return "", "", errors.New(nonEmpty(resp.GetError(), "Rust apikeys Hash failed"))
	}
	return resp.GetLookupPrefix(), resp.GetHash(), nil
}

func verifyGRPC(ctx context.Context, token, hash, pepper string) (bool, error) {
	cli := grpcClient()
	if cli == nil {
		return false, errNoGRPC
	}
	resp, err := cli.Verify(ctx, &gen.VerifyRequest{Token: token, Hash: hash, Pepper: pepper})
	if err != nil {
		return false, err
	}
	if !resp.GetOk() {
		return false, errors.New(nonEmpty(resp.GetError(), "Rust apikeys Verify failed"))
	}
	return resp.GetValid(), nil
}

var errNoGRPC = errors.New("apikeys gRPC worker unavailable")

func nonEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
