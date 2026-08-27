package metadata_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"

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

func rustMediaFileJSON(filePath string, props metadata.Info) (string, error) {
	binary, err := metadataworker.Resolve()
	if err != nil {
		return "", fmt.Errorf("resolve metadata worker: %w", err)
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
		return "", fmt.Errorf("encode map media request: %w", err)
	}

	cmd := exec.Command(binary, "--map-media-worker") //nolint:gosec // test-only resolved binary
	cmd.Stdin = bytes.NewReader(append(request, '\n'))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("map media worker failed: %w", err)
	}

	var response mapMediaResponse
	if err := json.Unmarshal(bytes.TrimSpace(output), &response); err != nil {
		return "", fmt.Errorf("decode map media response: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "map media worker failed"
		}
		return "", fmt.Errorf("%s", response.Error)
	}
	if response.MediaFileJSON == "" {
		return "", fmt.Errorf("map media worker returned empty media_file_json")
	}
	return response.MediaFileJSON, nil
}

func propsWithRustMediaFile(filePath string, props metadata.Info) (metadata.Info, error) {
	jsonPayload, err := rustMediaFileJSON(filePath, props)
	if err != nil {
		return props, err
	}
	props.MediaFileJSON = jsonPayload
	return props, nil
}

func toMediaFileFromTags(filePath string, props metadata.Info, tags model.RawTags) (model.MediaFile, error) {
	props.Tags = tags
	props, err := propsWithRustMediaFile(filePath, props)
	if err != nil {
		return model.MediaFile{}, err
	}
	return metadata.New(filePath, props).ToMediaFile(1, "folderID"), nil
}
