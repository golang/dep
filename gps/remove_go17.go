// +build go1.7

package gps

import "os"

// go1.7 and later deal with the file perms issue in os.RemoveAll(), so our
// workaround is no longer necessary.
func removeAll(path string) error {
	return os.RemoveAll(path)
}
