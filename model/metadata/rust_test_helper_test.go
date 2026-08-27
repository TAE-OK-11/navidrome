package metadata_test

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/metadata"
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

func rustMediaFileJSON(tb testing.TB, filePath string, props metadata.Info) string {
	tb.Helper()
	binary, err := metadataworker.Resolve()
	if err != nil {
		tb.Fatalf("resolve metadata worker: %v", err)
	}

	md := metadata.New(filePath, props)
	tags := make(map[string][]string, len(md.All()))
	for key, values := range md.All() {
		tags[string(key)] = values
	}
	lyricsJSON := props.LyricsJSON
	if lyricsJSON == "" {
		lyricsJSON = "[]"
	}

	request, err := json.Marshal(mapMediaRequest{
		Tags:       tags,
		Path:       filePath,
		LyricsJSON: lyricsJSON,
	})
	if err != nil {
		tb.Fatalf("encode map media request: %v", err)
	}

	cmd := exec.Command(binary, "--map-media-worker") //nolint:gosec // test-only resolved binary
	cmd.Stdin = bytes.NewReader(append(request, '\n'))
	output, err := cmd.Output()
	if err != nil {
		tb.Fatalf("map media worker failed: %v", err)
	}

	var response mapMediaResponse
	if err := json.Unmarshal(bytes.TrimSpace(output), &response); err != nil {
		tb.Fatalf("decode map media response: %v", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "map media worker failed"
		}
		tb.Fatal(response.Error)
	}
	if response.MediaFileJSON == "" {
		tb.Fatal("map media worker returned empty media_file_json")
	}
	return response.MediaFileJSON
}

func propsWithRustMediaFile(tb testing.TB, filePath string, props metadata.Info) metadata.Info {
	tb.Helper()
	props.MediaFileJSON = rustMediaFileJSON(tb, filePath, props)
	return props
}

func toMediaFileFromTags(tb testing.TB, filePath string, props metadata.Info, tags model.RawTags) model.MediaFile {
	tb.Helper()
	props.Tags = tags
	props = propsWithRustMediaFile(tb, filePath, props)
	return metadata.New(filePath, props).ToMediaFile(1, "folderID")
}
