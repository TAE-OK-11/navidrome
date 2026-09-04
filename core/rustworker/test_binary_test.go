package rustworker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureTestBinaryFindsWorkspaceRelease(t *testing.T) {
	root, err := RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(root, "rust", "target", "release", BinaryName("navidrome-metadata"))
	if _, err := os.Stat(candidate); err != nil {
		t.Skip("workspace metadata binary not built")
	}

	const envKey = "ND_RUSTWORKER_TEST_BINARY_PROBE"
	t.Setenv(envKey, "")
	if err := EnsureTestBinary(envKey, "navidrome-metadata", BinaryName("navidrome-metadata")); err != nil {
		t.Fatal(err)
	}
	got := os.Getenv(envKey)
	if got != candidate {
		t.Fatalf("EnsureTestBinary() set %q, want %q", got, candidate)
	}
}
