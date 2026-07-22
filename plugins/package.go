package plugins

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
)

const (
	// PackageExtension is the file extension for Navidrome plugin packages.
	PackageExtension = ".ndp"

	// manifestFileName is the name of the manifest file inside the package.
	manifestFileName = "manifest.json"

	// wasmFileName is the name of the WebAssembly module inside the package.
	wasmFileName = "plugin.wasm"

	maxManifestUncompressedSize = 1 << 20  // 1 MiB
	maxWASMUncompressedSize     = 64 << 20 // 64 MiB
)

// ndpPackage represents a loaded .ndp plugin package.
// It contains the manifest and wasm bytes read from the archive.
type ndpPackage struct {
	Manifest  *Manifest
	WasmBytes []byte
}

// openPackage opens an .ndp file and extracts the manifest and wasm bytes.
// The caller does not need to call Close() - all resources are read into memory.
func openPackage(ndpPath string) (*ndpPackage, error) {
	// Open the zip archive
	zr, err := zip.OpenReader(ndpPath)
	if err != nil {
		return nil, fmt.Errorf("opening package: %w", err)
	}
	defer zr.Close()

	manifestFile, wasmFile, err := findPackageFiles(zr.File)
	if err != nil {
		return nil, err
	}
	if manifestFile == nil {
		return nil, errors.New("package missing manifest.json")
	}
	if wasmFile == nil {
		return nil, errors.New("package missing plugin.wasm")
	}
	manifestBytes, err := readZipFile(manifestFile, maxManifestUncompressedSize)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	wasmBytes, err := readZipFile(wasmFile, maxWASMUncompressedSize)
	if err != nil {
		return nil, fmt.Errorf("reading wasm: %w", err)
	}

	// Parse and validate manifest
	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	return &ndpPackage{
		Manifest:  manifest,
		WasmBytes: wasmBytes,
	}, nil
}

// ReadManifest reads and validates the manifest from a .ndp file without loading
// the wasm bytes (it runs ParseManifest, so JSON-schema and cross-field
// validation are applied). Useful for quick plugin discovery and validation.
func ReadManifest(ndpPath string) (*Manifest, error) {
	// Open the zip archive
	zr, err := zip.OpenReader(ndpPath)
	if err != nil {
		return nil, fmt.Errorf("opening package: %w", err)
	}
	defer zr.Close()

	manifestFile, _, err := findPackageFiles(zr.File)
	if err != nil {
		return nil, err
	}
	if manifestFile == nil {
		return nil, errors.New("package missing manifest.json")
	}
	manifestBytes, err := readZipFile(manifestFile, maxManifestUncompressedSize)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	manifest, err := ParseManifest(manifestBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return manifest, nil
}

func findPackageFiles(files []*zip.File) (manifest, wasm *zip.File, err error) {
	for _, f := range files {
		switch f.Name {
		case manifestFileName:
			if manifest != nil {
				return nil, nil, fmt.Errorf("package contains duplicate %s", manifestFileName)
			}
			manifest = f
		case wasmFileName:
			if wasm != nil {
				return nil, nil, fmt.Errorf("package contains duplicate %s", wasmFileName)
			}
			wasm = f
		}
	}
	return manifest, wasm, nil
}

// readZipFile reads a bounded file from a zip archive. The metadata check
// rejects obvious bombs early; the streaming limit also protects against
// malformed archives whose declared size is inaccurate.
func readZipFile(f *zip.File, maxSize int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(maxSize) {
		return nil, fmt.Errorf("%s uncompressed size %d exceeds limit %d", f.Name, f.UncompressedSize64, maxSize)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return readLimited(rc, maxSize)
}

func readLimited(r io.Reader, maxSize int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("uncompressed content exceeds limit %d", maxSize)
	}
	return data, nil
}
