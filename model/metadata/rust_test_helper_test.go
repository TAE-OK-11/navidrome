package metadata_test

import (
	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/metadata"
)

func rustMediaFileJSON(filePath string, props metadata.Info) (string, error) {
	md := metadata.New(filePath, props)
	tags := make(map[string][]string, len(md.All()))
	for key, values := range md.All() {
		tags[string(key)] = values
	}
	return metadataworker.MapMediaFileJSON(filePath, tags, props.LyricsJSON)
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
