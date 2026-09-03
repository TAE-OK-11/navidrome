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
	folderHashGRPCOnce  sync.Once
	folderHashGRPCProc  *rustworker.GRPCProcess
	folderHashGRPCCli   gen.FolderHashClient
	errFolderHashNoGRPC = errors.New("folder-hash gRPC worker unavailable")
)

func folderHashGRPC() gen.FolderHashClient {
	folderHashGRPCOnce.Do(func() {
		binary, err := Resolve()
		if err != nil {
			return
		}
		proc, err := rustworker.StartGRPC(context.Background(), binary, rustworker.DefaultListenAddr("navidrome-folder-hash"), nil)
		if err != nil {
			rustworker.LogGRPCUnavailable("folder-hash", err)
			return
		}
		cli := gen.NewFolderHashClient(proc.Conn)
		healthCtx, cancel := context.WithTimeout(context.Background(), rustworker.DefaultGRPCDialTimeout)
		defer cancel()
		if _, err := cli.Health(healthCtx, &gen.HealthRequest{}); err != nil {
			proc.Close()
			rustworker.LogGRPCUnavailable("folder-hash", err)
			return
		}
		folderHashGRPCProc = proc
		folderHashGRPCCli = cli
		if folderHashGRPCProc.Cmd != nil && folderHashGRPCProc.Cmd.Process != nil {
			log.Info("Folder hashing routed through Rust gRPC worker", "pid", folderHashGRPCProc.Cmd.Process.Pid, "listen", folderHashGRPCProc.Addr)
		} else {
			log.Info("Folder hashing routed through Rust gRPC worker", "listen", folderHashGRPCProc.Addr)
		}
	})
	return folderHashGRPCCli
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
