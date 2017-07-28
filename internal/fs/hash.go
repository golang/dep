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

var pathSeparator = string(os.PathSeparator)

// HashFromNode returns a deterministic hash of the file system node specified
// by pathname, performing a breadth-first traversal of directories, while
// ignoring any directory child named "vendor".
//
// While filepath.Walk could have been used, that standard library function
// skips symbolic links, and for now, it's a design requirement for this
// function to follow symbolic links.
//
// WARNING: This function loops indefinitely when it encounters a loop caused by
// poorly created symbolic links.
func HashFromNode(pathname string) (hash string, err error) {
	// bool argument: whether or not prevent file system loops due to symbolic
	// links
	return hashFromNode(pathname, false)
}

func hashFromNode(pathname string, preventLoops bool) (hash string, err error) {
	h := sha256.New()
	var fileInfos []os.FileInfo

	// Initialize a work queue with the os-agnostic cleaned up pathname.
	pathnameQueue := []string{filepath.Clean(pathname)}

	for len(pathnameQueue) > 0 {
		// NOTE: pop a pathname from the queue
		pathname, pathnameQueue = pathnameQueue[0], pathnameQueue[1:]

		fi, er := os.Stat(pathname)
		if er != nil {
			err = errors.Wrap(er, "cannot stat")
			return
		}

		fh, er := os.Open(pathname)
		if er != nil {
			err = errors.Wrap(er, "cannot open")
			return
		}

		// NOTE: Optionally disable checks to prevent infinite recursion when a
		// symbolic link causes an infinite loop, because this method does not
		// scale.
		if preventLoops {
			// Have we visited this node already?
			for _, seen := range fileInfos {
				if os.SameFile(fi, seen) {
					goto skipNode
				}
			}
			fileInfos = append(fileInfos, fi)
		}

		// NOTE: Write pathname to hash, because hash ought to be as much a
		// function of the names of the files and directories as their
		// contents. Added benefit is that empty directories effect final hash
		// value.
		//
		// Ignore return values from writing to the hash, because hash write
		// always returns nil error.
		_, _ = h.Write([]byte(pathname))

		if fi.IsDir() {
			childrenNames, er := fh.Readdirnames(0) // 0: read names of all children
			if er != nil {
				err = errors.Wrap(er, "cannot read directory")
				return
			}
			// NOTE: Sort children names to ensure deterministic ordering of
			// contents of each directory, so hash remains same even if
			// operating system returns same values in a different order on
			// subsequent invocation.
			sort.Strings(childrenNames)

			for _, childName := range childrenNames {
				switch childName {
				case ".", "..", "vendor":
					// skip
				default:
					pathnameQueue = append(pathnameQueue, pathname+pathSeparator+childName)
				}
			}
		} else {
			// NOTE: Format the file size as a base 10 integer, and ignore
			// return values from writing to the hash, because hash write always
			// returns a nil error.
			_, _ = h.Write([]byte(strconv.FormatInt(fi.Size(), 10)))
			_, er = io.Copy(h, fh)
			err = errors.Wrap(er, "cannot read file") // errors.Wrap only wraps non-nil, so elide checking here
		}

	skipNode:
		// NOTE: Close the file handle to the open directory or file.
		if er = fh.Close(); err == nil {
			err = errors.Wrap(er, "cannot close")
		}
		if err != nil {
			return // early termination if error
		}
	}

	hash = fmt.Sprintf("%x", h.Sum(nil))
	return
}
