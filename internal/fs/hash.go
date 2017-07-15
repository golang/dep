package fs

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/pkg/errors"
)

const (
	pathSeparator = string(filepath.Separator)
	skipModes     = os.ModeDevice | os.ModeNamedPipe | os.ModeSocket | os.ModeCharDevice
)

// HashFromNode returns a deterministic hash of the specified file system node,
// performing a breadth-first traversal of directories. While the specified
// prefix is joined with the pathname to walk the file system, the prefix string
// is eliminated from the pathname of the nodes encounted when hashing the
// pathnames.
//
// This function ignores any file system node named `vendor`, `.bzr`, `.git`,
// `.hg`, and `.svn`, as these are typically used as Version Control System
// (VCS) directories.
//
// Other than the `vendor` and VCS directories mentioned above, the calculated
// hash includes the pathname to every discovered file system node, whether it
// is an empty directory, a non-empty directory, empty file, non-empty file, or
// symbolic link. If a symbolic link, the referent name is included. If a
// non-empty file, the file's contents are incuded. If a non-empty directory,
// the contents of the directory are included.
//
// While filepath.Walk could have been used, that standard library function
// skips symbolic links, and for now, we want to hash the referent string of
// symbolic links.
func HashFromNode(prefix, pathname string) (hash string, err error) {
	// Create a single hash instance for the entire operation, rather than a new
	// hash for each node we encounter.
	h := sha256.New()

	// "../../../vendor", "github.com/account/library"
	prefix = filepath.Clean(prefix) + pathSeparator
	prefixLength := len(prefix)
	if prefixLength > 0 {
		prefixLength += len(pathSeparator) // if not empty string, include len of path separator
	}
	joined := filepath.Join(prefix, pathname)

	// Initialize a work queue with the os-agnostic cleaned up pathname. Note
	// that we use `filepath.Clean` rather than `filepath.Abs`, because we don't
	// want the hash to be based on the absolute pathnames of the specified
	// directory and contents.
	pathnameQueue := []string{joined}

	for len(pathnameQueue) > 0 {
		// NOTE: unshift a pathname from the queue
		pathname, pathnameQueue = pathnameQueue[0], pathnameQueue[1:]

		fi, er := os.Lstat(pathname)
		if er != nil {
			err = errors.Wrap(er, "cannot Lstat")
			return
		}

		mode := fi.Mode()

		// Skip special files
		if mode&skipModes != 0 {
			continue
		}

		// NOTE: Write pathname to hash, because hash ought to be as much a
		// function of the names of the files and directories as their
		// contents. Added benefit is that even empty directories and symbolic
		// links will effect final hash value.
		//
		// NOTE: Throughout this function, we ignore return values from writing
		// to the hash, because hash write always returns nil error.
		_, _ = h.Write([]byte(pathname)[prefixLength:])

		if mode&os.ModeSymlink != 0 {
			referent, er := os.Readlink(pathname)
			if er != nil {
				err = errors.Wrap(er, "cannot Readlink")
				return
			}
			// Write the referent to the hash and proceed to the next pathname
			// in the queue.
			_, _ = h.Write([]byte(referent))
			continue
		}

		fh, er := os.Open(pathname)
		if er != nil {
			err = errors.Wrap(er, "cannot Open")
			return
		}

		if fi.IsDir() {
			childrenNames, er := fh.Readdirnames(0) // 0: read names of all children
			if er != nil {
				err = errors.Wrap(er, "cannot Readdirnames")
				// NOTE: Even if there was an error reading the names of the
				// directory entries, we still must close file handle for the
				// open directory before we return. In this case, we simply skip
				// sorting and adding entry names to the work queue beforehand.
				childrenNames = nil
			}

			// NOTE: Sort children names to ensure deterministic ordering of
			// contents of each directory, ensuring hash remains same even if
			// operating system returns same values in a different order on
			// subsequent invocation.
			sort.Strings(childrenNames)

			for _, childName := range childrenNames {
				switch childName {
				case ".", "..", "vendor", ".bzr", ".git", ".hg", ".svn":
					// skip
				default:
					pathnameQueue = append(pathnameQueue, pathname+pathSeparator+childName)
				}
			}
		} else {
			_, _ = h.Write([]byte(strconv.FormatInt(fi.Size(), 10))) // format file size as base 10 integer
			_, er = io.Copy(h, fh)
			err = errors.Wrap(er, "cannot Copy") // errors.Wrap only wraps non-nil, so elide checking here
		}

		// NOTE: Close the file handle to the open directory or file.
		if er = fh.Close(); err == nil {
			err = errors.Wrap(er, "cannot Close")
		}
		if err != nil {
			return // early termination iff error
		}
	}

	hash = fmt.Sprintf("%x", h.Sum(nil))
	return
}
