package scanner

import (
	"testing"

	"github.com/navidrome/navidrome/model"
	"github.com/stretchr/testify/require"
)

func TestLibraryRelativePathMatchesRustWalkKeys(t *testing.T) {
	t.Parallel()
	lib := model.Library{ID: 1, Path: "/music"}

	require.Equal(t, ".", model.NewFolder(lib, ".").LibraryRelativePath())
	require.Equal(t, "The Beatles/Help!", model.NewFolder(lib, "The Beatles/Help!").LibraryRelativePath())
	require.Equal(t, "The Beatles/Help!/01", model.NewFolder(lib, "The Beatles/Help!/01").LibraryRelativePath())
}
