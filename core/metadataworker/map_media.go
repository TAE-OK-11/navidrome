package metadataworker

import (
	"bufio"
	"context"
	"errors"
	"fmt"

	"github.com/navidrome/navidrome/core/rustworker"
)

type mapMediaRequest struct {
	Tags       map[string][]string `json:"tags"`
	Path       string              `json:"path"`
	LyricsJSON string              `json:"lyrics_json,omitempty"`
}

type mapMediaResponse struct {
	OK            bool   `json:"ok"`
	MediaFileJSON string `json:"media_file_json,omitempty"`
	Error         string `json:"error,omitempty"`
}

// MapMediaFileJSON runs the Rust map-media worker for tests and fake extractors.
func MapMediaFileJSON(path string, tags map[string][]string, lyricsJSON string) (string, error) {
	binary, err := Resolve()
	if err != nil {
		return "", fmt.Errorf("resolve metadata worker: %w", err)
	}
	if lyricsJSON == "" {
		lyricsJSON = "[]"
	}
	if mediaJSON, err := mapMediaGRPC(context.Background(), path, tags, lyricsJSON); rustworker.PreferGRPC(err, errNoGRPC) {
		return mediaJSON, err
	}
	pipes, err := rustworker.Start(binary, "--map-media-worker")
	if err != nil {
		return "", err
	}
	defer rustworker.Close(pipes)

	writer := bufio.NewWriter(pipes.Stdin)
	if err := rustworker.WriteJSONLine(writer, mapMediaRequest{
		Tags:       tags,
		Path:       path,
		LyricsJSON: lyricsJSON,
	}); err != nil {
		return "", err
	}

	var response mapMediaResponse
	if err := rustworker.ReadJSONLine(bufio.NewReader(pipes.Stdout), &response); err != nil {
		return "", err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "map media worker failed"
		}
		return "", errors.New(response.Error)
	}
	if response.MediaFileJSON == "" {
		return "", errors.New("map media worker returned empty media_file_json")
	}
	return response.MediaFileJSON, nil
}
