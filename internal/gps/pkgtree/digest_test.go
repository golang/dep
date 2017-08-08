// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkgtree

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// crossBuffer is a test io.Reader that emits a few canned responses.
type crossBuffer struct {
	readCount  int
	iterations []string
}

func (cb *crossBuffer) Read(buf []byte) (int, error) {
	if cb.readCount == len(cb.iterations) {
		return 0, io.EOF
	}
	cb.readCount++
	return copy(buf, cb.iterations[cb.readCount-1]), nil
}

func streamThruLineEndingReader(t *testing.T, iterations []string) []byte {
	dst := new(bytes.Buffer)
	n, err := io.Copy(dst, newLineEndingReader(&crossBuffer{iterations: iterations}))
	if got, want := err, error(nil); got != want {
		t.Errorf("(GOT): %v; (WNT): %v", got, want)
	}
	if got, want := n, int64(dst.Len()); got != want {
		t.Errorf("(GOT): %v; (WNT): %v", got, want)
	}
	return dst.Bytes()
}

func TestLineEndingReader(t *testing.T) {
	testCases := []struct {
		input  []string
		output string
	}{
		{[]string{"now is the time\r\n"}, "now is the time\n"},
		{[]string{"now is the time\n"}, "now is the time\n"},
		{[]string{"now is the time\r"}, "now is the time\r"},     // trailing CR ought to convey
		{[]string{"\rnow is the time"}, "\rnow is the time"},     // CR not followed by LF ought to convey
		{[]string{"\rnow is the time\r"}, "\rnow is the time\r"}, // CR not followed by LF ought to convey

		{[]string{"this is the result\r\nfrom the first read\r", "\nthis is the result\r\nfrom the second read\r"},
			"this is the result\nfrom the first read\nthis is the result\nfrom the second read\r"},
		{[]string{"now is the time\r\nfor all good engineers\r\nto improve their test coverage!\r\n"},
			"now is the time\nfor all good engineers\nto improve their test coverage!\n"},
		{[]string{"now is the time\r\nfor all good engineers\r", "\nto improve their test coverage!\r\n"},
			"now is the time\nfor all good engineers\nto improve their test coverage!\n"},
	}

	for _, testCase := range testCases {
		got := streamThruLineEndingReader(t, testCase.input)
		if want := []byte(testCase.output); !bytes.Equal(got, want) {
			t.Errorf("Input: %#v; (GOT): %#q; (WNT): %#q", testCase.input, got, want)
		}
	}
}

////////////////////////////////////////

func getTestdataVerifyRoot(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	parent, _ := filepath.Split(cwd)
	return filepath.Join(parent, "testdata_digest")
}

func TestDigestFromPathnameWithFile(t *testing.T) {
	relativePathname := "github.com/alice/match/match.go"
	want := []byte{0x43, 0xe6, 0x3, 0x53, 0xda, 0x4, 0x18, 0xf6, 0xbd, 0x98, 0xf4, 0x6c, 0x6d, 0xb8, 0xc1, 0x8d, 0xa2, 0x78, 0x16, 0x45, 0xf7, 0xca, 0xc, 0xec, 0xcf, 0x2e, 0xa1, 0x64, 0x55, 0x69, 0xbf, 0x8f}

	// NOTE: Create the hash using both an absolute and a relative pathname to
	// ensure hash ignores prefix.

	t.Run("AbsolutePrefix", func(t *testing.T) {
		prefix := getTestdataVerifyRoot(t)
		got, err := DigestFromPathname(prefix, relativePathname)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("\n(GOT):\n\t%#v\n(WNT):\n\t%#v", got, want)
		}
	})

	t.Run("RelativePrefix", func(t *testing.T) {
		prefix := "../testdata_digest"
		got, err := DigestFromPathname(prefix, relativePathname)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("\n(GOT):\n\t%#v\n(WNT):\n\t%#v", got, want)
		}
	})
}

func TestDigestFromPathnameWithDirectory(t *testing.T) {
	relativePathname := "launchpad.net/match"
	want := []byte{0x74, 0xe, 0x17, 0x87, 0xd4, 0x8e, 0x56, 0x2e, 0x7e, 0x32, 0x4e, 0x80, 0x3a, 0x5f, 0x3a, 0x10, 0x33, 0x43, 0x2c, 0x24, 0x8e, 0xf7, 0x1a, 0x37, 0x5e, 0x76, 0xf4, 0x6, 0x2b, 0xf3, 0xfd, 0x91}

	// NOTE: Create the hash using both an absolute and a relative pathname to
	// ensure hash ignores prefix.

	t.Run("AbsolutePrefix", func(t *testing.T) {
		prefix := getTestdataVerifyRoot(t)
		got, err := DigestFromPathname(prefix, relativePathname)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("\n(GOT):\n\t%#v\n(WNT):\n\t%#v", got, want)
		}
	})

	t.Run("RelativePrefix", func(t *testing.T) {
		prefix := "../testdata_digest"
		got, err := DigestFromPathname(prefix, relativePathname)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("\n(GOT):\n\t%#v\n(WNT):\n\t%#v", got, want)
		}
	})
}

func TestVerifyDepTree(t *testing.T) {
	vendorRoot := getTestdataVerifyRoot(t)

	wantSums := map[string][]byte{
		"github.com/alice/match":       []byte{0x87, 0x4, 0x53, 0x5f, 0xb8, 0xca, 0xe9, 0x52, 0x40, 0x41, 0xcd, 0x93, 0x2e, 0x42, 0xfb, 0x14, 0x77, 0x57, 0x9c, 0x3, 0x6b, 0xe1, 0x15, 0xe7, 0xfa, 0xc1, 0xf0, 0x98, 0x4b, 0x61, 0x9f, 0x48},
		"github.com/alice/mismatch":    []byte("some non-matching digest"),
		"github.com/bob/emptyDigest":   nil, // empty hash result
		"github.com/bob/match":         []byte{0x6a, 0x11, 0xf9, 0x46, 0xcc, 0xf3, 0x44, 0xdb, 0x2c, 0x6b, 0xcc, 0xb4, 0x0, 0x71, 0xe5, 0xc4, 0xee, 0x9a, 0x26, 0x71, 0x9d, 0xab, 0xe2, 0x40, 0xb7, 0xbf, 0x2a, 0xd9, 0x4, 0xcf, 0xc9, 0x46},
		"github.com/charlie/notInTree": nil, // not in tree result ought to superseede empty digest result
		// matching result at seldomly found directory level
		"launchpad.net/match": []byte{0x74, 0xe, 0x17, 0x87, 0xd4, 0x8e, 0x56, 0x2e, 0x7e, 0x32, 0x4e, 0x80, 0x3a, 0x5f, 0x3a, 0x10, 0x33, 0x43, 0x2c, 0x24, 0x8e, 0xf7, 0x1a, 0x37, 0x5e, 0x76, 0xf4, 0x6, 0x2b, 0xf3, 0xfd, 0x91},
	}

	status, err := VerifyDepTree(vendorRoot, wantSums)
	if err != nil {
		t.Fatal(err)
	}

	// NOTE: When true, display the digests of the directories specified by the
	// digest keys.
	if false {
		for k, want := range wantSums {
			got, err := DigestFromPathname(vendorRoot, k)
			if err != nil {
				t.Error(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%q\n(GOT):\n\t%#v\n(WNT):\n\t%#v", k, got, want)
			}
		}
	}

	if got, want := len(status), 7; got != want {
		t.Errorf("\n(GOT): %v; (WNT): %v", got, want)
	}

	checkStatus := func(t *testing.T, status map[string]VendorStatus, key string, want VendorStatus) {
		got, ok := status[key]
		if !ok {
			t.Errorf("Want key: %q", key)
			return
		}
		if got != want {
			t.Errorf("Key: %q; (GOT): %v; (WNT): %v", key, got, want)
		}
	}

	checkStatus(t, status, "github.com/alice/match", NoMismatch)
	checkStatus(t, status, "github.com/alice/mismatch", DigestMismatchInLock)
	checkStatus(t, status, "github.com/alice/notInLock", NotInLock)
	checkStatus(t, status, "github.com/bob/match", NoMismatch)
	checkStatus(t, status, "github.com/bob/emptyDigest", EmptyDigestInLock)
	checkStatus(t, status, "github.com/charlie/notInTree", NotInTree)
	checkStatus(t, status, "launchpad.net/match", NoMismatch)
}

func BenchmarkVerifyDepTree(b *testing.B) {
	b.Skip("Eliding benchmark of user's Go source directory")

	prefix := filepath.Join(os.Getenv("GOPATH"), "src")

	for i := 0; i < b.N; i++ {
		_, err := VerifyDepTree(prefix, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
