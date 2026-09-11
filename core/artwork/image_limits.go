package artwork

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/metadataworker"
	_ "golang.org/x/image/webp"
)

const (
	MaxImageDimension = 16_384
	MaxImagePixels    = 40_000_000
)

func ValidateImageConfig(config image.Config) error {
	if config.Width <= 0 || config.Height <= 0 {
		return fmt.Errorf("invalid image dimensions %dx%d", config.Width, config.Height)
	}
	if config.Width > MaxImageDimension || config.Height > MaxImageDimension {
		return fmt.Errorf("image dimensions %dx%d exceed maximum dimension %d", config.Width, config.Height, MaxImageDimension)
	}
	pixels := int64(config.Width) * int64(config.Height)
	if pixels > MaxImagePixels {
		return fmt.Errorf("image dimensions %dx%d exceed maximum pixel count %d", config.Width, config.Height, MaxImagePixels)
	}
	return nil
}

// DecodeImage validates dimensions before the decoder allocates a full pixel
// buffer. TeeReader preserves the bytes consumed by DecodeConfig so a
// non-seekable network or archive reader can be decoded without reopening it.
func DecodeImage(reader io.Reader) (image.Image, string, error) {
	var prefix bytes.Buffer
	config, _, err := image.DecodeConfig(io.TeeReader(reader, &prefix))
	if err != nil {
		return nil, "", err
	}
	if err := ValidateImageConfig(config); err != nil {
		return nil, "", err
	}
	return image.Decode(io.MultiReader(bytes.NewReader(prefix.Bytes()), reader))
}

func maxImageReadBytes() int64 {
	raw := consts.DefaultMaxImageSize
	if conf.Server != nil && conf.Server.MaxImageSize != "" {
		raw = conf.Server.MaxImageSize
	}
	size, err := humanize.ParseBytes(raw)
	if err != nil || size == 0 || size >= ^uint64(0)>>1 {
		return 20 << 20
	}
	return int64(size)
}

func capImageReader(r io.ReadCloser) io.ReadCloser {
	return struct {
		io.Reader
		io.Closer
	}{Reader: io.LimitReader(r, maxImageReadBytes()), Closer: r}
}

// readImageBytes enforces the cap while receiving, including readers whose size
// is unknown or changes. One extra byte distinguishes an exact fit from overflow.
func readImageBytes(reader io.Reader) ([]byte, error) {
	maxBytes := maxImageReadBytes()
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading image data: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("image exceeds maximum size of %d bytes", maxBytes)
	}
	return data, nil
}

func readImageFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readImageBytes(file)
}

// maxValidateGRPCBytes keeps ProcessImage validate unary payloads under the
// shared 64MiB local IPC message limit (see rustworker.maxGRPCMsgSize).
const maxValidateGRPCBytes = (64 << 20) - (1 << 20)

// ValidateUploadedImage inspects image headers via metadata ProcessImage
// (validate mode). Falls back to Go image.DecodeConfig when the worker is
// unavailable, the payload exceeds the gRPC budget, or image-rs cannot parse a
// header-only / truncated body that DecodeConfig still accepts. Worker
// dimension-limit rejections are preserved (no Go fallback).
func ValidateUploadedImage(ctx context.Context, data []byte) (ImageInfo, error) {
	if len(data) == 0 {
		return ImageInfo{}, fmt.Errorf("empty image payload")
	}
	if len(data) <= maxValidateGRPCBytes {
		info, err := persistentImageWorkers.validateImage(ctx, data)
		if err == nil {
			if err := ValidateImageConfig(image.Config{Width: info.Width, Height: info.Height}); err != nil {
				return ImageInfo{}, err
			}
			if info.Format == "" {
				return ImageInfo{}, fmt.Errorf("could not determine image type")
			}
			return info, nil
		}
		// Prefer the worker, but keep Go DecodeConfig parity for header-only /
		// truncated payloads that image-rs cannot dimension yet. Preserve worker
		// dimension-limit rejections (no Go fallback).
		if errors.Is(err, metadataworker.ErrNoGRPC) || shouldFallbackValidate(err) {
			return validateUploadedImageGo(data)
		}
		return ImageInfo{}, err
	}
	return validateUploadedImageGo(data)
}

func shouldFallbackValidate(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "exceed") {
		return false
	}
	return true
}

func validateUploadedImageGo(data []byte) (ImageInfo, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ImageInfo{}, err
	}
	if err := ValidateImageConfig(config); err != nil {
		return ImageInfo{}, err
	}
	if format == "" {
		return ImageInfo{}, fmt.Errorf("could not determine image type")
	}
	return ImageInfo{Width: config.Width, Height: config.Height, Format: format}, nil
}
