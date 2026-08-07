package metadata

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateUTF8BytesPreservesRuneBoundaries(t *testing.T) {
	got := truncateUTF8Bytes("가나다🙂", 7)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated metadata is invalid UTF-8: %q", got)
	}
	if got != "가나" {
		t.Fatalf("unexpected truncation: got %q, want %q", got, "가나")
	}
	if len(got) > 7 {
		t.Fatalf("truncated metadata exceeds byte limit: %d", len(got))
	}
}
