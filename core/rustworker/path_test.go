package rustworker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBinaryConfiguredAbsolute(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "navidrome-metadata")
	if err := os.WriteFile(binary, []byte{0}, 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveBinary(binary, "navidrome-metadata")
	if err != nil {
		t.Fatalf("ResolveBinary: %v", err)
	}
	if resolved != binary {
		t.Fatalf("resolved = %q, want %q", resolved, binary)
	}
}

func TestResolveBinaryMissingAbsolute(t *testing.T) {
	_, err := ResolveBinary("/no/such/binary", "navidrome-metadata")
	if err == nil {
		t.Fatal("expected error for missing absolute path")
	}
}
