package metadataworker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/navidrome/navidrome/core/metadataworker/gen"
	"github.com/navidrome/navidrome/core/rustworker"
	"github.com/navidrome/navidrome/log"
)

var (
	grpcOnce  sync.Once
	grpcProc  *rustworker.GRPCProcess
	grpcCli   gen.MetadataClient
	errNoGRPC = errors.New("metadata gRPC worker unavailable")
)

// ErrNoGRPC is returned when the metadata gRPC worker is not running.
var ErrNoGRPC = errNoGRPC

func grpcClient() gen.MetadataClient {
	grpcOnce.Do(func() {
		binary, err := Resolve()
		if err != nil {
			return
		}
		proc, err := rustworker.StartGRPC(context.Background(), binary, rustworker.DefaultListenAddr("navidrome-metadata"), nil)
		if err != nil {
			log.Warn("Rust metadata gRPC worker unavailable; using NDJSON fallback", err)
			return
		}
		cli := gen.NewMetadataClient(proc.Conn)
		healthCtx, cancel := context.WithTimeout(context.Background(), rustworker.DefaultGRPCDialTimeout)
		defer cancel()
		if _, err := cli.Health(healthCtx, &gen.HealthRequest{}); err != nil {
			proc.Close()
			log.Warn("Rust metadata gRPC health failed; using NDJSON fallback", err)
			return
		}
		grpcProc = proc
		grpcCli = cli
		if grpcProc.Cmd != nil && grpcProc.Cmd.Process != nil {
			log.Info("Metadata extract routed through Rust gRPC worker", "pid", grpcProc.Cmd.Process.Pid, "listen", grpcProc.Addr)
		} else {
			log.Info("Metadata extract routed through Rust gRPC worker", "listen", grpcProc.Addr)
		}
	})
	return grpcCli
}

func tagsToProto(tags map[string][]string) map[string]*gen.StringList {
	if len(tags) == 0 {
		return nil
	}
	out := make(map[string]*gen.StringList, len(tags))
	for key, values := range tags {
		out[key] = &gen.StringList{Values: append([]string(nil), values...)}
	}
	return out
}

func tagsFromProto(tags map[string]*gen.StringList) map[string][]string {
	if len(tags) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(tags))
	for key, list := range tags {
		if list == nil {
			continue
		}
		out[key] = append([]string(nil), list.Values...)
	}
	return out
}

func mappingsToProto(mappings map[string]TagMappingExport) map[string]*gen.TagMapping {
	if len(mappings) == 0 {
		return nil
	}
	out := make(map[string]*gen.TagMapping, len(mappings))
	for name, mapping := range mappings {
		out[name] = &gen.TagMapping{
			Aliases:   append([]string(nil), mapping.Aliases...),
			Type:      mapping.Type,
			MaxLength: int32(mapping.MaxLength),
			Split:     append([]string(nil), mapping.Split...),
			Album:     mapping.Album,
		}
	}
	return out
}

func Extract(ctx context.Context, files []ExtractFile, cfg WorkerScanConfig) (*gen.ExtractResponse, error) {
	return extractGRPC(ctx, files, cfg)
}

type ExtractFile struct {
	Key  string
	Path string
}

func extractGRPC(ctx context.Context, files []ExtractFile, cfg WorkerScanConfig) (*gen.ExtractResponse, error) {
	cli := grpcClient()
	if cli == nil {
		return nil, errNoGRPC
	}
	protoFiles := make([]*gen.ExtractFile, len(files))
	for i, file := range files {
		protoFiles[i] = &gen.ExtractFile{Key: file.Key, Path: file.Path}
	}
	var pidJSON string
	if len(cfg.PIDConfig) > 0 {
		raw, err := json.Marshal(cfg.PIDConfig)
		if err != nil {
			return nil, err
		}
		pidJSON = string(raw)
	}
	resp, err := cli.Extract(ctx, &gen.ExtractRequest{
		Files:                 protoFiles,
		TagMappings:           mappingsToProto(cfg.TagMappings),
		ArtistSplitExceptions: append([]string(nil), cfg.ArtistSplitExceptions...),
		ArtistsSplit:          append([]string(nil), cfg.ArtistsSplit...),
		RolesSplit:            append([]string(nil), cfg.RolesSplit...),
		ArtistJoiner:          cfg.ArtistJoiner,
		PidConfigJson:         pidJSON,
		LibraryId:             int32(cfg.LibraryID),
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func cleanTagsGRPC(ctx context.Context, filePath string, tags map[string][]string, mappings map[string]TagMappingExport, exceptions []string) (map[string][]string, error) {
	cli := grpcClient()
	if cli == nil {
		return nil, errNoGRPC
	}
	resp, err := cli.CleanTags(ctx, &gen.CleanTagsRequest{
		Path:                  filePath,
		Tags:                  tagsToProto(tags),
		Mappings:              mappingsToProto(mappings),
		ArtistSplitExceptions: exceptions,
	})
	if err != nil {
		return nil, err
	}
	if !resp.GetOk() {
		return nil, errors.New(nonEmpty(resp.GetError(), "Rust clean tags failed"))
	}
	return tagsFromProto(resp.GetTags()), nil
}

func mapMediaGRPC(ctx context.Context, path string, tags map[string][]string, lyricsJSON string) (string, error) {
	cli := grpcClient()
	if cli == nil {
		return "", errNoGRPC
	}
	resp, err := cli.MapMedia(ctx, &gen.MapMediaRequest{
		Tags:       tagsToProto(tags),
		Path:       path,
		LyricsJson: lyricsJSON,
	})
	if err != nil {
		return "", err
	}
	if !resp.GetOk() {
		return "", errors.New(nonEmpty(resp.GetError(), "Rust map media failed"))
	}
	if resp.GetMediaFileJson() == "" {
		return "", errors.New("map media worker returned empty media_file_json")
	}
	return resp.GetMediaFileJson(), nil
}

func parseLyricsGRPC(ctx context.Context, suffix, lang string, contents []byte) (string, error) {
	cli := grpcClient()
	if cli == nil {
		return "", errNoGRPC
	}
	resp, err := cli.ParseLyrics(ctx, &gen.ParseLyricsRequest{Suffix: suffix, Lang: lang, Contents: contents})
	if err != nil {
		return "", err
	}
	if !resp.GetOk() {
		return "", errors.New(nonEmpty(resp.GetError(), "Rust lyrics worker could not parse lyrics"))
	}
	if resp.GetLyricsJson() == "" {
		return "[]", nil
	}
	return resp.GetLyricsJson(), nil
}

func normalizeFTSGRPC(ctx context.Context, values []string) (string, error) {
	cli := grpcClient()
	if cli == nil {
		return "", errNoGRPC
	}
	resp, err := cli.NormalizeFts(ctx, &gen.NormalizeFtsRequest{Values: values})
	if err != nil {
		return "", err
	}
	if !resp.GetOk() {
		return "", errors.New(nonEmpty(resp.GetError(), "Rust normalize worker failed"))
	}
	return resp.GetNormalized(), nil
}

func buildFTS5QueryGRPC(ctx context.Context, query string) (buildFTS5QueryResult, error) {
	cli := grpcClient()
	if cli == nil {
		return buildFTS5QueryResult{}, errNoGRPC
	}
	resp, err := cli.BuildFts5Query(ctx, &gen.BuildFts5QueryRequest{Query: query})
	if err != nil {
		return buildFTS5QueryResult{}, err
	}
	if !resp.GetOk() {
		return buildFTS5QueryResult{}, errors.New(nonEmpty(resp.GetError(), "Rust build FTS5 query worker failed"))
	}
	return buildFTS5QueryResult{Query: resp.GetQuery(), Degraded: resp.GetDegraded()}, nil
}

// ProcessImage is the gRPC image-resize/sniff entry used by the artwork package.
func ProcessImage(ctx context.Context, req *gen.ImageRequest) (*gen.ImageResponse, error) {
	cli := grpcClient()
	if cli == nil {
		return nil, errNoGRPC
	}
	resp, err := cli.ProcessImage(ctx, req)
	if err != nil {
		return nil, err
	}
	if !resp.GetOk() {
		return nil, errors.New(nonEmpty(resp.GetError(), "Rust image worker request failed"))
	}
	return resp, nil
}

func ExtractPicture(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	cli := grpcClient()
	if cli == nil {
		return nil, errNoGRPC
	}
	resp, err := cli.ExtractPicture(ctx, &gen.ExtractPictureRequest{Path: path, MaxBytes: maxBytes})
	if err != nil {
		return nil, err
	}
	if !resp.GetOk() {
		return nil, errors.New(nonEmpty(resp.GetError(), "metadata worker could not extract artwork"))
	}
	return resp.GetBody(), nil
}

func nonEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
