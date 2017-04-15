//+build !windows

package gps

import "os"

func stripVendor(path string, info os.FileInfo, err error) error {
	if info.Name() == "vendor" {
		if _, err := os.Lstat(path); err == nil {
			if (info.Mode() & os.ModeSymlink) != 0 {
				realInfo, err := os.Stat(path)
				if err != nil {
					return err
				}
				if realInfo.IsDir() {
					return os.Remove(path)
				}
			}
			if info.IsDir() {
				return removeAll(path)
			}
		}
	}

	return nil
}
