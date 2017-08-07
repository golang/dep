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
		0x36, 0xe6, 0x36, 0xcf, 0x62, 0x5e, 0x18, 0xcc,
		0x0e, 0x0d, 0xed, 0xad, 0x6d, 0x69, 0x08, 0x80,
		0x54, 0xec, 0x69, 0xde, 0x58, 0xa1, 0x2e, 0x09,
		0x9f, 0x8f, 0x4a, 0xba, 0x44, 0x2f, 0xae, 0xc8,
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
		0x9c, 0x09, 0xeb, 0x81, 0xd0, 0x47, 0x4f, 0x68,
		0xb5, 0x50, 0xe0, 0x94, 0x94, 0x9a, 0x41, 0x80,
		0x28, 0xef, 0x63, 0x35, 0x6f, 0x64, 0x92, 0x7e,
		0x6a, 0x43, 0xd7, 0x9d, 0x45, 0xf9, 0x8a, 0xeb,
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
			0x53, 0xed, 0x38, 0xc0, 0x69, 0x34, 0xf0, 0x76,
			0x26, 0x62, 0xe5, 0xa1, 0xa4, 0xa9, 0xe9, 0x23,
			0x73, 0xf0, 0x02, 0x03, 0xfa, 0x43, 0xd0, 0x7e,
			0x7a, 0x29, 0x89, 0xae, 0x4c, 0x44, 0x50, 0x11,
		},

		// mismatching result
		"github.com/alice/alice2": []byte("non matching digest"),

		// not in tree result ought to superseede empty hash result
		"github.com/charlie/notInTree": nil,

		// another matching result
		"github.com/bob/bob1": []byte{
			0x75, 0xc0, 0x5a, 0x13, 0xb0, 0xd6, 0x19, 0x5d,
			0x36, 0x26, 0xf4, 0xbc, 0xcb, 0x4d, 0x92, 0x08,
			0x4d, 0x03, 0xe0, 0x70, 0x11, 0x53, 0xcd, 0x5c,
			0x6a, 0x53, 0xc6, 0x31, 0x84, 0x34, 0x4f, 0xf4,
		},

		// empty hash result
		"github.com/bob/bob2": nil,

		// matching result at seldomly found directory level
		"launchpad.net/nifty": []byte{
			0x9c, 0x09, 0xeb, 0x81, 0xd0, 0x47, 0x4f, 0x68, 0xb5, 0x50, 0xe0, 0x94, 0x94, 0x9a, 0x41, 0x80, 0x28, 0xef, 0x63, 0x35, 0x6f, 0x64, 0x92, 0x7e, 0x6a, 0x43, 0xd7, 0x9d, 0x45, 0xf9, 0x8a, 0xeb,
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
