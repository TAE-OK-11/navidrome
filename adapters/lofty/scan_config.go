package lofty

import (
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/model"
)

func workerScanConfig(libraryID int) metadataworker.WorkerScanConfig {
	mappings := model.TagMappings()
	tagMappings := make(map[string]metadataworker.TagMappingExport, len(mappings))
	for name, mapping := range mappings {
		tagMappings[string(name)] = metadataworker.TagMappingExport{
			Aliases:   append([]string(nil), mapping.Aliases...),
			Type:      string(mapping.Type),
			MaxLength: mapping.MaxLength,
			Split:     append([]string(nil), mapping.Split...),
			Album:     mapping.Album,
		}
	}
	return metadataworker.WorkerScanConfig{
		TagMappings:           tagMappings,
		ArtistSplitExceptions: append([]string(nil), conf.Server.Scanner.ArtistSplitExceptions...),
		PIDConfig: map[string]any{
			"track":                conf.Server.PID.Track,
			"album":                conf.Server.PID.Album,
			"group_album_releases": conf.Server.Scanner.GroupAlbumReleases,
		},
		LibraryID: libraryID,
	}
}

func exportTagMappings() map[string]metadataworker.TagMappingExport {
	return workerScanConfig(0).TagMappings
}
