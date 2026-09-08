package artwork

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/navidrome/navidrome/conf"
)

func TestImageReadsEnforceLimitDuringReceipt(t *testing.T) {
	old := conf.Server.MaxImageSize
	conf.Server.MaxImageSize = "4 B"
	t.Cleanup(func() { conf.Server.MaxImageSize = old })
	for _, data := range []string{"1234", "123456789"} {
		t.Run(data, func(t *testing.T) {
			reader := bytes.NewBufferString(data)
			got, err := readImageBytes(reader)
			if len(data) == 4 {
				if err != nil || string(got) != data {
					t.Fatalf("exact-limit image = %q, %v", got, err)
				}
			} else if err == nil || got != nil || reader.Len() != len(data)-5 {
				t.Fatalf("overflow must stop after limit+1: data=%q err=%v unread=%d", got, err, reader.Len())
			}

			path := filepath.Join(t.TempDir(), "image")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			got, err = readImageFile(path)
			if (err != nil) != (len(data) > 4) || (len(data) > 4 && got != nil) {
				t.Fatalf("file read limit: data=%q err=%v", got, err)
			}
		})
	}
	if _, err := readImageBytes(failedImageReader{}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read failure lost: %v", err)
	}
}

type failedImageReader struct{}

func (failedImageReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestImageLimitLeavesRoomForOverflowProbe(t *testing.T) {
	old := conf.Server.MaxImageSize
	conf.Server.MaxImageSize = "9223372036854775807 B"
	t.Cleanup(func() { conf.Server.MaxImageSize = old })
	if limit := maxImageReadBytes(); limit <= 0 || limit+1 <= 0 {
		t.Fatalf("overflow probe would wrap: %d", limit)
	}
}
