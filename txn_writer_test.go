// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestTxnWriterBadInputs(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("txnwriter")
	td := tg.path(".")

	sw := safeWriter{}

	// no root
	if err := sw.writeAllSafe(false); err == nil {
		t.Errorf("should have errored without a root path, but did not")
	}
	sw.root = td

	if err := sw.writeAllSafe(false); err != nil {
		t.Errorf("write with only root should be fine, just a no-op, but got err %q", err)
	}
	if err := sw.writeAllSafe(true); err == nil {
		t.Errorf("should fail because no source manager was provided for writing vendor")
	}

	if err := sw.writeAllSafe(true); err == nil {
		t.Errorf("should fail because no lock was provided from which to write vendor")
	}
	// now check dir validation
	sw.root = filepath.Join(td, "nonexistent")
	if err := sw.writeAllSafe(false); err == nil {
		t.Errorf("should have errored with nonexistent dir for root path, but did not")
	}

	sw.root = filepath.Join(td, "myfile")
	srcf, err := os.Create(sw.root)
	if err != nil {
		t.Fatal(err)
	}
	srcf.Close()
	if err := sw.writeAllSafe(false); err == nil {
		t.Errorf("should have errored when root path is a file, but did not")
	}
}

func TestTxnWriter(t *testing.T) {
	needsExternalNetwork(t)
	needsGit(t)

	tg := testgo(t)
	tg.tempDir("")
	defer tg.cleanup()

	var c *ctx
	c = &ctx{
		GOPATH: tg.path("."),
	}
	sm, err := c.sourceManager()
	defer sm.Release()
	tg.must(err)

	var sw safeWriter
	var mpath, lpath, vpath string
	var count int
	reset := func() {
		pr := filepath.Join("src", "txnwriter"+strconv.Itoa(count))
		tg.tempDir(pr)

		sw = safeWriter{
			root: tg.path(pr),
			sm:   sm,
		}
		tg.cd(sw.root)

		mpath = filepath.Join(sw.root, manifestName)
		lpath = filepath.Join(sw.root, lockName)
		vpath = filepath.Join(sw.root, vendorName)

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
	tg.must(err)
	l, err := readLock(strings.NewReader(expectedLock))
	tg.must(err)

	// Just write manifest
	sw.m = m
	tg.must(sw.writeAllSafe(false))
	tg.mustExist(mpath)
	tg.mustNotExist(lpath)
	tg.mustNotExist(vpath)

	diskm := tg.readManifest()
	if diskm != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, diskm)
	}

	// Manifest and lock, but no vendor
	sw.l = l
	tg.must(sw.writeAllSafe(false))
	tg.mustExist(mpath)
	tg.mustExist(lpath)
	tg.mustNotExist(vpath)

	diskm = tg.readManifest()
	if diskm != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, diskm)
	}

	diskl := tg.readLock()
	if diskl != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, diskl)
	}

	tg.must(sw.writeAllSafe(true))
	tg.mustExist(mpath)
	tg.mustExist(lpath)
	tg.mustExist(vpath)
	tg.mustExist(filepath.Join(vpath, "github.com", "sdboyer", "dep-test"))

	diskm = tg.readManifest()
	if diskm != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, diskm)
	}

	diskl = tg.readLock()
	if diskl != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, diskl)
	}

	// start fresh, ignoring the manifest now
	reset()
	sw.l = l
	sw.nl = l

	tg.must(sw.writeAllSafe(false))
	// locks are equivalent, so nothing gets written
	tg.mustNotExist(mpath)
	tg.mustNotExist(lpath)
	tg.mustNotExist(vpath)

	l2 := &lock{}
	*l2 = *l
	// zero out the input hash to ensure non-equivalency
	l2.Memo = []byte{}
	sw.l = l2
	tg.must(sw.writeAllSafe(true))
	tg.mustNotExist(mpath)
	tg.mustExist(lpath)
	tg.mustExist(vpath)
	tg.mustExist(filepath.Join(vpath, "github.com", "sdboyer", "dep-test"))

	diskl = tg.readLock()
	if diskl != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, diskl)
	}

	// repeat op to ensure good behavior when vendor dir already exists
	sw.l = nil
	tg.must(sw.writeAllSafe(true))
	tg.mustNotExist(mpath)
	tg.mustExist(lpath)
	tg.mustExist(vpath)
	tg.mustExist(filepath.Join(vpath, "github.com", "sdboyer", "dep-test"))

	diskl = tg.readLock()
	if diskl != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, diskl)
	}

	// TODO test txn rollback cases. maybe we can force errors with chmodding?
}
