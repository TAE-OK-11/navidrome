package artwork

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/utils/slice"
)

type playlistArtworkReader struct {
	cacheKey
	a  *artwork
	pl model.Playlist
}

const tileSize = 600

func newPlaylistArtworkReader(ctx context.Context, artwork *artwork, artID model.ArtworkID) (*playlistArtworkReader, error) {
	pl, err := artwork.ds.Playlist(ctx).Get(artID.ID)
	if err != nil {
		return nil, err
	}
	a := &playlistArtworkReader{
		a:  artwork,
		pl: *pl,
	}
	a.cacheKey.artID = artID
	a.cacheKey.lastUpdate = pl.UpdatedAt

	// Check sidecar and ExternalImageURL local file ModTimes for cache invalidation.
	// If either is newer than the playlist's UpdatedAt, use that instead so the
	// cache is busted when a user replaces a sidecar image or local file reference.
	for _, path := range []string{
		findPlaylistSidecarPath(ctx, pl.Path),
		pl.ExternalImageURL,
	} {
		if path == "" || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			continue
		}
		if info, err := os.Stat(path); err == nil {
			if info.ModTime().After(a.cacheKey.lastUpdate) {
				a.cacheKey.lastUpdate = info.ModTime()
			}
		}
	}

	return a, nil
}

func (a *playlistArtworkReader) LastUpdated() time.Time {
	return a.lastUpdate
}

func (a *playlistArtworkReader) Reader(ctx context.Context) (io.ReadCloser, string, error) {
	return selectImageReader(ctx, a.artID,
		a.fromPlaylistUploadedImage(),
		a.fromPlaylistSidecar(ctx),
		a.fromPlaylistExternalImage(ctx),
		a.fromGeneratedTiledCover(ctx),
		fromAlbumPlaceholder(),
	)
}

func (a *playlistArtworkReader) fromPlaylistUploadedImage() sourceFunc {
	return fromLocalFile(a.pl.UploadedImagePath())
}

func (a *playlistArtworkReader) fromPlaylistSidecar(ctx context.Context) sourceFunc {
	return fromLibraryLocalFile(ctx, a.a.ds, findPlaylistSidecarPath(ctx, a.pl.Path))
}

func (a *playlistArtworkReader) fromPlaylistExternalImage(ctx context.Context) sourceFunc {
	return func() (io.ReadCloser, string, error) {
		imgURL := a.pl.ExternalImageURL
		if imgURL == "" {
			return nil, "", nil
		}
		parsed, err := url.Parse(imgURL)
		if err != nil {
			return nil, "", err
		}
		if parsed.Scheme == "http" || parsed.Scheme == "https" {
			if !conf.Server.EnableM3UExternalAlbumArt {
				return nil, "", nil
			}
			return fromURL(ctx, parsed)
		}
		return fromLibraryLocalFile(ctx, a.a.ds, imgURL)()
	}
}

func fromLibraryLocalFile(ctx context.Context, ds model.DataStore, path string) sourceFunc {
	return func() (io.ReadCloser, string, error) {
		if path == "" {
			return nil, "", nil
		}
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, "", err
		}
		libraries, err := ds.Library(ctx).GetAll()
		if err != nil {
			return nil, "", err
		}
		for _, library := range libraries {
			resolvedRoot, rootErr := filepath.EvalSymlinks(library.Path)
			if rootErr != nil || !pathWithinRoot(resolvedRoot, target) {
				continue
			}
			rel, relErr := filepath.Rel(resolvedRoot, target)
			if relErr != nil {
				continue
			}
			root, rootErr := os.OpenRoot(resolvedRoot)
			if rootErr != nil {
				continue
			}
			f, openErr := root.Open(rel)
			_ = root.Close()
			return f, target, openErr
		}
		return nil, "", fmt.Errorf("artwork path %q resolves outside accessible library roots", path)
	}
}

// fromLocalFile returns a sourceFunc that opens the given local path.
// Returns (nil, "", nil) if path is empty — signalling "not found, try next source".
func fromLocalFile(path string) sourceFunc {
	return func() (io.ReadCloser, string, error) {
		if path == "" {
			return nil, "", nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, "", err
		}
		return f, path, nil
	}
}

// findPlaylistSidecarPath scans the directory of the playlist file for a sidecar
// image file with the same base name (case-insensitive). Returns empty string if
// no matching image is found or if plsPath is empty.
func findPlaylistSidecarPath(ctx context.Context, plsPath string) string {
	if plsPath == "" {
		return ""
	}
	dir := filepath.Dir(plsPath)
	base := strings.TrimSuffix(filepath.Base(plsPath), filepath.Ext(plsPath))

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Warn(ctx, "Could not read directory for playlist sidecar", "dir", dir, err)
		return ""
	}
	for _, entry := range entries {
		name := entry.Name()
		nameBase := strings.TrimSuffix(name, filepath.Ext(name))
		if !entry.IsDir() && strings.EqualFold(nameBase, base) && model.IsImageFile(name) {
			return filepath.Join(dir, name)
		}
	}
	return ""
}

func (a *playlistArtworkReader) fromGeneratedTiledCover(ctx context.Context) sourceFunc {
	return func() (io.ReadCloser, string, error) {
		payloads, err := a.loadTilePayloads(ctx)
		if err != nil {
			return nil, "", err
		}
		if out, err := persistentImageWorkers.mosaic(ctx, payloads, tileSize, conf.Server.CoverArtQuality, "png"); err == nil {
			return io.NopCloser(bytes.NewReader(out)), "", nil
		} else if ctx.Err() != nil {
			return nil, "", ctx.Err()
		} else {
			return nil, "", fmt.Errorf("Rust playlist mosaic unavailable: %w", err)
		}
	}
}

func toAlbumArtworkIDs(albumIDs []string) []model.ArtworkID {
	return slice.Map(albumIDs, func(id string) model.ArtworkID {
		al := model.Album{ID: id}
		return al.CoverArtID()
	})
}

func (a *playlistArtworkReader) loadTilePayloads(ctx context.Context) ([][]byte, error) {
	tracksRepo := a.a.ds.Playlist(ctx).Tracks(a.pl.ID, false)
	albumIds, err := tracksRepo.GetAlbumIDs(model.QueryOptions{Max: 4, Sort: "random()"})
	if err != nil {
		log.Error(ctx, "Error getting album IDs for playlist", "id", a.pl.ID, "name", a.pl.Name, err)
		return nil, err
	}
	ids := toAlbumArtworkIDs(albumIds)

	var payloads [][]byte
	for _, id := range ids {
		r, _, err := fromAlbum(ctx, a.a, id)()
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(r, maxImageReadBytes()))
			_ = r.Close()
			if readErr == nil && len(data) > 0 {
				payloads = append(payloads, data)
			}
		}
		if len(payloads) == 4 {
			break
		}
	}
	switch len(payloads) {
	case 0:
		return nil, errors.New("could not find any eligible cover")
	case 2:
		payloads = append(payloads, payloads[1], payloads[0])
	case 3:
		payloads = append(payloads, payloads[0])
	}
	return payloads, nil
}
