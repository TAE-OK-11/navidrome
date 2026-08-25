package scanner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os/exec"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/scannerworker"
)

const maxRustScanEntries = 10_000_000

type rustScanRequest struct {
	Root             string   `json:"root"`
	Targets          []string `json:"targets"`
	FollowSymlinks   bool     `json:"follow_symlinks"`
	IgnoreDotFolders bool     `json:"ignore_dot_folders"`
}

type rustScanEvent struct {
	Kind    string          `json:"kind"`
	Folder  *rustScanFolder `json:"folder"`
	Message string          `json:"message"`
}

type rustScanFolder struct {
	Path              string                  `json:"path"`
	ModTimeNS         int64                   `json:"mod_time_ns"`
	ImagesUpdatedAtNS int64                   `json:"images_updated_at_ns"`
	NumPlaylists      int                     `json:"num_playlists"`
	NumSubfolders     int                     `json:"num_subfolders"`
	AudioFiles        map[string]rustScanFile `json:"audio_files"`
	ImageFiles        map[string]rustScanFile `json:"image_files"`
}

type rustScanFile struct {
	Name      string `json:"name"`
	Size      uint64 `json:"size"`
	ModTimeNS int64  `json:"mod_time_ns"`
}

func collectRustFolders(ctx context.Context, job *scanJob, targets []string) ([]rustScanFolder, []string, error) {
	binary, err := scannerworker.Resolve()
	if err != nil {
		return nil, nil, err
	}
	request, err := json.Marshal(rustScanRequest{
		Root:             job.localRoot,
		Targets:          targets,
		FollowSymlinks:   conf.Server.Scanner.FollowSymlinks,
		IgnoreDotFolders: conf.Server.Scanner.IgnoreDotFolders,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("encoding Rust scanner request: %w", err)
	}
	request = append(request, '\n')

	cmd := exec.CommandContext(ctx, binary) //nolint:gosec // resolved administrator-controlled binary
	cmd.Stdin = bytes.NewReader(request)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("opening Rust scanner output: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("starting Rust scanner %q: %w", binary, err)
	}

	decoder := json.NewDecoder(bufio.NewReaderSize(stdout, 256*1024))
	folders := make([]rustScanFolder, 0, 1024)
	var warnings []string
	done := false
	for {
		var event rustScanEvent
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, nil, fmt.Errorf("decoding Rust scanner response: %w", err)
		}
		switch event.Kind {
		case "folder":
			if event.Folder == nil || event.Folder.Path == "" {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return nil, nil, errors.New("Rust scanner returned an invalid folder event")
			}
			folders = append(folders, *event.Folder)
			if len(folders) > maxRustScanEntries {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return nil, nil, errors.New("Rust scanner exceeded folder safety limit")
			}
		case "warning":
			if event.Message != "" {
				warnings = append(warnings, event.Message)
			}
		case "done":
			done = true
		default:
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, nil, fmt.Errorf("Rust scanner returned unknown event %q", event.Kind)
		}
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		return nil, nil, fmt.Errorf("Rust scanner failed: %s: %w", stderr.String(), waitErr)
	}
	if !done {
		return nil, nil, errors.New("Rust scanner ended without completion marker")
	}
	if err := validateRustFolders(folders); err != nil {
		return nil, nil, err
	}
	return folders, warnings, nil
}

func validateRustFolders(folders []rustScanFolder) error {
	for _, folder := range folders {
		for name, file := range folder.AudioFiles {
			if file.Size > math.MaxInt64 {
				return fmt.Errorf("audio file %q in %q exceeds supported size", name, folder.Path)
			}
		}
		for name, file := range folder.ImageFiles {
			if file.Size > math.MaxInt64 {
				return fmt.Errorf("image file %q in %q exceeds supported size", name, folder.Path)
			}
		}
	}
	return nil
}

func folderEntryFromRust(job *scanJob, source rustScanFolder) (*folderEntry, error) {
	entry := job.createFolderEntry(source.Path)
	entry.path = source.Path
	entry.modTime = unixNanoTime(source.ModTimeNS)
	entry.imagesUpdatedAt = unixNanoTime(source.ImagesUpdatedAtNS)
	entry.numPlaylists = source.NumPlaylists
	entry.numSubFolders = source.NumSubfolders
	for name, file := range source.AudioFiles {
		dirEntry, err := rustDirEntryFromFile(name, file)
		if err != nil {
			return nil, err
		}
		entry.audioFiles[name] = dirEntry
		entry.fileInfos[name] = dirEntry.info
	}
	for name, file := range source.ImageFiles {
		dirEntry, err := rustDirEntryFromFile(name, file)
		if err != nil {
			return nil, err
		}
		entry.imageFiles[name] = dirEntry
		entry.fileInfos[name] = dirEntry.info
	}
	entry.elapsed.Start()
	return entry, nil
}

func rustDirEntryFromFile(name string, file rustScanFile) (*rustDirEntry, error) {
	if file.Size > math.MaxInt64 {
		return nil, fmt.Errorf("file %q exceeds supported size", name)
	}
	return &rustDirEntry{
		name: name,
		info: rustFileInfo{name: name, size: int64(file.Size), modTime: unixNanoTime(file.ModTimeNS)},
	}, nil
}

func unixNanoTime(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value)
}

type rustDirEntry struct {
	name string
	info rustFileInfo
}

func (e *rustDirEntry) Name() string               { return e.name }
func (e *rustDirEntry) IsDir() bool                { return false }
func (e *rustDirEntry) Type() fs.FileMode          { return 0 }
func (e *rustDirEntry) Info() (fs.FileInfo, error) { return e.info, nil }

type rustFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (i rustFileInfo) Name() string       { return i.name }
func (i rustFileInfo) Size() int64        { return i.size }
func (i rustFileInfo) Mode() fs.FileMode  { return 0 }
func (i rustFileInfo) ModTime() time.Time { return i.modTime }
func (i rustFileInfo) IsDir() bool        { return false }
func (i rustFileInfo) Sys() any           { return nil }
