//go:build !windows

package hotcache

import "os"

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
