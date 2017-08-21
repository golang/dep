package pkgtree

import (
	"bytes"
	"io"
)

// lineEndingReader is a `io.Reader` that converts CRLF sequences to LF.
//
// When cloning or checking out repositories, some Version Control Systems,
// VCSs, on some supported Go Operating System architectures, GOOS, will
// automatically convert line endings that end in a single line feed byte, LF,
// to line endings that end in a two byte sequence of carriage return, CR,
// followed by LF. This LF to CRLF conversion would cause otherwise identical
// versioned files to have different on disk contents simply based on which VCS
// and GOOS are involved. Different file contents for the same file would cause
// the resultant hashes to differ. In order to ensure file contents normalize
// and produce the same hash, this structure wraps an io.Reader that modifies
// the file's contents when it is read, translating all CRLF sequences to LF.
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
		// Read one fewer bytes so we have room if the first byte of the
		// upcoming Read is not a LF, in which case we will need to insert
		// trailing CR from previous read.
		buflen--
	}
	nr, er := f.src.Read(buf[:buflen])
	if nr > 0 {
		if f.prevReadEndedCR && buf[0] != '\n' {
			// Having a CRLF split across two Read operations is rare, so the
			// performance impact of copying entire buffer to the right by one
			// byte, while suboptimal, will at least will not happen very
			// often. This negative performance impact is mitigated somewhat on
			// many Go compilation architectures, GOARCH, because the `copy`
			// builtin uses a machine opcode for performing the memory copy on
			// possibly overlapping regions of memory. This machine opcodes is
			// not instantaneous and does require multiple CPU cycles to
			// complete, but is significantly faster than the application
			// looping through bytes.
			copy(buf[1:nr+1], buf[:nr]) // shift data to right one byte
			buf[0] = '\r'               // insert the previous skipped CR byte at start of buf
			nr++                        // pretend we read one more byte
		}

		// Remove any CRLF sequences in the buffer using `bytes.Index` because,
		// like the `copy` builtin on many GOARCHs, it also takes advantage of a
		// machine opcode to search for byte patterns.
		var searchOffset int // index within buffer from whence the search will commence for each loop; set to the index of the end of the previous loop.
		var shiftCount int   // each subsequenct shift operation needs to shift bytes to the left by one more position than the shift that preceded it.
		previousIndex := -1  // index of previously found CRLF; -1 means no previous index
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
