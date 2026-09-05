package rustworker

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestWaitReadyParsesBanner(t *testing.T) {
	addr, err := WaitReady(context.Background(), strings.NewReader("READY unix:/tmp/x.sock\n"))
	if err != nil {
		t.Fatal(err)
	}
	if addr != "unix:/tmp/x.sock" {
		t.Fatalf("addr=%q", addr)
	}
}

func TestWaitReadyRejectsBadBanner(t *testing.T) {
	_, err := WaitReady(context.Background(), strings.NewReader("NOTREADY\n"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWaitReadyRespectsCancel(t *testing.T) {
	r, w := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	defer w.Close()
	_, err := WaitReady(ctx, r)
	if err == nil {
		t.Fatal("expected timeout")
	}
}
