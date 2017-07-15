package fs

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/pkg/errors"
)

// HashFromPathname returns a hash of the specified file or directory, ignoring
// all file system objects named, `vendor` and their descendants. This function
// follows symbolic links.
func HashFromPathname(pathname string) (hash string, err error) {
	fi, err := os.Stat(pathname)
	if err != nil {
		return "", errors.Wrap(err, "could not stat")
	}
	if fi.IsDir() {
		return hashFromDirectory(pathname, fi)
	}
	return hashFromFile(pathname, fi)
}

func hashFromFile(pathname string, fi os.FileInfo) (hash string, err error) {
	fh, err := os.Open(pathname)
	if err != nil {
		return "", errors.Wrap(err, "could not open")
	}
	defer func() {
		err = errors.Wrap(fh.Close(), "could not close")
	}()

	h := sha256.New()
	_, _ = h.Write([]byte(strconv.FormatInt(fi.Size(), 10)))

	if _, err = io.Copy(h, fh); err != nil {
		err = errors.Wrap(err, "could not read file")
		return
	}

	hash = fmt.Sprintf("%x", h.Sum(nil))
	return
}

func hashFromDirectory(pathname string, fi os.FileInfo) (hash string, err error) {
	const maxFileInfos = 32

	fh, err := os.Open(pathname)
	if err != nil {
		return hash, errors.Wrap(err, "could not open")
	}
	defer func() {
		err = errors.Wrap(fh.Close(), "could not close")
	}()

	h := sha256.New()

	// NOTE: Chunk through file system objects to prevent allocating too much
	// memory for directories with tens of thousands of child objects.
	for {
		var children []os.FileInfo
		var childHash string

		children, err = fh.Readdir(maxFileInfos)
		if err != nil {
			if err == io.EOF {
				err = nil
				break
			}
			return hash, errors.Wrap(err, "could not read directory")
		}
		for _, child := range children {
			switch child.Name() {
			case ".", "..", "vendor":
				// skip
			default:
				childPathname := pathname + string(os.PathSeparator) + child.Name()
				if childHash, err = HashFromPathname(childPathname); err != nil {
					err = errors.Wrap(err, "could not compute hash from pathname")
					return
				}
				_, _ = h.Write([]byte(childPathname))
				_, _ = h.Write([]byte(childHash))
			}
		}
	}

	hash = fmt.Sprintf("%x", h.Sum(nil))
	return
}
