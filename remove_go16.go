// +build !go1.7

package gps

import (
	"os"
	"path/filepath"
	"runtime"
)

// removeAll removes path and any children it contains. It deals correctly with
// removal on Windows where, prior to Go 1.7, there were issues when files were
// set to read-only.
func removeAll(path string) error {
	// Only need special handling for windows
	if runtime.GOOS != "windows" {
		return os.RemoveAll(path)
	}

	// Simple case: if Remove works, we're done.
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}

	// make sure all files are writable so we can delete them
	err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil && err != filepath.SkipDir {
			// walk gave us some error, give it back.
			return err
		}
		mode := info.Mode()
		if mode|0200 == mode {
			return nil
		}

		return os.Chmod(path, mode|0200)
	})
	if err != nil {
		return err
	}

	return os.Remove(path)
}
