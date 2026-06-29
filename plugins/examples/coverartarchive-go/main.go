package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/navidrome/navidrome/plugins/pdk/go/host"
	"github.com/navidrome/navidrome/plugins/pdk/go/metadata"
)

type coverArtArchivePlugin struct{}

func init() {
	metadata.Register(&coverArtArchivePlugin{})
}

var _ metadata.AlbumImagesProvider = (*coverArtArchivePlugin)(nil)

type caaResponse struct {
	Images []caaImage `json:"images"`
}

type caaImage struct {
	Front      bool              `json:"front"`
	Types      []string          `json:"types"`
	Image      string            `json:"image"`
	Thumbnails map[string]string `json:"thumbnails"`
}

func (*coverArtArchivePlugin) GetAlbumImages(input metadata.AlbumRequest) (*metadata.AlbumImagesResponse, error) {
	if input.MBID == "" {
		return nil, errors.New("not found: MBID required")
	}

	resp, err := host.HTTPSend(host.HTTPRequest{
		Method: "GET",
		URL:    "https://coverartarchive.org/release/" + input.MBID,
		Headers: map[string]string{
			"Accept":     "application/json",
			"User-Agent": "NavidromeCoverArtArchivePlugin/1.0",
		},
		TimeoutMs: 10000,
	})
	if err != nil {
		return nil, fmt.Errorf("not found: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("not found: CAA returned status %d", resp.StatusCode)
	}

	var parsed caaResponse
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, fmt.Errorf("not found: invalid JSON response: %w", err)
	}

	image := frontImage(parsed.Images)
	if image == nil {
		return nil, errors.New("not found: no front cover image")
	}

	images := imageList(*image)
	if len(images) == 0 {
		return nil, errors.New("not found: no usable image URLs")
	}

	return &metadata.AlbumImagesResponse{Images: images}, nil
}

func frontImage(images []caaImage) *caaImage {
	for i := range images {
		if images[i].Front {
			return &images[i]
		}
	}
	for i := range images {
		for _, typ := range images[i].Types {
			if typ == "Front" {
				return &images[i]
			}
		}
	}
	if len(images) > 0 {
		return &images[0]
	}
	return nil
}

func imageList(img caaImage) []metadata.ImageInfo {
	result := make([]metadata.ImageInfo, 0, len(img.Thumbnails)+1)
	for sizeName, url := range img.Thumbnails {
		if url == "" {
			continue
		}
		var size int32
		switch sizeName {
		case "small":
			size = 250
		case "large":
			size = 500
		default:
			var parsed int
			if _, err := fmt.Sscanf(sizeName, "%d", &parsed); err != nil || parsed <= 0 {
				continue
			}
			size = int32(parsed)
		}
		result = append(result, metadata.ImageInfo{URL: url, Size: size})
	}
	if len(result) == 0 && img.Image != "" {
		result = append(result, metadata.ImageInfo{URL: img.Image})
	}
	return result
}

func main() {}
