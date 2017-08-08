package pkgtree

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/pkg/errors"
)

const (
	pathSeparator = string(filepath.Separator)

	// when walking vendor root hierarchy, ignore file system nodes of the
	// following types.
	skipModes = os.ModeDevice | os.ModeNamedPipe | os.ModeSocket | os.ModeCharDevice
)

// DigestFromPathname returns a deterministic hash of the specified file system
// node, performing a breadth-first traversal of directories. While the
// specified prefix is joined with the pathname to walk the file system, the
// prefix string is eliminated from the pathname of the nodes encounted when
// hashing the pathnames, so that the resultant hash is agnostic to the absolute
// root directory path of the nodes being checked.
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
// skips symbolic links, and for now, we want the hash to include the symbolic
// link referents.
func DigestFromPathname(prefix, pathname string) ([]byte, error) {
	// Create a single hash instance for the entire operation, rather than a new
	// hash for each node we encounter.
	h := sha256.New()

	// Initialize a work queue with the os-agnostic cleaned up pathname. Note
	// that we use `filepath.Clean` rather than `filepath.Abs`, because the hash
	// has pathnames which are relative to prefix, and there is no reason to
	// convert to absolute pathname for every invocation of this function.
	prefix = filepath.Clean(prefix) + pathSeparator
	prefixLength := len(prefix) // store length to trim off pathnames later
	pathnameQueue := []string{filepath.Join(prefix, pathname)}

	// As we enumerate over the queue and encounter a directory, its children
	// will be added to the work queue.
	for len(pathnameQueue) > 0 {
		// Unshift a pathname from the queue (breadth-first traversal of
		// hierarchy)
		pathname, pathnameQueue = pathnameQueue[0], pathnameQueue[1:]

		fi, err := os.Lstat(pathname)
		if err != nil {
			return nil, errors.Wrap(err, "cannot Lstat")
		}
		mode := fi.Mode()

		// Skip file system nodes we are not concerned with
		if mode&skipModes != 0 {
			continue
		}

		// Write the prefix-stripped pathname to hash because the hash is as
		// much a function of the relative names of the files and directories as
		// it is their contents. Added benefit is that even empty directories
		// and symbolic links will effect final hash value. Use
		// `filepath.ToSlash` to ensure relative pathname is os-agnostic.
		writeBytesWithNull(h, []byte(filepath.ToSlash(pathname[prefixLength:])))

		if mode&os.ModeSymlink != 0 {
			referent, err := os.Readlink(pathname)
			if err != nil {
				return nil, errors.Wrap(err, "cannot Readlink")
			}
			// Write the os-agnostic referent to the hash and proceed to the
			// next pathname in the queue.
			writeBytesWithNull(h, []byte(filepath.ToSlash(referent)))
			continue
		}

		// For both directories and regular files, we must create a file system
		// handle in order to read their contents.
		fh, err := os.Open(pathname)
		if err != nil {
			return nil, errors.Wrap(err, "cannot Open")
		}

		if fi.IsDir() {
			childrenNames, err := sortedListOfDirectoryChildrenFromFileHandle(fh)
			if err != nil {
				_ = fh.Close() // already have an error reading directory; ignore Close result.
				return nil, errors.Wrap(err, "cannot get list of directory children")
			}
			for _, childName := range childrenNames {
				switch childName {
				case ".", "..", "vendor", ".bzr", ".git", ".hg", ".svn":
					// skip
				default:
					pathnameQueue = append(pathnameQueue, filepath.Join(pathname, childName))
				}
			}
		} else {
			writeBytesWithNull(h, []byte(strconv.FormatInt(fi.Size(), 10))) // format file size as base 10 integer
			_, err = io.Copy(h, &lineEndingWriterTo{fh})                    // fast copy of file contents to hash
			err = errors.Wrap(err, "cannot Copy")                           // errors.Wrap only wraps non-nil, so elide guard condition
		}

		// Close the file handle to the open directory without masking possible
		// previous error value.
		if er := fh.Close(); err == nil {
			err = errors.Wrap(er, "cannot Close")
		}
		if err != nil {
			return nil, err // early termination iff error
		}
	}

	return h.Sum(nil), nil
}

// lineEndingWriterTo
type lineEndingWriterTo struct {
	src io.Reader
}

func (liw *lineEndingWriterTo) Read(buf []byte) (int, error) {
	return liw.src.Read(buf)
}

// WriteTo writes data to w until there's no more data to write or
// when an error occurs. The return value n is the number of bytes
// written. Any error encountered during the write is also returned.
//
// The Copy function uses WriterTo if available.
func (liw *lineEndingWriterTo) WriteTo(dst io.Writer) (int64, error) {
	// Some VCS systems automatically convert LF line endings to CRLF on some OS
	// platforms, such as Windows. This would cause the a file checked out on
	// Windows to have a different digest than the same file on macOS or
	// Linux. In order to ensure file contents normalize the same, we need to
	// modify the file's contents when we compute its hash.
	//
	// Keep reading from embedded io.Reader and writing to io.Writer until EOF.
	// For each blob, when read CRLF, convert to LF.  Ensure handle case when CR
	// read in one buffer and LF read in another.  Another option is just filter
	// out all CR bytes, but unneeded removal of CR bytes increases our surface
	// area for accidental hash collisions. Therefore, we will only convert CRLF
	// to LF.

	// Create a buffer to hold file contents; use same size as `io.Copy`
	// creates.
	buf := make([]byte, 32*1024)

	var err error
	var finalCR bool
	var written int64

	for {
		nr, er := liw.src.Read(buf)
		if nr > 0 {
			// When previous buffer ended in CR and this buffer does not start
			// in LF, we need to emit a CR byte.
			if finalCR && buf[0] != '\n' {
				// We owe the destination a CR byte because we held onto it from
				// last loop.
				nw, ew := dst.Write([]byte{'\r'})
				if nw > 0 {
					written += int64(nw)
				}
				if ew != nil {
					err = ew
					break
				}
			}

			// Remove any CRLF sequences in buf, using `bytes.Index` because
			// that takes advantage of machine opcodes for searching for byte
			// patterns on many platforms.
			for {
				index := bytes.Index(buf, []byte("\r\n"))
				if index == -1 {
					break
				}
				// Want to skip index byte, where the CR is.
				copy(buf[index:nr-1], buf[index+1:nr])
				nr--
			}

			// When final byte is CR, do not emit until we ensure first byte on
			// next read is LF.
			if finalCR = buf[nr-1] == '\r'; finalCR {
				nr-- // pretend byte was never read from source
			}

			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	if finalCR {
		nw, ew := dst.Write([]byte{'\r'})
		if nw > 0 {
			written += int64(nw)
		}
		if ew != nil {
			err = ew
		}
	}
	return written, err
}

// writeBytesWithNull appends the specified data to the specified hash, followed by
// the NULL byte, in order to make accidental hash collisions less likely.
func writeBytesWithNull(h hash.Hash, data []byte) {
	// Ignore return values from writing to the hash, because hash write always
	// returns nil error.
	_, _ = h.Write(append(data, 0))
}

// VendorStatus represents one of a handful of possible statuses of a particular
// subdirectory under vendor.
type VendorStatus uint8

const (
	// NotInLock is used when a file system node exists for which there is no
	// corresponding dependency in the lock file.
	NotInLock VendorStatus = iota

	// NotInTree is used when a lock file dependency exists for which there is
	// no corresponding file system node.
	NotInTree

	// NoMismatch is used when the digest for a dependency listed in the
	// lockfile matches what is calculated from the file system.
	NoMismatch

	// EmptyDigestInLock is used when the digest for a dependency listed in the
	// lock file is the empty string. NOTE: Seems like a special case of
	// DigestMismatchInLock.
	EmptyDigestInLock

	// DigestMismatchInLock is used when the digest for a dependency listed in
	// the lock file does not match what is calculated from the file system.
	DigestMismatchInLock
)

func (ls VendorStatus) String() string {
	switch ls {
	case NotInTree:
		return "not in tree"
	case NotInLock:
		return "not in lock"
	case NoMismatch:
		return "match"
	case EmptyDigestInLock:
		return "empty digest in lock"
	case DigestMismatchInLock:
		return "mismatch"
	}
	return "unknown"
}

type fsnode struct {
	pathname             string
	isRequiredAncestor   bool
	myIndex, parentIndex int
}

func (n fsnode) String() string {
	return fmt.Sprintf("[%d:%d %q %t]", n.myIndex, n.parentIndex, n.pathname, n.isRequiredAncestor)
}

// sortedListOfDirectoryChildrenFromPathname returns a lexicographical sorted
// list of child nodes for the specified directory.
func sortedListOfDirectoryChildrenFromPathname(pathname string) ([]string, error) {
	fh, err := os.Open(pathname)
	if err != nil {
		return nil, errors.Wrap(err, "cannot Open")
	}

	childrenNames, err := sortedListOfDirectoryChildrenFromFileHandle(fh)

	// Close the file handle to the open directory without masking possible
	// previous error value.
	if er := fh.Close(); err == nil {
		err = errors.Wrap(er, "cannot Close")
	}

	return childrenNames, err
}

// sortedListOfDirectoryChildrenFromPathname returns a lexicographical sorted
// list of child nodes for the specified open file handle to a directory. This
// function is written once to avoid writing the logic in two places.
func sortedListOfDirectoryChildrenFromFileHandle(fh *os.File) ([]string, error) {
	childrenNames, err := fh.Readdirnames(0) // 0: read names of all children
	if err != nil {
		return nil, errors.Wrap(err, "cannot Readdirnames")
	}
	if len(childrenNames) > 0 {
		sort.Strings(childrenNames)
	}
	return childrenNames, nil
}

// VerifyDepTree verifies dependency tree according to expected digest sums, and
// returns an associative array of file system nodes and their respective vendor
// status, in accordance with the provided expected digest sums parameter.
func VerifyDepTree(vendorPathname string, wantSums map[string][]byte) (map[string]VendorStatus, error) {
	// NOTE: Ensure top level pathname is a directory
	fi, err := os.Stat(vendorPathname)
	if err != nil {
		return nil, errors.Wrap(err, "cannot Stat")
	}
	if !fi.IsDir() {
		return nil, errors.Errorf("cannot verify non directory: %q", vendorPathname)
	}

	vendorPathname = filepath.Clean(vendorPathname) + pathSeparator
	prefixLength := len(vendorPathname)

	var otherNode *fsnode
	currentNode := &fsnode{pathname: vendorPathname, parentIndex: -1, isRequiredAncestor: true}
	queue := []*fsnode{currentNode} // queue of directories that must be inspected

	// In order to identify all file system nodes that are not in the lock file,
	// represented by the specified expected sums parameter, and in order to
	// only report the top level of a subdirectory of file system nodes, rather
	// than every node internal to them, we will create a tree of nodes stored
	// in a slice.  We do this because we do not know at what level a project
	// exists at. Some projects are fewer than and some projects more than the
	// typical three layer subdirectory under the vendor root directory.
	//
	// For a following few examples, assume the below vendor root directory:
	//
	// github.com/alice/alice1/a1.go
	// github.com/alice/alice2/a2.go
	// github.com/bob/bob1/b1.go
	// github.com/bob/bob2/b2.go
	// launghpad.net/nifty/n1.go
	//
	// 1) If only the `alice1` and `alice2` projects were in the lock file, we'd
	// prefer the output to state that `github.com/bob` is `NotInLock`.
	//
	// 2) If `alice1`, `alice2`, and `bob1` were in the lock file, we'd want to
	// report `github.com/bob/bob2` as `NotInLock`.
	//
	// 3) If none of `alice1`, `alice2`, `bob1`, or `bob2` were in the lock
	// file, the entire `github.com` directory would be reported as `NotInLock`.
	//
	// Each node in our tree has the slice index of its parent node, so once we
	// can categorically state a particular directory is required because it is
	// in the lock file, we can mark all of its ancestors as also being
	// required. Then, when we finish walking the directory hierarchy, any nodes
	// which are not required but have a required parent will be marked as
	// `NotInLock`.
	nodes := []*fsnode{currentNode}

	// Mark directories of expected projects as required. When the respective
	// project is found in the vendor root hierarchy, its status will be updated
	// to reflect whether its digest is empty, or whether or not it matches the
	// expected digest.
	status := make(map[string]VendorStatus)
	for pathname := range wantSums {
		status[pathname] = NotInTree
	}

	for len(queue) > 0 {
		// pop node from the queue (depth first traversal, reverse lexicographical order)
		lq1 := len(queue) - 1
		currentNode, queue = queue[lq1], queue[:lq1]

		// log.Printf("NODE: %s", currentNode)
		short := currentNode.pathname[prefixLength:] // chop off the vendor root prefix, including the path separator
		if expectedSum, ok := wantSums[short]; ok {
			ls := EmptyDigestInLock
			if len(expectedSum) > 0 {
				ls = NoMismatch
				projectSum, err := DigestFromPathname(vendorPathname, short)
				if err != nil {
					return nil, errors.Wrap(err, "cannot compute dependency hash")
				}
				if !bytes.Equal(projectSum, expectedSum) {
					ls = DigestMismatchInLock
				}
			}
			status[short] = ls

			// NOTE: Mark current nodes and all parents: required.
			for pni := currentNode.myIndex; pni != -1; pni = otherNode.parentIndex {
				otherNode = nodes[pni]
				otherNode.isRequiredAncestor = true
				// log.Printf("parent node: %s", otherNode)
			}

			continue // do not need to process directory's contents
		}

		childrenNames, err := sortedListOfDirectoryChildrenFromPathname(currentNode.pathname)
		if err != nil {
			return nil, errors.Wrap(err, "cannot get sorted list of directory children")
		}
		for _, childName := range childrenNames {
			switch childName {
			case ".", "..", "vendor", ".bzr", ".git", ".hg", ".svn":
				// skip
			default:
				childPathname := filepath.Join(currentNode.pathname, childName)
				otherNode = &fsnode{pathname: childPathname, myIndex: len(nodes), parentIndex: currentNode.myIndex}

				fi, err := os.Stat(childPathname)
				if err != nil {
					return nil, errors.Wrap(err, "cannot Stat")
				}
				// Skip non-interesting file system nodes
				mode := fi.Mode()
				if mode&skipModes != 0 || mode&os.ModeSymlink != 0 {
					// log.Printf("DEBUG: skipping: %v; %q", mode, currentNode.pathname)
					continue
				}

				nodes = append(nodes, otherNode)
				if fi.IsDir() {
					queue = append(queue, otherNode)
				}
			}
		}

		if err != nil {
			return nil, err // early termination iff error
		}
	}

	// Ignoring first node in the list, walk nodes from end to
	// beginning. Whenever a node is not required, but its parent is required,
	// then that node and all under it ought to be marked as `NotInLock`.
	for i := len(nodes) - 1; i > 0; i-- {
		currentNode = nodes[i]
		if !currentNode.isRequiredAncestor && nodes[currentNode.parentIndex].isRequiredAncestor {
			status[currentNode.pathname[prefixLength:]] = NotInLock
		}
	}

	return status, nil
}
