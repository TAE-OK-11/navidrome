package scanner

import (
	"math"
	"testing"
)

func TestValidateRustFoldersRejectsOversizedFiles(t *testing.T) {
	t.Parallel()

	err := validateRustFolder(&rustScanFolder{
		Path: "Album",
		AudioFiles: map[string]rustScanFile{
			"huge.flac": {Name: "huge.flac", Size: math.MaxUint64},
		},
	})
	if err == nil {
		t.Fatal("expected oversized audio file to fail validation")
	}
}
