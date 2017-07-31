// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fs

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/golang/dep/internal/test"
)

// This function tests HadFilepathPrefix. It should test it on both case
// sensitive and insensitive situations. However, the only reliable way to test
// case-insensitive behaviour is if using case-insensitive filesystem.  This
// cannot be guaranteed in an automated test. Therefore, the behaviour of the
// tests is not to test case sensitivity on *nix and to assume that Windows is
// case-insensitive. Please see link below for some background.
//
// https://superuser.com/questions/266110/how-do-you-make-windows-7-fully-case-sensitive-with-respect-to-the-filesystem
//
// NOTE: NTFS can be made case-sensitive. However many Windows programs,
// including Windows Explorer do not handle gracefully multiple files that
// differ only in capitalization. It is possible that this can cause these tests
// to fail on some setups.
func TestHasFilepathPrefix(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// dir2 is the same as dir but with different capitalization on Windows to
	// test case insensitivity
	var dir2 string
	if runtime.GOOS == "windows" {
		dir = strings.ToLower(dir)
		dir2 = strings.ToUpper(dir)
	} else {
		dir2 = dir
	}

	cases := []struct {
		path   string
		prefix string
		want   bool
	}{
		{filepath.Join(dir, "a", "b"), filepath.Join(dir2), true},
		{filepath.Join(dir, "a", "b"), filepath.Join(dir2, "a"), true},
		{filepath.Join(dir, "a", "b"), filepath.Join(dir2, "a", "b"), true},
		{filepath.Join(dir, "a", "b"), filepath.Join(dir2, "c"), false},
		{filepath.Join(dir, "a", "b"), filepath.Join(dir2, "a", "d", "b"), false},
		{filepath.Join(dir, "a", "b"), filepath.Join(dir2, "a", "b2"), false},
		{filepath.Join(dir), filepath.Join(dir2, "a", "b"), false},
		{filepath.Join(dir, "ab"), filepath.Join(dir2, "a", "b"), false},
		{filepath.Join(dir, "ab"), filepath.Join(dir2, "a"), false},
		{filepath.Join(dir, "123"), filepath.Join(dir2, "123"), true},
		{filepath.Join(dir, "123"), filepath.Join(dir2, "1"), false},
		{filepath.Join(dir, "⌘"), filepath.Join(dir2, "⌘"), true},
		{filepath.Join(dir, "a"), filepath.Join(dir2, "⌘"), false},
		{filepath.Join(dir, "⌘"), filepath.Join(dir2, "a"), false},
	}

	for _, c := range cases {
		if err := os.MkdirAll(c.path, 0755); err != nil {
			t.Fatal(err)
		}

		if err = os.MkdirAll(c.prefix, 0755); err != nil {
			t.Fatal(err)
		}

		if got := HasFilepathPrefix(c.path, c.prefix); c.want != got {
			t.Fatalf("dir: %q, prefix: %q, expected: %v, got: %v", c.path, c.prefix, c.want, got)
		}
	}
}

// This function tests HadFilepathPrefix. It should test it on both case
// sensitive and insensitive situations. However, the only reliable way to test
// case-insensitive behaviour is if using case-insensitive filesystem.  This
// cannot be guaranteed in an automated test. Therefore, the behaviour of the
// tests is not to test case sensitivity on *nix and to assume that Windows is
// case-insensitive. Please see link below for some background.
//
// https://superuser.com/questions/266110/how-do-you-make-windows-7-fully-case-sensitive-with-respect-to-the-filesystem
//
// NOTE: NTFS can be made case-sensitive. However many Windows programs,
// including Windows Explorer do not handle gracefully multiple files that
// differ only in capitalization. It is possible that this can cause these tests
// to fail on some setups.
func TestHasFilepathPrefix_Files(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// dir2 is the same as dir but with different capitalization on Windows to
	// test case insensitivity
	var dir2 string
	if runtime.GOOS == "windows" {
		dir = strings.ToLower(dir)
		dir2 = strings.ToUpper(dir)
	} else {
		dir2 = dir
	}

	existingFile := filepath.Join(dir, "exists")
	if err = os.MkdirAll(existingFile, 0755); err != nil {
		t.Fatal(err)
	}

	nonExistingFile := filepath.Join(dir, "does_not_exists")

	cases := []struct {
		path   string
		prefix string
		want   bool
	}{
		{existingFile, filepath.Join(dir2), true},
		{nonExistingFile, filepath.Join(dir2), false},
	}

	for _, c := range cases {
		if got := HasFilepathPrefix(c.path, c.prefix); c.want != got {
			t.Fatalf("dir: %q, prefix: %q, expected: %v, got: %v", c.path, c.prefix, c.want, got)
		}
	}
}

func TestRenameWithFallback(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	if err = RenameWithFallback(filepath.Join(dir, "does_not_exists"), filepath.Join(dir, "dst")); err == nil {
		t.Fatal("expected an error for non existing file, but got nil")
	}

	srcpath := filepath.Join(dir, "src")

	if srcf, err := os.Create(srcpath); err != nil {
		t.Fatal(err)
	} else {
		srcf.Close()
	}

	if err = RenameWithFallback(srcpath, filepath.Join(dir, "dst")); err != nil {
		t.Fatal(err)
	}

	srcpath = filepath.Join(dir, "a")
	if err = os.MkdirAll(srcpath, 0777); err != nil {
		t.Fatal(err)
	}

	dstpath := filepath.Join(dir, "b")
	if err = os.MkdirAll(dstpath, 0777); err != nil {
		t.Fatal(err)
	}

	if err = RenameWithFallback(srcpath, dstpath); err == nil {
		t.Fatal("expected an error if dst is an existing directory, but got nil")
	}
}

func TestGenTestFilename(t *testing.T) {
	cases := []struct {
		str  string
		want string
	}{
		{"abc", "Abc"},
		{"ABC", "aBC"},
		{"AbC", "abC"},
		{"αβγ", "Αβγ"},
		{"123", "123"},
		{"1a2", "1A2"},
		{"12a", "12A"},
		{"⌘", "⌘"},
	}

	for _, c := range cases {
		got := genTestFilename(c.str)
		if c.want != got {
			t.Fatalf("str: %q, expected: %q, got: %q", c.str, c.want, got)
		}
	}
}

func BenchmarkGenTestFilename(b *testing.B) {
	cases := []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"αααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααααα",
		"11111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111",
		"⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘⌘",
	}

	for i := 0; i < b.N; i++ {
		for _, str := range cases {
			genTestFilename(str)
		}
	}
}

func TestCopyDir(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcdir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcdir, 0755); err != nil {
		t.Fatal(err)
	}

	files := []struct {
		path     string
		contents string
		fi       os.FileInfo
	}{
		{path: "myfile", contents: "hello world"},
		{path: filepath.Join("subdir", "file"), contents: "subdir file"},
	}

	// Create structure indicated in 'files'
	for i, file := range files {
		fn := filepath.Join(srcdir, file.path)
		dn := filepath.Dir(fn)
		if err = os.MkdirAll(dn, 0755); err != nil {
			t.Fatal(err)
		}

		fh, err := os.Create(fn)
		if err != nil {
			t.Fatal(err)
		}

		if _, err = fh.Write([]byte(file.contents)); err != nil {
			t.Fatal(err)
		}
		fh.Close()

		files[i].fi, err = os.Stat(fn)
		if err != nil {
			t.Fatal(err)
		}
	}

	destdir := filepath.Join(dir, "dest")
	if err := CopyDir(srcdir, destdir); err != nil {
		t.Fatal(err)
	}

	// Compare copy against structure indicated in 'files'
	for _, file := range files {
		fn := filepath.Join(srcdir, file.path)
		dn := filepath.Dir(fn)
		dirOK, err := IsDir(dn)
		if err != nil {
			t.Fatal(err)
		}
		if !dirOK {
			t.Fatalf("expected %s to be a directory", dn)
		}

		got, err := ioutil.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}

		if file.contents != string(got) {
			t.Fatalf("expected: %s, got: %s", file.contents, string(got))
		}

		gotinfo, err := os.Stat(fn)
		if err != nil {
			t.Fatal(err)
		}

		if file.fi.Mode() != gotinfo.Mode() {
			t.Fatalf("expected %s: %#v\n to be the same mode as %s: %#v",
				file.path, file.fi.Mode(), fn, gotinfo.Mode())
		}
	}
}

func TestCopyDirFail_SrcInaccessible(t *testing.T) {
	if runtime.GOOS == "windows" {
		// XXX: setting permissions works differently in
		// Microsoft Windows. Skipping this this until a
		// compatible implementation is provided.
		t.Skip("skipping on windows")
	}

	var srcdir, dstdir string

	cleanup := setupInaccessibleDir(t, func(dir string) error {
		srcdir = filepath.Join(dir, "src")
		return os.MkdirAll(srcdir, 0755)
	})
	defer cleanup()

	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	dstdir = filepath.Join(dir, "dst")
	if err = CopyDir(srcdir, dstdir); err == nil {
		t.Fatalf("expected error for CopyDir(%s, %s), got none", srcdir, dstdir)
	}
}

func TestCopyDirFail_DstInaccessible(t *testing.T) {
	if runtime.GOOS == "windows" {
		// XXX: setting permissions works differently in
		// Microsoft Windows. Skipping this this until a
		// compatible implementation is provided.
		t.Skip("skipping on windows")
	}

	var srcdir, dstdir string

	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcdir = filepath.Join(dir, "src")
	if err = os.MkdirAll(srcdir, 0755); err != nil {
		t.Fatal(err)
	}

	cleanup := setupInaccessibleDir(t, func(dir string) error {
		dstdir = filepath.Join(dir, "dst")
		return nil
	})
	defer cleanup()

	if err := CopyDir(srcdir, dstdir); err == nil {
		t.Fatalf("expected error for CopyDir(%s, %s), got none", srcdir, dstdir)
	}
}

func TestCopyDirFail_SrcIsNotDir(t *testing.T) {
	var srcdir, dstdir string

	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcdir = filepath.Join(dir, "src")
	if _, err = os.Create(srcdir); err != nil {
		t.Fatal(err)
	}

	dstdir = filepath.Join(dir, "dst")

	if err = CopyDir(srcdir, dstdir); err == nil {
		t.Fatalf("expected error for CopyDir(%s, %s), got none", srcdir, dstdir)
	}

	if err != errSrcNotDir {
		t.Fatalf("expected %v error for CopyDir(%s, %s), got %s", errSrcNotDir, srcdir, dstdir, err)
	}

}

func TestCopyDirFail_DstExists(t *testing.T) {
	var srcdir, dstdir string

	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcdir = filepath.Join(dir, "src")
	if err = os.MkdirAll(srcdir, 0755); err != nil {
		t.Fatal(err)
	}

	dstdir = filepath.Join(dir, "dst")
	if err = os.MkdirAll(dstdir, 0755); err != nil {
		t.Fatal(err)
	}

	if err = CopyDir(srcdir, dstdir); err == nil {
		t.Fatalf("expected error for CopyDir(%s, %s), got none", srcdir, dstdir)
	}

	if err != errDstExist {
		t.Fatalf("expected %v error for CopyDir(%s, %s), got %s", errDstExist, srcdir, dstdir, err)
	}
}

func TestCopyDirFailOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		// XXX: setting permissions works differently in
		// Microsoft Windows. os.Chmod(..., 0222) below is not
		// enough for the file to be readonly, and os.Chmod(...,
		// 0000) returns an invalid argument error. Skipping
		// this this until a compatible implementation is
		// provided.
		t.Skip("skipping on windows")
	}

	var srcdir, dstdir string

	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcdir = filepath.Join(dir, "src")
	if err = os.MkdirAll(srcdir, 0755); err != nil {
		t.Fatal(err)
	}

	srcfn := filepath.Join(srcdir, "file")
	srcf, err := os.Create(srcfn)
	if err != nil {
		t.Fatal(err)
	}
	srcf.Close()

	// setup source file so that it cannot be read
	if err = os.Chmod(srcfn, 0222); err != nil {
		t.Fatal(err)
	}

	dstdir = filepath.Join(dir, "dst")

	if err = CopyDir(srcdir, dstdir); err == nil {
		t.Fatalf("expected error for CopyDir(%s, %s), got none", srcdir, dstdir)
	}
}

func TestCopyFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcf, err := os.Create(filepath.Join(dir, "srcfile"))
	if err != nil {
		t.Fatal(err)
	}

	want := "hello world"
	if _, err := srcf.Write([]byte(want)); err != nil {
		t.Fatal(err)
	}
	srcf.Close()

	destf := filepath.Join(dir, "destf")
	if err := copyFile(srcf.Name(), destf); err != nil {
		t.Fatal(err)
	}

	got, err := ioutil.ReadFile(destf)
	if err != nil {
		t.Fatal(err)
	}

	if want != string(got) {
		t.Fatalf("expected: %s, got: %s", want, string(got))
	}

	wantinfo, err := os.Stat(srcf.Name())
	if err != nil {
		t.Fatal(err)
	}

	gotinfo, err := os.Stat(destf)
	if err != nil {
		t.Fatal(err)
	}

	if wantinfo.Mode() != gotinfo.Mode() {
		t.Fatalf("expected %s: %#v\n to be the same mode as %s: %#v", srcf.Name(), wantinfo.Mode(), destf, gotinfo.Mode())
	}
}

func TestCopyFileSymlink(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcPath := filepath.Join(dir, "src")
	symlinkPath := filepath.Join(dir, "symlink")
	dstPath := filepath.Join(dir, "dst")

	srcf, err := os.Create(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	srcf.Close()

	if err = os.Symlink(srcPath, symlinkPath); err != nil {
		t.Fatalf("could not create symlink: %s", err)
	}

	if err = copyFile(symlinkPath, dstPath); err != nil {
		t.Fatalf("failed to copy symlink: %s", err)
	}

	resolvedPath, err := os.Readlink(dstPath)
	if err != nil {
		t.Fatalf("could not resolve symlink: %s", err)
	}

	if resolvedPath != srcPath {
		t.Fatalf("resolved path is incorrect. expected %s, got %s", srcPath, resolvedPath)
	}
}

func TestCopyFileSymlinkToDirectory(t *testing.T) {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcPath := filepath.Join(dir, "src")
	symlinkPath := filepath.Join(dir, "symlink")
	dstPath := filepath.Join(dir, "dst")

	err = os.MkdirAll(srcPath, 0777)
	if err != nil {
		t.Fatal(err)
	}

	if err = os.Symlink(srcPath, symlinkPath); err != nil {
		t.Fatalf("could not create symlink: %v", err)
	}

	if err = copyFile(symlinkPath, dstPath); err != nil {
		t.Fatalf("failed to copy symlink: %s", err)
	}

	resolvedPath, err := os.Readlink(dstPath)
	if err != nil {
		t.Fatalf("could not resolve symlink: %s", err)
	}

	if resolvedPath != srcPath {
		t.Fatalf("resolved path is incorrect. expected %s, got %s", srcPath, resolvedPath)
	}
}

func TestCopyFileFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		// XXX: setting permissions works differently in
		// Microsoft Windows. Skipping this this until a
		// compatible implementation is provided.
		t.Skip("skipping on windows")
	}

	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srcf, err := os.Create(filepath.Join(dir, "srcfile"))
	if err != nil {
		t.Fatal(err)
	}
	srcf.Close()

	var dstdir string

	cleanup := setupInaccessibleDir(t, func(dir string) error {
		dstdir = filepath.Join(dir, "dir")
		return os.Mkdir(dstdir, 0777)
	})
	defer cleanup()

	fn := filepath.Join(dstdir, "file")
	if err := copyFile(srcf.Name(), fn); err == nil {
		t.Fatalf("expected error for %s, got none", fn)
	}
}

// setupInaccessibleDir creates a temporary location with a single
// directory in it, in such a way that that directory is not accessible
// after this function returns.
//
// op is called with the directory as argument, so that it can create
// files or other test artifacts.
//
// If setupInaccessibleDir fails in its preparation, or op fails, t.Fatal
// will be invoked.
//
// This function returns a cleanup function that removes all the temporary
// files this function creates. It is the caller's responsibility to call
// this function before the test is done running, whether there's an error or not.
func setupInaccessibleDir(t *testing.T, op func(dir string) error) func() {
	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
		return nil // keep compiler happy
	}

	subdir := filepath.Join(dir, "dir")

	cleanup := func() {
		if err := os.Chmod(subdir, 0777); err != nil {
			t.Error(err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	}

	if err := os.Mkdir(subdir, 0777); err != nil {
		cleanup()
		t.Fatal(err)
		return nil
	}

	if err := op(subdir); err != nil {
		cleanup()
		t.Fatal(err)
		return nil
	}

	if err := os.Chmod(subdir, 0666); err != nil {
		cleanup()
		t.Fatal(err)
		return nil
	}

	return cleanup
}

func TestIsRegular(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	var fn string

	cleanup := setupInaccessibleDir(t, func(dir string) error {
		fn = filepath.Join(dir, "file")
		fh, err := os.Create(fn)
		if err != nil {
			return err
		}

		return fh.Close()
	})
	defer cleanup()

	tests := map[string]struct {
		exists bool
		err    bool
	}{
		wd: {false, true},
		filepath.Join(wd, "testdata"):                       {false, true},
		filepath.Join(wd, "testdata", "test.file"):          {true, false},
		filepath.Join(wd, "this_file_does_not_exist.thing"): {false, false},
		fn: {false, true},
	}

	if runtime.GOOS == "windows" {
		// This test doesn't work on Microsoft Windows because
		// of the differences in how file permissions are
		// implemented. For this to work, the directory where
		// the file exists should be inaccessible.
		delete(tests, fn)
	}

	for f, want := range tests {
		got, err := IsRegular(f)
		if err != nil {
			if want.exists != got {
				t.Fatalf("expected %t for %s, got %t", want.exists, f, got)
			}
			if !want.err {
				t.Fatalf("expected no error, got %v", err)
			}
		} else {
			if want.err {
				t.Fatalf("expected error for %s, got none", f)
			}
		}

		if got != want.exists {
			t.Fatalf("expected %t for %s, got %t", want, f, got)
		}
	}

}

func TestIsDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	var dn string

	cleanup := setupInaccessibleDir(t, func(dir string) error {
		dn = filepath.Join(dir, "dir")
		return os.Mkdir(dn, 0777)
	})
	defer cleanup()

	tests := map[string]struct {
		exists bool
		err    bool
	}{
		wd: {true, false},
		filepath.Join(wd, "testdata"):                       {true, false},
		filepath.Join(wd, "main.go"):                        {false, true},
		filepath.Join(wd, "this_file_does_not_exist.thing"): {false, true},
		dn: {false, true},
	}

	if runtime.GOOS == "windows" {
		// This test doesn't work on Microsoft Windows because
		// of the differences in how file permissions are
		// implemented. For this to work, the directory where
		// the directory exists should be inaccessible.
		delete(tests, dn)
	}

	for f, want := range tests {
		got, err := IsDir(f)
		if err != nil && !want.err {
			t.Fatalf("expected no error, got %v", err)
		}

		if got != want.exists {
			t.Fatalf("expected %t for %s, got %t", want.exists, f, got)
		}
	}
}

func TestIsNonEmptyDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("empty")

	testCases := []struct {
		path  string
		empty bool
		err   bool
	}{
		{wd, true, false},
		{"testdata", true, false},
		{filepath.Join(wd, "fs.go"), false, true},
		{filepath.Join(wd, "this_file_does_not_exist.thing"), false, false},
		{h.Path("empty"), false, false},
	}

	// This test case doesn't work on Microsoft Windows because of the
	// differences in how file permissions are implemented.
	if runtime.GOOS != "windows" {
		var inaccessibleDir string
		cleanup := setupInaccessibleDir(t, func(dir string) error {
			inaccessibleDir = filepath.Join(dir, "empty")
			return os.Mkdir(inaccessibleDir, 0777)
		})
		defer cleanup()

		testCases = append(testCases, struct {
			path  string
			empty bool
			err   bool
		}{inaccessibleDir, false, true})
	}

	for _, want := range testCases {
		got, err := IsNonEmptyDir(want.path)
		if want.err && err == nil {
			if got {
				t.Fatalf("wanted false with error for %v, but got true", want.path)
			}
			t.Fatalf("wanted an error for %v, but it was nil", want.path)
		}

		if got != want.empty {
			t.Fatalf("wanted %t for %v, but got %t", want.empty, want.path, got)
		}
	}
}

func TestIsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		// XXX: creating symlinks is not supported in Go on
		// Microsoft Windows. Skipping this this until a solution
		// for creating symlinks is is provided.
		t.Skip("skipping on windows")
	}

	dir, err := ioutil.TempDir("", "dep")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	dirPath := filepath.Join(dir, "directory")
	if err = os.MkdirAll(dirPath, 0777); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(dir, "file")
	f, err := os.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	dirSymlink := filepath.Join(dir, "dirSymlink")
	fileSymlink := filepath.Join(dir, "fileSymlink")
	if err = os.Symlink(dirPath, dirSymlink); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filePath, fileSymlink); err != nil {
		t.Fatal(err)
	}

	var (
		inaccessibleFile    string
		inaccessibleSymlink string
	)

	cleanup := setupInaccessibleDir(t, func(dir string) error {
		inaccessibleFile = filepath.Join(dir, "file")
		if fh, err := os.Create(inaccessibleFile); err != nil {
			return err
		} else if err = fh.Close(); err != nil {
			return err
		}

		inaccessibleSymlink = filepath.Join(dir, "symlink")
		return os.Symlink(inaccessibleFile, inaccessibleSymlink)
	})
	defer cleanup()

	tests := map[string]struct {
		expected bool
		err      bool
	}{
		dirPath:             {false, false},
		filePath:            {false, false},
		dirSymlink:          {true, false},
		fileSymlink:         {true, false},
		inaccessibleFile:    {false, true},
		inaccessibleSymlink: {false, true},
	}

	for path, want := range tests {
		got, err := IsSymlink(path)
		if err != nil {
			if !want.err {
				t.Errorf("expected no error, got %v", err)
			}
		}

		if got != want.expected {
			t.Errorf("expected %t for %s, got %t", want.expected, path, got)
		}
	}
}
