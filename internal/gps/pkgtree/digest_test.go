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
	want := []byte{0xef, 0x1e, 0x30, 0x16, 0xc4, 0xb1, 0xdb, 0xd9, 0x38, 0x65, 0xec, 0x90, 0xca, 0xad, 0x89, 0x52, 0xf9, 0x56, 0xb5, 0xa7, 0xfd, 0x83, 0x8e, 0xd7, 0x21, 0x6b, 0xbd, 0x63, 0x9a, 0xde, 0xc3, 0xeb}

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
	want := []byte{0x8, 0xe5, 0x5b, 0xb6, 0xb8, 0x44, 0x91, 0x80, 0x46, 0xca, 0xc6, 0x2e, 0x44, 0x7b, 0x42, 0xd4, 0xfb, 0x2d, 0xfd, 0x4c, 0xd9, 0xc9, 0xd, 0x38, 0x23, 0xed, 0xa5, 0xf4, 0xbc, 0x69, 0xd4, 0x8b}

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
		"github.com/alice/match":       []byte{0x13, 0x22, 0x4c, 0xd5, 0x5b, 0x3e, 0x29, 0xe5, 0x40, 0x23, 0x56, 0xc7, 0xc1, 0x50, 0x4, 0x1, 0x60, 0x46, 0xd4, 0x14, 0x9b, 0x63, 0x88, 0x37, 0xeb, 0x53, 0xfc, 0xe7, 0x63, 0x53, 0xf3, 0xf6},
		"github.com/alice/mismatch":    []byte("some non-matching digest"),
		"github.com/bob/emptyDigest":   nil, // empty hash result
		"github.com/charlie/notInTree": nil, // not in tree result ought to superseede empty digest result
		"github.com/bob/match":         []byte{0x73, 0xf5, 0xdc, 0xad, 0x39, 0x31, 0x25, 0x78, 0x9d, 0x2c, 0xbe, 0xd0, 0x9e, 0x6, 0x40, 0x64, 0xcd, 0x32, 0x9c, 0x28, 0xf4, 0xa0, 0xd2, 0xa4, 0x46, 0xca, 0x1f, 0x13, 0xfb, 0xaf, 0x53, 0x96},
		// matching result at seldomly found directory level
		"launchpad.net/match": []byte{0x8, 0xe5, 0x5b, 0xb6, 0xb8, 0x44, 0x91, 0x80, 0x46, 0xca, 0xc6, 0x2e, 0x44, 0x7b, 0x42, 0xd4, 0xfb, 0x2d, 0xfd, 0x4c, 0xd9, 0xc9, 0xd, 0x38, 0x23, 0xed, 0xa5, 0xf4, 0xbc, 0x69, 0xd4, 0x8b},
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
		if ok != true {
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

type crossBuffer struct {
	readCount int
}

func (cb *crossBuffer) Read(buf []byte) (int, error) {
	const first = "this is the result\r\nfrom the first read\r"
	const second = "\nthis is the result\r\nfrom the second read\r"

	cb.readCount++

	switch cb.readCount {
	case 1:
		return copy(buf, first), nil
	case 2:
		return copy(buf, second), nil
	default:
		return 0, io.EOF
	}
}

func TestLineEndingWriteToCrossBufferCRLF(t *testing.T) {
	src := &lineEndingWriterTo{new(crossBuffer)}
	dst := new(bytes.Buffer)

	// the final CR byte ought to be conveyed to destination
	const output = "this is the result\nfrom the first read\nthis is the result\nfrom the second read\r"

	n, err := io.Copy(dst, src)
	if got, want := err, error(nil); got != want {
		t.Errorf("(GOT): %v; (WNT): %v", got, want)
	}
	if got, want := n, int64(len(output)); got != want {
		t.Errorf("(GOT): %v; (WNT): %v", got, want)
	}
	if got, want := dst.Bytes(), []byte(output); !bytes.Equal(got, want) {
		t.Errorf("(GOT): %#q; (WNT): %#q", got, want)
	}
}

func TestLineEndingWriteTo(t *testing.T) {
	const input = "now is the time\r\nfor all good engineers\r\nto improve their test coverage!\r\n"
	const output = "now is the time\nfor all good engineers\nto improve their test coverage!\n"

	src := &lineEndingWriterTo{bytes.NewBufferString(input)}
	dst := new(bytes.Buffer)

	n, err := io.Copy(dst, src)
	if got, want := err, error(nil); got != want {
		t.Errorf("(GOT): %v; (WNT): %v", got, want)
	}
	if got, want := n, int64(len(output)); got != want {
		t.Errorf("(GOT): %v; (WNT): %v", got, want)
	}
	if got, want := dst.Bytes(), []byte(output); !bytes.Equal(got, want) {
		t.Errorf("(GOT): %#q; (WNT): %#q", got, want)
	}
}
