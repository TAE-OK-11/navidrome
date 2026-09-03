package rustworker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStartGRPCSkippedInTests(t *testing.T) {
	if os.Getenv("ND_GRPCWORKERINTESTS") != "" {
		t.Skip("ND_GRPCWORKERINTESTS is set")
	}
	_, err := StartGRPC(context.Background(), "true", DefaultListenAddr("navidrome-grpc-test"), nil)
	if !errors.Is(err, ErrSkippedInTests) {
		t.Fatalf("StartGRPC in tests: got %v, want ErrSkippedInTests", err)
	}
}

func TestUnlinkUnixListenOnlyRemovesTempDirSockets(t *testing.T) {
	outside := "/etc/hostname"
	info, err := os.Stat(outside)
	if err != nil {
		t.Skip(err)
	}
	UnlinkUnixListen("unix:" + outside)
	got, err := os.Stat(outside)
	if err != nil || got.Size() != info.Size() {
		t.Fatalf("must not remove sockets outside TempDir: %v", err)
	}

	inside := filepath.Join(os.TempDir(), fmt.Sprintf("navidrome-grpc-unlink-test-%d.sock", os.Getpid()))
	if err := os.WriteFile(inside, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(inside) })
	UnlinkUnixListen("unix:" + inside)
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Fatalf("stale socket under TempDir should be removed, stat=%v", err)
	}
}
