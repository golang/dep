package pkgtree

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func getTestdataVerifyRoot(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(cwd, "..", "testdata_digest")
}

func TestDigestFromPathnameWithFile(t *testing.T) {
	relativePathname := "github.com/alice/alice1/a.go"
	want := []byte{0xc6, 0xd1, 0x58, 0x5f, 0x86, 0x60, 0xea, 0x3d, 0x44, 0xce, 0x51, 0x29, 0x92, 0xbc, 0xd9, 0xbe, 0x67, 0x7d, 0xe9, 0xd8, 0xc6, 0xb0, 0x6b, 0x21, 0x98, 0x9e, 0x24, 0xb0, 0x34, 0xd1, 0xe5, 0x3c}

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
	relativePathname := "launchpad.net/nifty"
	want := []byte{0xf9, 0xd5, 0xc4, 0x87, 0x54, 0x96, 0xd1, 0x90, 0x93, 0xf0, 0xbb, 0x6b, 0x9c, 0x13, 0x5c, 0x6b, 0xd1, 0x9b, 0xcc, 0xe2, 0x30, 0x91, 0xd4, 0xc4, 0x85, 0x8e, 0xa5, 0xb0, 0x8d, 0x99, 0xf0, 0xe2}

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

	status, err := VerifyDepTree(vendorRoot, map[string][]byte{
		// matching result
		"github.com/alice/alice1": []byte{0x53, 0x2c, 0x1e, 0xda, 0x88, 0x71, 0xa0, 0x12, 0x81, 0xc1, 0xc8, 0xc3, 0xfb, 0xc3, 0x99, 0x9a, 0xe2, 0x95, 0xb1, 0x56, 0xe4, 0xfa, 0x9d, 0x88, 0x16, 0xc8, 0x69, 0x90, 0x74, 0x0, 0xc2, 0xb},

		// mismatching result
		"github.com/alice/alice2": []byte("non matching digest"),

		// not in tree result ought to superseede empty hash result
		"github.com/charlie/notInTree": nil,

		// another matching result
		"github.com/bob/bob1": []byte{0x4c, 0x86, 0xa, 0x58, 0x43, 0xc5, 0xbe, 0xa6, 0xe4, 0xe4, 0xbc, 0xa6, 0xbb, 0x86, 0x57, 0x7a, 0x9e, 0x55, 0x95, 0x1b, 0x77, 0x90, 0x94, 0xe0, 0x9, 0x40, 0xc8, 0x4b, 0x9e, 0xb1, 0xed, 0x4b},

		// empty hash result
		"github.com/bob/bob2": nil,

		// matching result at seldomly found directory level
		"launchpad.net/nifty": []byte{0xf9, 0xd5, 0xc4, 0x87, 0x54, 0x96, 0xd1, 0x90, 0x93, 0xf0, 0xbb, 0x6b, 0x9c, 0x13, 0x5c, 0x6b, 0xd1, 0x9b, 0xcc, 0xe2, 0x30, 0x91, 0xd4, 0xc4, 0x85, 0x8e, 0xa5, 0xb0, 0x8d, 0x99, 0xf0, 0xe2},
	})
	if err != nil {
		t.Fatal(err)
	}

	// NOTE: When true, display the vendor status map.
	if false {
		for k := range status {
			digest, err := DigestFromPathname(vendorRoot, k)
			if err != nil {
				t.Error(err)
			}
			t.Logf("%q\t%#v", k, digest)
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

	checkStatus(t, status, "github.com/alice/alice1", NoMismatch)
	checkStatus(t, status, "github.com/alice/alice2", DigestMismatchInLock)
	checkStatus(t, status, "github.com/alice/notInLock", NotInLock)
	checkStatus(t, status, "github.com/bob/bob1", NoMismatch)
	checkStatus(t, status, "github.com/bob/bob2", EmptyDigestInLock)
	checkStatus(t, status, "github.com/charlie/notInTree", NotInTree)
	checkStatus(t, status, "launchpad.net/nifty", NoMismatch)
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
