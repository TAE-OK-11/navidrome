package ftsnormalize

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeInProcessMatchesRustSemantics(t *testing.T) {
	t.Parallel()
	out := normalizeInProcess("R.E.M.", "Bjørk")
	if !strings.Contains(out, "REM") || !strings.Contains(out, "Bjork") {
		t.Fatalf("normalizeInProcess = %q, want REM and Bjork variants", out)
	}
}

func TestNormalizeForFTSUsesInProcess(t *testing.T) {
	t.Parallel()
	got := NormalizeForFTS(context.Background(), "AC/DC")
	if got == "" {
		t.Fatal("expected non-empty normalized FTS tokens")
	}
}
