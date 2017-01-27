// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/dep/test"
)

func TestTxnWriterBadInputs(t *testing.T) {
	tg := test.Testgo(t)
	defer tg.Cleanup()

	tg.TempDir("txnwriter")
	td := tg.Path(".")

	var sw SafeWriter

	// no root
	if err := sw.WriteAllSafe(false); err == nil {
		t.Errorf("should have errored without a root path, but did not")
	}
	sw.Root = td

	if err := sw.WriteAllSafe(false); err != nil {
		t.Errorf("write with only root should be fine, just a no-op, but got err %q", err)
	}
	if err := sw.WriteAllSafe(true); err == nil {
		t.Errorf("should fail because no source manager was provided for writing vendor")
	}

	if err := sw.WriteAllSafe(true); err == nil {
		t.Errorf("should fail because no lock was provided from which to write vendor")
	}
	// now check dir validation
	sw.Root = filepath.Join(td, "nonexistent")
	if err := sw.WriteAllSafe(false); err == nil {
		t.Errorf("should have errored with nonexistent dir for root path, but did not")
	}

	sw.Root = filepath.Join(td, "myfile")
	srcf, err := os.Create(sw.Root)
	if err != nil {
		t.Fatal(err)
	}
	srcf.Close()
	if err := sw.WriteAllSafe(false); err == nil {
		t.Errorf("should have errored when root path is a file, but did not")
	}
}

func TestTxnWriter(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	tg := test.Testgo(t)
	tg.TempDir("")
	defer tg.Cleanup()

	c := &Ctx{
		GOPATH: tg.Path("."),
	}
	sm, err := c.SourceManager()
	defer sm.Release()
	tg.Must(err)

	var sw SafeWriter
	var mpath, lpath, vpath string
	var count int
	reset := func() {
		pr := filepath.Join("src", "txnwriter"+strconv.Itoa(count))
		tg.TempDir(pr)

		sw = SafeWriter{
			Root:          tg.Path(pr),
			SourceManager: sm,
		}
		tg.Cd(sw.Root)

		mpath = filepath.Join(sw.Root, ManifestName)
		lpath = filepath.Join(sw.Root, LockName)
		vpath = filepath.Join(sw.Root, "vendor")

		count++
	}
	reset()

	// super basic manifest and lock
	expectedManifest := `{
    "dependencies": {
        "github.com/sdboyer/dep-test": {
            "version": "1.0.0"
        }
    }
}
`
	expectedLock := `{
    "memo": "595716d270828e763c811ef79c9c41f85b1d1bfbdfe85280036405c03772206c",
    "projects": [
        {
            "name": "github.com/sdboyer/dep-test",
            "version": "1.0.0",
            "revision": "2a3a211e171803acb82d1d5d42ceb53228f51751",
            "packages": [
                "."
            ]
        }
    ]
}
`

	m, err := readManifest(strings.NewReader(expectedManifest))
	tg.Must(err)
	l, err := readLock(strings.NewReader(expectedLock))
	tg.Must(err)

	// Just write manifest
	sw.Manifest = m
	tg.Must(sw.WriteAllSafe(false))
	tg.MustExist(mpath)
	tg.MustNotExist(lpath)
	tg.MustNotExist(vpath)

	diskm := tg.ReadManifest()
	if diskm != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, diskm)
	}

	// Manifest and lock, but no vendor
	sw.Lock = l
	tg.Must(sw.WriteAllSafe(false))
	tg.MustExist(mpath)
	tg.MustExist(lpath)
	tg.MustNotExist(vpath)

	diskm = tg.ReadManifest()
	if diskm != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, diskm)
	}

	diskl := tg.ReadLock()
	if diskl != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, diskl)
	}

	tg.Must(sw.WriteAllSafe(true))
	tg.MustExist(mpath)
	tg.MustExist(lpath)
	tg.MustExist(vpath)
	tg.MustExist(filepath.Join(vpath, "github.com", "sdboyer", "dep-test"))

	diskm = tg.ReadManifest()
	if diskm != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, diskm)
	}

	diskl = tg.ReadLock()
	if diskl != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, diskl)
	}

	// start fresh, ignoring the manifest now
	reset()
	sw.Lock = l
	sw.NewLock = l

	tg.Must(sw.WriteAllSafe(false))
	// locks are equivalent, so nothing gets written
	tg.MustNotExist(mpath)
	tg.MustNotExist(lpath)
	tg.MustNotExist(vpath)

	l2 := new(Lock)
	*l2 = *l
	// zero out the input hash to ensure non-equivalency
	l2.Memo = []byte{}
	sw.Lock = l2
	tg.Must(sw.WriteAllSafe(true))
	tg.MustNotExist(mpath)
	tg.MustExist(lpath)
	tg.MustExist(vpath)
	tg.MustExist(filepath.Join(vpath, "github.com", "sdboyer", "dep-test"))

	diskl = tg.ReadLock()
	if diskl != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, diskl)
	}

	// repeat op to ensure good behavior when vendor dir already exists
	sw.Lock = nil
	tg.Must(sw.WriteAllSafe(true))
	tg.MustNotExist(mpath)
	tg.MustExist(lpath)
	tg.MustExist(vpath)
	tg.MustExist(filepath.Join(vpath, "github.com", "sdboyer", "dep-test"))

	diskl = tg.ReadLock()
	if diskl != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, diskl)
	}

	// TODO test txn rollback cases. maybe we can force errors with chmodding?
}
