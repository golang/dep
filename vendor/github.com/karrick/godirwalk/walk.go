package godirwalk

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/pkg/errors"
)

// Options provide parameters for how the Walk function operates.
type Options struct {
	// FollowSymbolicLinks specifies whether Walk will follow symbolic links
	// that refer to directories. When set to false or left as its zero-value,
	// Walk will still invoke the callback function with symbolic link nodes,
	// but if the symbolic link refers to a directory, it will not recurse on
	// that directory. When set to true, Walk will recurse on symbolic links
	// that refer to a directory.
	FollowSymbolicLinks bool

	// Unsorted controls whether or not Walk will sort the immediate descendants
	// of a directory by their relative names prior to visiting each of those
	// entries.
	//
	// When set to false or left at its zero-value, Walk will get the list of
	// immediate descendants of a particular directory, sort that list by
	// lexical order of their names, and then visit each node in the list in
	// sorted order. This will cause Walk to always traverse the same directory
	// tree in the same order, however may be inefficient for directories with
	// many immediate descendants.
	//
	// When set to true, Walk skips sorting the list of immediate descendants
	// for a directory, and simply visits each node in the order the operating
	// system enumerated them. This will be more fast, but with the side effect
	// that the traversal order may be different from one invocation to the
	// next.
	Unsorted bool

	// Callback is the function that Walk will invoke for every file system node
	// it encounters.
	Callback WalkFunc

	// ScratchBuffer is an optional scratch buffer for Walk to use when reading
	// directory entries, to reduce amount of garbage generation. Not all
	// architectures take advantage of the scratch buffer.
	ScratchBuffer []byte
}

// WalkFunc is the type of the function called for each file system node visited
// by Walk. The pathname argument will contain the argument to Walk as a prefix;
// that is, if Walk is called with "dir", which is a directory containing the
// file "a", the provided WalkFunc will be invoked with the argument "dir/a",
// using the correct os.PathSeparator for the Go Operating System architecture,
// GOOS. The directory entry argument is a pointer to a Dirent for the node,
// providing access to both the basename and the mode type of the file system
// node.
//
// If an error is returned by the walk function, processing stops. The sole
// exception is when the function returns the special value filepath.SkipDir. If
// the function returns filepath.SkipDir when invoked on a directory, Walk skips
// the directory's contents entirely. If the function returns filepath.SkipDir
// when invoked on a non-directory file system node, Walk skips the remaining
// files in the containing directory.
type WalkFunc func(osPathname string, directoryEntry *Dirent) error

// Walk walks the file tree rooted at the specified directory, calling the
// specified callback function for each file system node in the tree, including
// root, symbolic links, and other node types. The nodes are walked in lexical
// order, which makes the output deterministic but means that for very large
// directories this function can be inefficient.
//
// This function is often much faster than filepath.Walk because it does not
// invoke os.Stat for every node it encounters, but rather obtains the file
// system node type when it reads the parent directory.
//
//    func main() {
//        dirname := "."
//        if len(os.Args) > 1 {
//            dirname = os.Args[1]
//        }
//        err := godirwalk.Walk(dirname, &godirwalk.Options{
//            Callback: func(osPathname string, de *godirwalk.Dirent) error {
//                fmt.Printf("%s %s\n", de.ModeType(), osPathname)
//                return nil
//            },
//        })
//        if err != nil {
//            fmt.Fprintf(os.Stderr, "%s\n", err)
//            os.Exit(1)
//        }
//    }
func Walk(pathname string, options *Options) error {
	pathname = filepath.Clean(pathname)

	var fi os.FileInfo
	var err error

	if options.FollowSymbolicLinks {
		fi, err = os.Stat(pathname)
		if err != nil {
			return errors.Wrap(err, "cannot Stat")
		}
	} else {
		fi, err = os.Lstat(pathname)
		if err != nil {
			return errors.Wrap(err, "cannot Lstat")
		}
	}

	mode := fi.Mode()
	if mode&os.ModeDir == 0 {
		return errors.Errorf("cannot Walk non-directory: %s", pathname)
	}

	dirent := &Dirent{
		name:     filepath.Base(pathname),
		modeType: mode & os.ModeType,
	}

	err = walk(pathname, dirent, options)
	if err == filepath.SkipDir {
		return nil // silence SkipDir for top level
	}
	return err
}

// walk recursively traverses the file system node specified by pathname and the
// Dirent.
func walk(osPathname string, dirent *Dirent, options *Options) error {
	err := options.Callback(osPathname, dirent)
	if err != nil {
		if err != filepath.SkipDir {
			return errors.Wrap(err, "WalkFunc") // wrap potential errors returned by walkFn
		}
		return err
	}

	// On some platforms, an entry can have more than one mode type bit set.
	// For instance, it could have both the symlink bit and the directory bit
	// set indicating it's a symlink to a directory.
	if dirent.IsSymlink() {
		if !options.FollowSymbolicLinks {
			return nil
		}
		// Only need to Stat entry if platform did not already have os.ModeDir
		// set, such as would be the case for unix like operating systems. (This
		// guard eliminates extra os.Stat check on Windows.)
		if !dirent.IsDir() {
			referent, err := os.Readlink(osPathname)
			if err != nil {
				return errors.Wrap(err, "cannot Readlink")
			}
			fi, err := os.Stat(filepath.Join(filepath.Dir(osPathname), referent))
			if err != nil {
				return errors.Wrap(err, "cannot Stat")
			}
			dirent.modeType = fi.Mode() & os.ModeType
		}
	}

	if !dirent.IsDir() {
		return nil
	}

	// If get here, then specified pathname refers to a directory.
	deChildren, err := ReadDirents(osPathname, options.ScratchBuffer)
	if err != nil {
		return errors.Wrap(err, "cannot ReadDirents")
	}

	if !options.Unsorted {
		sort.Sort(deChildren) // sort children entries unless upstream says to leave unsorted
	}

	for _, deChild := range deChildren {
		osChildname := filepath.Join(osPathname, deChild.name)
		err = walk(osChildname, deChild, options)
		if err != nil {
			if err != filepath.SkipDir {
				return err
			}
			// If received skipdir on a directory, stop processing that
			// directory, but continue to its siblings. If received skipdir on a
			// non-directory, stop processing remaining siblings.
			if deChild.IsSymlink() {
				// Only need to Stat entry if platform did not already have
				// os.ModeDir set, such as would be the case for unix like
				// operating systems. (This guard eliminates extra os.Stat check
				// on Windows.)
				if !deChild.IsDir() {
					// Resolve symbolic link referent to determine whether node
					// is directory or not.
					referent, err := os.Readlink(osChildname)
					if err != nil {
						return errors.Wrap(err, "cannot Readlink")
					}
					fi, err := os.Stat(filepath.Join(osPathname, referent))
					if err != nil {
						return errors.Wrap(err, "cannot Stat")
					}
					deChild.modeType = fi.Mode() & os.ModeType
				}
			}
			if !deChild.IsDir() {
				// If not directory, return immediately, thus skipping remainder
				// of siblings.
				return nil
			}
		}
	}
	return nil
}
