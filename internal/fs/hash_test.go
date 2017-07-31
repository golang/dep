package fs

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/pkg/errors"
)

func TestHashFromNodeWithFile(t *testing.T) {
	actual, err := HashFromNode("", "./testdata/blob")
	if err != nil {
		t.Fatal(err)
	}
	expected := "bf7c45881248f74466f9624e8336747277d7901a4f7af43940be07c5539b78a8"
	if actual != expected {
		t.Errorf("Actual:\n\t%#q\nExpected:\n\t%#q", actual, expected)
	}
}

func TestHashFromNodeWithDirectory(t *testing.T) {
	actual, err := HashFromNode("../fs", "testdata/recursive")
	if err != nil {
		t.Fatal(err)
	}
	expected := "d5ac28114417eae59b9ac02e3fac5bdff673e93cc91b408cde1989e1cd2efbd0"
	if actual != expected {
		t.Errorf("Actual:\n\t%#q\nExpected:\n\t%#q", actual, expected)
	}
}

const benchmarkSource = "/usr/local/Cellar/go/1.8.3/libexec/src"

func BenchmarkHashFromNode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := HashFromNode("", benchmarkSource)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashFromNodeUsingWalk(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := hashFromNodeUsingWalk(benchmarkSource)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// hashFromNodeUsingWalk uses `filepath.Walk` in order to establish a
// performance benchmark. The actual hash function cannot use `filepath.Walk`
// because we want to detect symbolic links and include their referent in the
// hash output.
func hashFromNodeUsingWalk(pathname string) (hash string, err error) {
	h := sha256.New()

	err = filepath.Walk(filepath.Clean(pathname), func(pathname string, fi os.FileInfo, err error) error {
		if err != nil && err != filepath.SkipDir {
			return err
		}

		switch fi.Name() {
		case ".", "..", "vendor", ".bzr", ".git", ".hg", ".svn":
			return filepath.SkipDir
		}

		_, _ = h.Write([]byte(pathname))

		if fi.IsDir() {
			return nil
		}

		fi, er := os.Stat(pathname)
		if er != nil {
			return errors.Wrap(er, "cannot Stat")
		}

		fh, er := os.Open(pathname)
		if er != nil {
			return errors.Wrap(er, "cannot Open")
		}

		_, _ = h.Write([]byte(strconv.FormatInt(fi.Size(), 10))) // format file size as base 10 integer
		_, er = io.Copy(h, fh)
		err = errors.Wrap(er, "cannot Copy") // errors.Wrap only wraps non-nil, so elide checking here

		// NOTE: Close the file handle to the open directory or file.
		if er = fh.Close(); err == nil {
			err = errors.Wrap(er, "cannot Close")
		}
		return err
	})

	hash = fmt.Sprintf("%x", h.Sum(nil))
	return
}
