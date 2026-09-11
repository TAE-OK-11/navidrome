package artwork

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"

	"github.com/navidrome/navidrome/tests"
)

func TestValidateUploadedImageAcceptsSmallPNG(t *testing.T) {
	tests.Init(t, false)
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	info, err := ValidateUploadedImage(context.Background(), buf.Bytes())
	if err != nil {
		t.Fatalf("expected validate success: %v", err)
	}
	if info.Width != 32 || info.Height != 24 || info.Format != "png" {
		t.Fatalf("unexpected info %+v", info)
	}
}

func TestValidateUploadedImageRejectsHugeDimensions(t *testing.T) {
	tests.Init(t, false)
	_, err := ValidateUploadedImage(context.Background(), syntheticPNGHeader(10_000, 5_000))
	if err == nil {
		t.Fatal("expected pixel-budget rejection")
	}
}
