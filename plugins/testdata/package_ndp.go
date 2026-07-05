//go:build ignore

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 4 {
		fatalf("usage: go run package_ndp.go <output.ndp> <manifest.json> <plugin.wasm>")
	}
	if err := packageNDP(os.Args[1], os.Args[2], os.Args[3]); err != nil {
		fatalf("%v", err)
	}
}

func packageNDP(output, manifest, wasm string) error {
	out, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("creating %s: %w", output, err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	for _, src := range []string{manifest, wasm} {
		if err := addFile(zw, src); err != nil {
			_ = zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", output, err)
	}
	return nil
}

func addFile(zw *zip.Writer, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer in.Close()

	w, err := zw.Create(filepath.Base(src))
	if err != nil {
		return fmt.Errorf("creating archive entry for %s: %w", src, err)
	}
	if _, err := io.Copy(w, in); err != nil {
		return fmt.Errorf("writing archive entry for %s: %w", src, err)
	}
	return nil
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
