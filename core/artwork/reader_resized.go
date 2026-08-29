package artwork

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gen2brain/webp"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

func init() {
	conf.AddHook(func() {
		// gen2brain/webp selects native (purego/libwebp) vs WASM in its own
		// package init() and exposes the result only via webp.Dynamic(); there is
		// no runtime way to switch back. On 32-bit ARM/x86 the purego callback path
		// crashes (issue #5597), so those builds must be compiled with the
		// "nodynamic" tag (see Dockerfile), which makes webp.Dynamic() report an
		// error here and forces the safe WASM path.
		if err := webp.Dynamic(); err != nil {
			log.Debug("Using WASM WebP encoder/decoder", "reason", err)
		} else {
			log.Debug("Using native libwebp for WebP encoding/decoding")
		}
	})
}

type resizedArtworkReader struct {
	artID      model.ArtworkID
	cacheKey   string
	lastUpdate time.Time
	size       int
	square     bool
	a          *artwork
	original   artworkReader
}

func resizedFromOriginal(ctx context.Context, a *artwork, artID model.ArtworkID, size int, square bool) (*resizedArtworkReader, error) {
	r := &resizedArtworkReader{a: a}
	r.artID = artID
	r.size = size
	r.square = square

	// Get lastUpdated and cacheKey from original artwork
	original, err := a.getArtworkReader(ctx, artID, 0, false)
	if err != nil {
		return nil, err
	}
	r.cacheKey = original.Key()
	r.lastUpdate = original.LastUpdated()
	r.original = original
	return r, nil
}

func (a *resizedArtworkReader) Key() string {
	baseKey := fmt.Sprintf("%s.%d", a.cacheKey, a.size)
	if a.square {
		return baseKey + ".square"
	}
	return fmt.Sprintf("%s.%d", baseKey, conf.Server.CoverArtQuality)
}

func (a *resizedArtworkReader) LastUpdated() time.Time {
	return a.lastUpdate
}

func (a *resizedArtworkReader) Reader(ctx context.Context) (io.ReadCloser, string, error) {
	orig, sourcePath, err := a.original.Reader(ctx)
	if err != nil {
		return nil, "", err
	}
	defer orig.Close()

	var resized io.Reader
	var origSize int
	if absPath, ok := a.resolveLocalImagePath(ctx, sourcePath); ok {
		resized, origSize, err = a.resizeImageFromPath(ctx, absPath)
	} else {
		resized, origSize, err = a.resizeImage(ctx, orig)
	}
	if resized == nil {
		log.Trace(ctx, "Image smaller than requested size", "artID", a.artID, "original", origSize, "resized", a.size, "square", a.square)
	} else {
		log.Trace(ctx, "Resizing artwork", "artID", a.artID, "original", origSize, "resized", a.size, "square", a.square)
	}
	if err != nil {
		log.Warn(ctx, "Could not resize image. Will return image as is", "artID", a.artID, "size", a.size, "square", a.square, err)
	}
	if err != nil || resized == nil {
		// if we couldn't resize the image, return the original
		orig, _, err = a.original.Reader(ctx)
		return orig, "", err
	}
	// Preserve ReadCloser semantics if the resized reader already supports Close
	// (e.g., ffmpeg pipe), otherwise wrap with NopCloser
	if rc, ok := resized.(io.ReadCloser); ok {
		return rc, fmt.Sprintf("%s@%d", a.artID, a.size), nil
	}
	return io.NopCloser(resized), fmt.Sprintf("%s@%d", a.artID, a.size), nil
}

func (a *resizedArtworkReader) resizeImage(ctx context.Context, reader io.Reader) (io.Reader, int, error) {
	maxBytes := maxImageReadBytes()
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, 0, fmt.Errorf("reading image data: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, 0, fmt.Errorf("image exceeds maximum size of %d bytes", maxBytes)
	}

	// Sniff animation before decode — animated WebP/PNG may not decode via Go's image package.
	flags, sniffErr := persistentImageWorkers.sniffAnimation(ctx, data)
	if sniffErr == nil {
		if flags.AnimatedGIF {
			if resized, err := persistentImageWorkers.resizeAnimatedGIF(ctx, data, a.size, conf.Server.CoverArtQuality); err == nil {
				return bytes.NewReader(resized), 0, nil
			} else if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			return nil, 0, fmt.Errorf("Rust animated GIF resize unavailable: %w", err)
		} else if flags.AnimatedWebP {
			if resized, err := persistentImageWorkers.resizeAnimatedWebP(ctx, data, a.size, conf.Server.CoverArtQuality); err == nil {
				return bytes.NewReader(resized), 0, nil
			} else if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			log.Debug(ctx, "Rust animated WebP resize unavailable; returning original bytes", "error", err)
			return bytes.NewReader(data), 0, nil
		} else if flags.AnimatedPNG {
			if resized, err := persistentImageWorkers.resizeAnimatedPNG(ctx, data, a.size, conf.Server.CoverArtQuality); err == nil {
				return bytes.NewReader(resized), 0, nil
			} else if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			log.Debug(ctx, "Rust animated PNG resize unavailable; returning original bytes", "error", err)
			return bytes.NewReader(data), 0, nil
		}
	} else if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	} else {
		log.Debug(ctx, "Rust animation sniff unavailable; treating as static image", "error", sniffErr)
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	if err := ValidateImageConfig(config); err != nil {
		return nil, 0, err
	}

	return resizeStaticImageWithConfigContext(ctx, data, config, format, a.size, a.square)
}

func localImageFilePath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return abs, true
}

func (a *resizedArtworkReader) resolveLocalImagePath(ctx context.Context, path string) (string, bool) {
	if abs, ok := localImageFilePath(path); ok {
		return abs, true
	}
	if path == "" || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return "", false
	}
	libraries, err := a.a.ds.Library(ctx).GetAll()
	if err != nil {
		return "", false
	}
	for _, library := range libraries {
		candidate := filepath.Join(library.Path, filepath.FromSlash(path))
		if abs, ok := localImageFilePath(candidate); ok {
			return abs, true
		}
	}
	return "", false
}

func (a *resizedArtworkReader) resizeImageFromPath(ctx context.Context, path string) (io.Reader, int, error) {
	flags, sniffErr := persistentImageWorkers.sniffAnimationPath(ctx, path)
	if sniffErr == nil {
		if flags.AnimatedGIF {
			resized, err := persistentImageWorkers.resizeAnimatedGIFPath(ctx, path, a.size, conf.Server.CoverArtQuality)
			if err == nil {
				return bytes.NewReader(resized), 0, nil
			}
			if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			return nil, 0, fmt.Errorf("Rust animated GIF resize unavailable: %w", err)
		} else if flags.AnimatedWebP {
			resized, err := persistentImageWorkers.resizeAnimatedWebPPath(ctx, path, a.size, conf.Server.CoverArtQuality)
			if err == nil {
				return bytes.NewReader(resized), 0, nil
			}
			if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			log.Debug(ctx, "Rust animated WebP resize unavailable; returning original bytes", "error", err)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil, 0, readErr
			}
			return bytes.NewReader(data), 0, nil
		} else if flags.AnimatedPNG {
			resized, err := persistentImageWorkers.resizeAnimatedPNGPath(ctx, path, a.size, conf.Server.CoverArtQuality)
			if err == nil {
				return bytes.NewReader(resized), 0, nil
			}
			if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			log.Debug(ctx, "Rust animated PNG resize unavailable; returning original bytes", "error", err)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil, 0, readErr
			}
			return bytes.NewReader(data), 0, nil
		}
	} else if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	} else {
		log.Debug(ctx, "Rust animation sniff unavailable; treating as static image", "error", sniffErr)
	}

	outputFormat := "jpeg"
	if conf.Server.EnableWebPEncoding {
		outputFormat = "webp"
	} else if a.square || strings.EqualFold(filepath.Ext(path), ".png") {
		outputFormat = "png"
	}
	resized, resizeErr := persistentImageWorkers.resizePath(
		ctx,
		path,
		a.size,
		conf.Server.CoverArtQuality,
		a.square,
		outputFormat,
	)
	if resizeErr == nil {
		return bytes.NewReader(resized), 0, nil
	} else if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}
	if strings.Contains(resizeErr.Error(), "does not require resizing") {
		return nil, 0, nil
	}
	return nil, 0, fmt.Errorf("Rust artwork resize unavailable: %w", resizeErr)
}

func resizeStaticImage(data []byte, size int, square bool) (io.Reader, int, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	if err := ValidateImageConfig(config); err != nil {
		return nil, 0, err
	}
	return resizeStaticImageWithConfigContext(context.Background(), data, config, format, size, square)
}

func resizeStaticImageWithConfigContext(ctx context.Context, data []byte, config image.Config, format string, size int, square bool) (io.Reader, int, error) {
	originalSize := max(config.Width, config.Height)
	if size > originalSize {
		size = originalSize
	}
	// Avoid decoding every pixel merely to discover that the original can be
	// returned unchanged. This is common when clients request large covers.
	if originalSize <= size && !square {
		return nil, originalSize, nil
	}

	outputFormat := "jpeg"
	if conf.Server.EnableWebPEncoding {
		outputFormat = "webp"
	} else if format == "png" || square {
		outputFormat = "png"
	}
	resized, resizeErr := persistentImageWorkers.resize(
		ctx,
		data,
		size,
		conf.Server.CoverArtQuality,
		square,
		outputFormat,
	)
	if resizeErr == nil {
		return bytes.NewReader(resized), originalSize, nil
	} else if ctx.Err() != nil {
		return nil, originalSize, ctx.Err()
	}
	return nil, originalSize, fmt.Errorf("Rust artwork resize unavailable: %w", resizeErr)
}
