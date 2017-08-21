// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkgtree

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func getTestdataVerifyRoot(tb testing.TB) string {
	cwd, err := os.Getwd()
	if err != nil {
		tb.Fatal(err)
	}
	return filepath.Join(filepath.Dir(cwd), "testdata_digest")
}

func TestDigestFromDirectoryBailsUnlessDirectory(t *testing.T) {
	prefix := getTestdataVerifyRoot(t)
	relativePathname := "launchpad.net/match"
	_, err := DigestFromDirectory(filepath.Join(prefix, relativePathname))
	if got, want := err, error(nil); got != want {
		t.Errorf("\n(GOT): %v; (WNT): %v", got, want)
	}
}

func TestDigestFromDirectory(t *testing.T) {
	relativePathname := "launchpad.net/match"
	want := []byte{0x7e, 0x10, 0x6, 0x2f, 0x8, 0x3, 0x3c, 0x76, 0xae, 0xbc, 0xa4, 0xc9, 0xec, 0x73, 0x67, 0x15, 0x70, 0x2b, 0x0, 0x89, 0x27, 0xbb, 0x61, 0x9d, 0xc7, 0xc3, 0x39, 0x46, 0x3, 0x91, 0xb7, 0x3b}

	// NOTE: Create the hash using both an absolute and a relative pathname to
	// ensure hash ignores prefix.

	t.Run("AbsolutePrefix", func(t *testing.T) {
		prefix := getTestdataVerifyRoot(t)
		got, err := DigestFromDirectory(filepath.Join(prefix, relativePathname))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("\n(GOT):\n\t%#v\n(WNT):\n\t%#v", got, want)
		}
	})

	t.Run("RelativePrefix", func(t *testing.T) {
		prefix := "../testdata_digest"
		got, err := DigestFromDirectory(filepath.Join(prefix, relativePathname))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("\n(GOT):\n\t%#v\n(WNT):\n\t%#v", got, want)
		}
	})
}

func BenchmarkDigestFromDirectory(b *testing.B) {
	prefix := getTestdataVerifyRoot(b)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := DigestFromDirectory(prefix)
		if err != nil {
			b.Fatal(err)
		}
	}
}

const flameIterations = 100000

func BenchmarkFlameDigestFromDirectory(b *testing.B) {
	prefix := getTestdataVerifyRoot(b)
	b.ResetTimer()

	for i := 0; i < flameIterations; i++ {
		_, err := DigestFromDirectory(prefix)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestVerifyDepTree(t *testing.T) {
	vendorRoot := getTestdataVerifyRoot(t)

	wantSums := map[string][]byte{
		"github.com/alice/match":       []byte{0x7e, 0x10, 0x6, 0x2f, 0x8, 0x3, 0x3c, 0x76, 0xae, 0xbc, 0xa4, 0xc9, 0xec, 0x73, 0x67, 0x15, 0x70, 0x2b, 0x0, 0x89, 0x27, 0xbb, 0x61, 0x9d, 0xc7, 0xc3, 0x39, 0x46, 0x3, 0x91, 0xb7, 0x3b},
		"github.com/alice/mismatch":    []byte("some non-matching digest"),
		"github.com/bob/emptyDigest":   nil, // empty hash result
		"github.com/bob/match":         []byte{0x7e, 0x10, 0x6, 0x2f, 0x8, 0x3, 0x3c, 0x76, 0xae, 0xbc, 0xa4, 0xc9, 0xec, 0x73, 0x67, 0x15, 0x70, 0x2b, 0x0, 0x89, 0x27, 0xbb, 0x61, 0x9d, 0xc7, 0xc3, 0x39, 0x46, 0x3, 0x91, 0xb7, 0x3b},
		"github.com/charlie/notInTree": nil, // not in tree result ought to superseede empty digest result
		// matching result at seldomly found directory level
		"launchpad.net/match": []byte{0x7e, 0x10, 0x6, 0x2f, 0x8, 0x3, 0x3c, 0x76, 0xae, 0xbc, 0xa4, 0xc9, 0xec, 0x73, 0x67, 0x15, 0x70, 0x2b, 0x0, 0x89, 0x27, 0xbb, 0x61, 0x9d, 0xc7, 0xc3, 0x39, 0x46, 0x3, 0x91, 0xb7, 0x3b},
	}

	status, err := VerifyDepTree(vendorRoot, wantSums)
	if err != nil {
		t.Fatal(err)
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

	if t.Failed() {
		for k, want := range wantSums {
			got, err := DigestFromDirectory(filepath.Join(vendorRoot, k))
			if err != nil {
				t.Error(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%q\n(GOT):\n\t%#v\n(WNT):\n\t%#v", k, got, want)
			}
		}
	}
}

func BenchmarkVerifyDepTree(b *testing.B) {
	prefix := getTestdataVerifyRoot(b)

	for i := 0; i < b.N; i++ {
		_, err := VerifyDepTree(prefix, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
