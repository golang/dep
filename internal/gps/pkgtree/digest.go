// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkgtree

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
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
	skipSpecialNodes = os.ModeDevice | os.ModeNamedPipe | os.ModeSocket | os.ModeCharDevice
)

// DigestFromDirectory returns a hash of the specified directory contents, which
// will match the hash computed for any directory on any supported Go platform
// whose contents exactly match the specified directory.
//
// This function ignores any file system node named `vendor`, `.bzr`, `.git`,
// `.hg`, and `.svn`, as these are typically used as Version Control System
// (VCS) directories.
//
// Other than the `vendor` and VCS directories mentioned above, the calculated
// hash includes the pathname to every discovered file system node, whether it
// is an empty directory, a non-empty directory, empty file, non-empty file, or
// symbolic link. If a symbolic link, the referent name is included. If a
// non-empty file, the file's contents are included. If a non-empty directory,
// the contents of the directory are included.
//
// While filepath.Walk could have been used, that standard library function
// skips symbolic links, and for now, we want the hash to include the symbolic
// link referents.
func DigestFromDirectory(dirname string) ([]byte, error) {
	dirname = filepath.Clean(dirname)

	// Ensure parameter is a directory
	fi, err := os.Stat(dirname)
	if err != nil {
		return nil, errors.Wrap(err, "cannot Stat")
	}
	if !fi.IsDir() {
		return nil, errors.Errorf("cannot verify non directory: %q", dirname)
	}

	// Create a single hash instance for the entire operation, rather than a new
	// hash for each node we encounter.
	h := sha256.New()

	// Initialize a work queue with the empty string, which signifies the
	// starting directory itself.
	queue := []string{""}

	var relative string // pathname relative to dirname
	var pathname string // formed by combining dirname with relative for each node
	var bytesWritten int64
	modeBytes := make([]byte, 4) // scratch place to store encoded os.FileMode (uint32)

	// As we enumerate over the queue and encounter a directory, its children
	// will be added to the work queue.
	for len(queue) > 0 {
		// Unshift a pathname from the queue (breadth-first traversal of
		// hierarchy)
		relative, queue = queue[0], queue[1:]
		pathname = filepath.Join(dirname, relative)

		fi, err = os.Lstat(pathname)
		if err != nil {
			return nil, errors.Wrap(err, "cannot Lstat")
		}

		// We could make our own enum-like data type for encoding the file type,
		// but Go's runtime already gives us architecture independent file
		// modes, as discussed in `os/types.go`:
		//
		//    Go's runtime FileMode type has same definition on all systems, so
		//    that information about files can be moved from one system to
		//    another portably.
		var mt os.FileMode

		// We only care about the bits that identify the type of a file system
		// node, and can ignore append, exclusive, temporary, setuid, setgid,
		// permission bits, and sticky bits, which are coincident to bits which
		// declare type of the file system node.
		modeType := fi.Mode() & os.ModeType
		var shouldSkip bool // skip some types of file system nodes

		switch {
		case modeType&os.ModeDir > 0:
			mt = os.ModeDir
		case modeType&os.ModeSymlink > 0:
			mt = os.ModeSymlink
		case modeType&os.ModeNamedPipe > 0:
			mt = os.ModeNamedPipe
			shouldSkip = true
		case modeType&os.ModeSocket > 0:
			mt = os.ModeSocket
			shouldSkip = true
		case modeType&os.ModeDevice > 0:
			mt = os.ModeDevice
			shouldSkip = true
		}

		// Write the relative pathname to hash because the hash is a function of
		// the node names, node types, and node contents. Added benefit is that
		// empty directories, named pipes, sockets, devices, and symbolic links
		// will affect final hash value. Use `filepath.ToSlash` to ensure
		// relative pathname is os-agnostic.
		writeBytesWithNull(h, []byte(filepath.ToSlash(relative)))

		binary.LittleEndian.PutUint32(modeBytes, uint32(mt)) // encode the type of mode
		writeBytesWithNull(h, modeBytes)                     // and write to hash

		if shouldSkip {
			// There is nothing more to do for some of the node types.
			continue
		}

		if mt == os.ModeSymlink { // okay to check for equivalence because we set to this value
			relative, err = os.Readlink(pathname) // read the symlink referent
			if err != nil {
				return nil, errors.Wrap(err, "cannot Readlink")
			}
			// Write the os-agnostic referent to the hash and proceed to the
			// next pathname in the queue.
			writeBytesWithNull(h, []byte(filepath.ToSlash(relative))) // and write it to hash
			continue
		}

		// For both directories and regular files, we must create a file system
		// handle in order to read their contents.
		fh, err := os.Open(pathname)
		if err != nil {
			return nil, errors.Wrap(err, "cannot Open")
		}

		if mt == os.ModeDir {
			childrenNames, err := sortedListOfDirectoryChildrenFromFileHandle(fh)
			if err != nil {
				_ = fh.Close() // ignore close error because we already have an error reading directory
				return nil, errors.Wrap(err, "cannot get list of directory children")
			}
			for _, childName := range childrenNames {
				switch childName {
				case ".", "..", "vendor", ".bzr", ".git", ".hg", ".svn":
					// skip
				default:
					queue = append(queue, filepath.Join(relative, childName))
				}
			}
		} else {
			bytesWritten, err = io.Copy(h, newLineEndingReader(fh))            // fast copy of file contents to hash
			err = errors.Wrap(err, "cannot Copy")                              // errors.Wrap only wraps non-nil, so skip extra check
			writeBytesWithNull(h, []byte(strconv.FormatInt(bytesWritten, 10))) // format file size as base 10 integer
		}

		// Close the file handle to the open directory or file without masking
		// possible previous error value.
		if er := fh.Close(); err == nil {
			err = errors.Wrap(er, "cannot Close")
		}
		if err != nil {
			return nil, err // early termination iff error
		}
	}

	return h.Sum(nil), nil
}

// lineEndingReader is a `io.Reader` that converts CRLF sequences to LF.
//
// Some VCS systems automatically convert LF line endings to CRLF on some OS
// platforms. This would cause the a file checked out on those platforms to have
// a different digest than the same file on platforms that do not perform this
// translation. In order to ensure file contents normalize and hash the same,
// this struct satisfies the io.Reader interface by providing a Read method that
// modifies the file's contents when it is read, translating all CRLF sequences
// to LF.
type lineEndingReader struct {
	src             io.Reader // source io.Reader from which this reads
	prevReadEndedCR bool      // used to track whether final byte of previous Read was CR
}

// newLineEndingReader returns a new lineEndingReader that reads from the
// specified source io.Reader.
func newLineEndingReader(src io.Reader) *lineEndingReader {
	return &lineEndingReader{src: src}
}

var crlf = []byte("\r\n")

// Read consumes bytes from the structure's source io.Reader to fill the
// specified slice of bytes. It converts all CRLF byte sequences to LF, and
// handles cases where CR and LF straddle across two Read operations.
func (f *lineEndingReader) Read(buf []byte) (int, error) {
	buflen := len(buf)
	if f.prevReadEndedCR {
		// Read one less byte in case we need to insert CR in there
		buflen--
	}
	nr, er := f.src.Read(buf[:buflen])
	if nr > 0 {
		if f.prevReadEndedCR && buf[0] != '\n' {
			// Having a CRLF split across two Read operations is rare, so
			// ignoring performance impact of copying entire buffer by one
			// byte. Plus, `copy` builtin likely uses machine opcode for
			// performing the memory copy.
			copy(buf[1:nr+1], buf[:nr]) // shift data to right one byte
			buf[0] = '\r'               // insert the previous skipped CR byte at start of buf
			nr++                        // pretend we read one more byte
		}

		// Remove any CRLF sequences in buf, using `bytes.Index` because it
		// takes advantage of machine opcodes that search for byte patterns on
		// many architectures; and, using builtin `copy` which also takes
		// advantage of machine opcodes to quickly move overlapping regions of
		// bytes in memory.
		var searchOffset, shiftCount int
		previousIndex := -1 // index of previous CRLF index; -1 means no previous index known
		for {
			index := bytes.Index(buf[searchOffset:nr], crlf)
			if index == -1 {
				break
			}
			index += searchOffset // convert relative index to absolute
			if previousIndex != -1 {
				// shift substring between previous index and this index
				copy(buf[previousIndex-shiftCount:], buf[previousIndex+1:index])
				shiftCount++ // next shift needs to be 1 byte to the left
			}
			previousIndex = index
			searchOffset = index + 2 // start next search after len(crlf)
		}
		if previousIndex != -1 {
			// handle final shift
			copy(buf[previousIndex-shiftCount:], buf[previousIndex+1:nr])
			shiftCount++
		}
		nr -= shiftCount // shorten byte read count by number of shifts executed

		// When final byte from a read operation is CR, do not emit it until
		// ensure first byte on next read is not LF.
		if f.prevReadEndedCR = buf[nr-1] == '\r'; f.prevReadEndedCR {
			nr-- // pretend byte was never read from source
		}
	} else if f.prevReadEndedCR {
		// Reading from source returned nothing, but this struct is sitting on a
		// trailing CR from previous Read, so let's give it to client now.
		buf[0] = '\r'
		nr = 1
		er = nil
		f.prevReadEndedCR = false // prevent infinite loop
	}
	return nr, er
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
	// lock file is the empty string. While this is a special case of
	// DigestMismatchInLock, keeping both cases discrete is a desired feature.
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
	sort.Strings(childrenNames)
	return childrenNames, nil
}

// VerifyDepTree verifies a dependency tree according to expected digest sums,
// and returns an associative array of file system nodes and their respective
// vendor status, in accordance with the provided expected digest sums
// parameter.
//
// The vendor root will be converted to os-specific pathname for processing, and
// the map of project names to their expected digests are required to have the
// solidus character, `/`, as their path separator. For example,
// "github.com/alice/alice1".
func VerifyDepTree(vendorRoot string, wantSums map[string][]byte) (map[string]VendorStatus, error) {
	vendorRoot = filepath.Clean(vendorRoot) + pathSeparator

	// NOTE: Ensure top level pathname is a directory
	fi, err := os.Stat(vendorRoot)
	if err != nil {
		return nil, errors.Wrap(err, "cannot Stat")
	}
	if !fi.IsDir() {
		return nil, errors.Errorf("cannot verify non directory: %q", vendorRoot)
	}

	var otherNode *fsnode
	currentNode := &fsnode{pathname: vendorRoot, parentIndex: -1, isRequiredAncestor: true}
	queue := []*fsnode{currentNode} // queue of directories that must be inspected
	prefixLength := len(vendorRoot)

	// In order to identify all file system nodes that are not in the lock file,
	// represented by the specified expected sums parameter, and in order to
	// only report the top level of a subdirectory of file system nodes, rather
	// than every node internal to them, we will create a tree of nodes stored
	// in a slice. We do this because we cannot predict the depth at which
	// project roots occur. Some projects are fewer than and some projects more
	// than the typical three layer subdirectory under the vendor root
	// directory.
	//
	// For a following few examples, assume the below vendor root directory:
	//
	// github.com/alice/alice1/a1.go
	// github.com/alice/alice2/a2.go
	// github.com/bob/bob1/b1.go
	// github.com/bob/bob2/b2.go
	// launchpad.net/nifty/n1.go
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
		// pop node from the queue (depth first traversal, reverse lexicographical order inside a directory)
		lq1 := len(queue) - 1
		currentNode, queue[lq1], queue = queue[lq1], nil, queue[:lq1]

		// Chop off the vendor root prefix, including the path separator, then
		// normalize.
		projectNormalized := filepath.ToSlash(currentNode.pathname[prefixLength:])

		if expectedSum, ok := wantSums[projectNormalized]; ok {
			ls := EmptyDigestInLock
			if len(expectedSum) > 0 {
				projectSum, err := DigestFromDirectory(filepath.Join(vendorRoot, projectNormalized))
				if err != nil {
					return nil, errors.Wrap(err, "cannot compute dependency hash")
				}
				if bytes.Equal(projectSum, expectedSum) {
					ls = NoMismatch
				} else {
					ls = DigestMismatchInLock
				}
			}
			status[projectNormalized] = ls

			// NOTE: Mark current nodes and all parents: required.
			for i := currentNode.myIndex; i != -1; i = nodes[i].parentIndex {
				nodes[i].isRequiredAncestor = true
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
				if fi.Mode()&skipSpecialNodes != 0 {
					continue
				}
				// Keep track of all regular files and directories, but do not
				// need to visit files.
				nodes = append(nodes, otherNode)
				if fi.IsDir() {
					queue = append(queue, otherNode)
				}
			}
		}
	}

	// Ignoring first node in the list, walk nodes from last to first. Whenever
	// the current node is not required, but its parent is required, then the
	// current node ought to be marked as `NotInLock`.
	for len(nodes) > 1 {
		// pop off right-most node from slice of nodes
		ln1 := len(nodes) - 1
		currentNode, nodes[ln1], nodes = nodes[ln1], nil, nodes[:ln1]

		if !currentNode.isRequiredAncestor && nodes[currentNode.parentIndex].isRequiredAncestor {
			status[filepath.ToSlash(currentNode.pathname[prefixLength:])] = NotInLock
		}
	}
	currentNode, nodes = nil, nil

	return status, nil
}
