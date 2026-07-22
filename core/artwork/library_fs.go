package artwork

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/navidrome/navidrome/core/storage"
	"github.com/navidrome/navidrome/model"
)

// libraryView bundles the MusicFS for a library with its absolute root path,
// so readers can open library-relative paths through FS and compose absolute
// paths (for ffmpeg, which is path-based) via Abs.
type libraryView struct {
	FS      storage.MusicFS
	absRoot string
}

func pathWithinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (v libraryView) OpenArtwork(name string) (io.ReadCloser, error) {
	return openArtworkFS(v.FS, v.absRoot, name)
}

func openArtworkFS(fsys fs.FS, rootPath, name string) (io.ReadCloser, error) {
	if resolver, ok := fsys.(storage.SymlinkResolverFS); ok {
		target, err := resolver.ResolveSymlink(name)
		if err != nil {
			return nil, err
		}
		resolvedRoot, err := filepath.EvalSymlinks(rootPath)
		if err != nil {
			return nil, err
		}
		if !pathWithinRoot(resolvedRoot, target) {
			return nil, fmt.Errorf("artwork path %q resolves outside library root", name)
		}
		rel, err := filepath.Rel(resolvedRoot, target)
		if err != nil {
			return nil, err
		}
		root, err := os.OpenRoot(resolvedRoot)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		// Open the resolved relative target through os.Root. If any component is
		// replaced after resolution, Root still rejects an escaping symlink.
		return root.Open(rel)
	}
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Abs returns the absolute path for a library-relative path. Returns "" for an
// empty rel so callers (fromFFmpegTag) can treat it as "no path available".
func (v libraryView) Abs(rel string) string {
	if rel == "" {
		return ""
	}
	return filepath.Join(v.absRoot, rel)
}

// loadLibraryView resolves the MusicFS and absolute root path in a single
// library lookup.
func loadLibraryView(ctx context.Context, ds model.DataStore, libID int) (libraryView, error) {
	lib, err := ds.Library(ctx).Get(libID)
	if err != nil {
		return libraryView{}, err
	}
	s, err := storage.For(lib.Path)
	if err != nil {
		return libraryView{}, err
	}
	fs, err := s.FS()
	if err != nil {
		return libraryView{}, err
	}
	return libraryView{FS: fs, absRoot: lib.Path}, nil
}
