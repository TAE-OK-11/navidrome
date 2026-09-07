package scanner

import (
	"github.com/navidrome/navidrome/model"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestKnownFolderHashesPreservesSiblingAndRootPaths(t *testing.T) {
	updates := map[string]model.FolderUpdateInfo{
		"root":  {FullPath: ".", Hash: "root-hash"},
		"a":     {FullPath: "Artist/Album A", Hash: "a-hash"},
		"b":     {FullPath: "Artist/Album B", Hash: "b-hash"},
		"empty": {FullPath: "Empty"},
	}
	require.Equal(t, map[string]string{".": "root-hash", "Artist/Album A": "a-hash", "Artist/Album B": "b-hash"}, knownFolderHashes(updates, false))
	require.Nil(t, knownFolderHashes(updates, true), "full scans need complete file lists even for unchanged hashes")
}
