package scannerworker

import (
	"context"
	"errors"
	"sync"

	"github.com/navidrome/navidrome/core/rustworker"
	"github.com/navidrome/navidrome/core/scannerworker/gen"
	"github.com/navidrome/navidrome/log"
)

var (
	scannerGRPCOnce     sync.Once
	scannerGRPCProc     *rustworker.GRPCProcess
	folderHashGRPCCli   gen.FolderHashClient
	scannerGRPCCli      gen.ScannerClient
	errFolderHashNoGRPC = errors.New("folder-hash gRPC worker unavailable")
	errWalkNoGRPC       = errors.New("scanner walk gRPC worker unavailable")
)

// ErrWalkNoGRPC is returned when the scanner Walk RPC cannot be started.
var ErrWalkNoGRPC = errWalkNoGRPC

func ensureScannerGRPC() {
	scannerGRPCOnce.Do(func() {
		binary, err := Resolve()
		if err != nil {
			return
		}
		proc, err := rustworker.StartGRPC(context.Background(), binary, rustworker.DefaultListenAddr("navidrome-scanner"), nil)
		if err != nil {
			rustworker.LogGRPCUnavailable("scanner", err)
			return
		}
		cli := gen.NewFolderHashClient(proc.Conn)
		healthCtx, cancel := context.WithTimeout(context.Background(), rustworker.DefaultGRPCDialTimeout)
		defer cancel()
		if _, err := cli.Health(healthCtx, &gen.HealthRequest{}); err != nil {
			proc.Close()
			rustworker.LogGRPCUnavailable("scanner", err)
			return
		}
		scannerGRPCProc = proc
		folderHashGRPCCli = cli
		scannerGRPCCli = gen.NewScannerClient(proc.Conn)
		if scannerGRPCProc.Cmd != nil && scannerGRPCProc.Cmd.Process != nil {
			log.Info("Scanner hashing and walk routed through Rust gRPC worker", "pid", scannerGRPCProc.Cmd.Process.Pid, "listen", scannerGRPCProc.Addr)
		} else {
			log.Info("Scanner hashing and walk routed through Rust gRPC worker", "listen", scannerGRPCProc.Addr)
		}
	})
}

func folderHashGRPC() gen.FolderHashClient {
	ensureScannerGRPC()
	return folderHashGRPCCli
}

// ScannerGRPC returns the Walk client, or nil when the worker is unavailable.
func ScannerGRPC() gen.ScannerClient {
	ensureScannerGRPC()
	return scannerGRPCCli
}

func hashGRPC(ctx context.Context, request FolderHashRequest) (string, error) {
	cli := folderHashGRPC()
	if cli == nil {
		return "", errFolderHashNoGRPC
	}
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
