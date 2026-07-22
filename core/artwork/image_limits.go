package artwork

import (
	"bytes"
	"fmt"
	"image"
	"io"
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
