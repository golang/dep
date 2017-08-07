package pkgtree

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// prefix == "."
// symlink referent normalization

func getTestdataVerifyRoot(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(cwd, "..", "testdata_digest")
}

func TestDigestFromPathnameWithFile(t *testing.T) {
	relativePathname := "github.com/alice/alice1/a.go"
	want := []byte{
		0x1e, 0x86, 0x4d, 0x08, 0xeb, 0x0e, 0xa4, 0xb4,
		0x61, 0xba, 0x86, 0xe4, 0x2d, 0x1a, 0x1e, 0x75,
		0xf6, 0xa8, 0x7c, 0x0b, 0x53, 0x4d, 0x77, 0x4b,
		0x7b, 0xeb, 0x14, 0xe1, 0x99, 0x80, 0x0d, 0x04,
	}

	// NOTE: Create the hash using both an absolute and a relative pathname to
	// ensure hash ignores prefix.

	t.Run("AbsolutePrefix", func(t *testing.T) {
		prefix := getTestdataVerifyRoot(t)
		got, err := DigestFromPathname(prefix, relativePathname)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("\n(GOT):\n\t%x\n(WNT):\n\t%x", got, want)
		}
	})

	t.Run("RelativePrefix", func(t *testing.T) {
		prefix := "../testdata_digest"
		got, err := DigestFromPathname(prefix, relativePathname)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("\n(GOT):\n\t%x\n(WNT):\n\t%x", got, want)
		}
	})
}

func TestDigestFromPathnameWithDirectory(t *testing.T) {
	relativePathname := "launchpad.net/nifty"
	want := []byte{
		0x3c, 0xbb, 0x1c, 0x51, 0x76, 0x88, 0x88, 0x67,
		0x5d, 0x2b, 0x32, 0x5f, 0x44, 0xc0, 0x6b, 0x5d,
		0x72, 0x24, 0x99, 0x35, 0x3b, 0x94, 0x7e, 0xae,
		0x6b, 0xe4, 0xc3, 0xce, 0xd1, 0x2c, 0x30, 0xf0,
	}

	// NOTE: Create the hash using both an absolute and a relative pathname to
	// ensure hash ignores prefix.

	t.Run("AbsolutePrefix", func(t *testing.T) {
		prefix := getTestdataVerifyRoot(t)
		got, err := DigestFromPathname(prefix, relativePathname)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("\n(GOT):\n\t%x\n(WNT):\n\t%x", got, want)
		}
	})

	t.Run("RelativePrefix", func(t *testing.T) {
		prefix := "../testdata_digest"
		got, err := DigestFromPathname(prefix, relativePathname)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("\n(GOT):\n\t%x\n(WNT):\n\t%x", got, want)
		}
	})

}

func TestVerifyDepTree(t *testing.T) {
	vendorRoot := getTestdataVerifyRoot(t)

	status, err := VerifyDepTree(vendorRoot, map[string][]byte{
		// matching result
		"github.com/alice/alice1": []byte{
			0x32, 0x6c, 0xd0, 0x36, 0xbb, 0xd4, 0xd0, 0x06, 0x34, 0x41, 0xda, 0x80, 0x2f, 0xe3, 0x31, 0xa0, 0x2b, 0xe7, 0xd6, 0x5d, 0x4c, 0xea, 0xf0, 0xe7, 0xf6, 0x46, 0x23, 0x6c, 0xa9, 0x02, 0x3a, 0x35,
		},

		// mismatching result
		"github.com/alice/alice2": []byte("non matching digest"),

		// not in tree result ought to superseede empty hash result
		"github.com/charlie/notInTree": nil,

		// another matching result
		"github.com/bob/bob1": []byte{
			0x2b, 0xcf, 0xe2, 0xc4, 0x67, 0x77, 0xf2, 0x7d, 0x30, 0x7e, 0x1e, 0xba, 0xc3, 0x90, 0xdc, 0x2a, 0xdd, 0x3a, 0x91, 0x98, 0x09, 0xcd, 0x50, 0x0d, 0x7e, 0x51, 0x99, 0xf4, 0x96, 0x59, 0x1d, 0xd2,
		},

		// empty hash result
		"github.com/bob/bob2": nil,

		// matching result at seldomly found directory level
		"launchpad.net/nifty": []byte{
			0x3c, 0xbb, 0x1c, 0x51, 0x76, 0x88, 0x88, 0x67, 0x5d, 0x2b, 0x32, 0x5f, 0x44, 0xc0, 0x6b, 0x5d, 0x72, 0x24, 0x99, 0x35, 0x3b, 0x94, 0x7e, 0xae, 0x6b, 0xe4, 0xc3, 0xce, 0xd1, 0x2c, 0x30, 0xf0,
		},
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
			t.Logf("%q\t%x", k, digest)
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
