package scanner

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFolderHashInScanTargets(t *testing.T) {
	t.Parallel()
	targets := []string{"The Beatles/Help!", "Pink Floyd"}

	require.True(t, folderHashInScanTargets("The Beatles/Help!", nil))
	require.True(t, folderHashInScanTargets("The Beatles/Help!/01", targets))
	require.True(t, folderHashInScanTargets("Pink Floyd/The Wall", targets))
	require.False(t, folderHashInScanTargets("The Beatles/Revolver", targets))
	require.False(t, folderHashInScanTargets("Other/Artist", targets))
}
