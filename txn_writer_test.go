// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/dep/test"
	"github.com/pkg/errors"
)

func TestTxnWriterBadInputs(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("txnwriter")
	root := h.Path(".")

	var sw SafeWriter
	pc, err := NewContext()
	if err != nil {
		t.Fatal(err)
	}
	sm, err := pc.SourceManager()
	defer sm.Release()
	if err != nil {
		t.Fatal(err)
	}

	sw.Prepare(nil, nil, nil, false)

	// no root
	if err := sw.Write("", nil); err == nil {
		t.Fatalf("should have errored without a root path, but did not")
	}

	// noop
	if err := sw.Write(root, nil); err != nil {
		t.Fatalf("write with only root should be fine, just a no-op, but got err %q", err)
	}

	// force vendor with missing source manager
	sw.Prepare(nil, nil, nil, true)
	if !sw.Payload.HasVendor() {
		t.Fatalf("writeV not propogated")
	}
	if err := sw.Write(root, nil); err == nil {
		t.Fatalf("should fail because no source manager was provided for writing vendor")
	}

	// force vendor with missing lock
	if err := sw.Write(root, sm); err == nil {
		t.Fatalf("should fail because no lock was provided from which to write vendor")
	}

	// now check dir validation
	sw.Prepare(nil, nil, nil, false)
	missingroot := filepath.Join(root, "nonexistent")
	if err := sw.Write(missingroot, sm); err == nil {
		t.Fatalf("should have errored with nonexistent dir for root path, but did not")
	}

	fileroot := filepath.Join(root, "myfile")
	srcf, err := os.Create(fileroot)
	if err != nil {
		t.Fatal(err)
	}
	srcf.Close()
	if err := sw.Write(fileroot, sm); err == nil {
		t.Fatalf("should have errored when root path is a file, but did not")
	}
}

const safeWriterProject = "safewritertest"
const safeWriterGoldenManifest = "txn_writer/expected_manifest.json"
const safeWriterGoldenLock = "txn_writer/expected_lock.json"

func TestSafeWriter_ManifestOnly(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()
	pc.CopyFile(ManifestName, safeWriterGoldenManifest)
	pc.Load()

	var sw SafeWriter
	sw.Prepare(pc.Project.Manifest, nil, nil, false)

	// Verify prepared actions
	if !sw.Payload.HasManifest() {
		t.Fatal("Expected the payload to contain the manifest")
	}
	if sw.Payload.HasLock() {
		t.Fatal("Did not expect the payload to contain the lock")
	}
	if sw.Payload.HasVendor() {
		t.Fatal("Did not expect the payload to contain the vendor directory")
	}

	// Write changes
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager)
	h.Must(errors.Wrap(err, "SafeWriter.Write failed"))

	// Verify file system changes
	if err := pc.ManifestShouldMatchGolden(safeWriterGoldenManifest); err != nil {
		t.Fatal(err)
	}
	if err := pc.LockShouldNotExist(); err != nil {
		t.Fatal(err)
	}
	if err := pc.VendorShouldNotExist(); err != nil {
		t.Fatal(err)
	}
}

func TestSafeWriter_ManifestAndUnmodifiedLock(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()
	pc.CopyFile(ManifestName, safeWriterGoldenManifest)
	pc.CopyFile(LockName, safeWriterGoldenLock)
	pc.Load()

	var sw SafeWriter
	sw.Prepare(pc.Project.Manifest, pc.Project.Lock, nil, false)

	// Verify prepared actions
	if !sw.Payload.HasManifest() {
		t.Fatal("Expected the payload to contain the manifest")
	}
	if !sw.Payload.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if sw.Payload.HasVendor() {
		t.Fatal("Did not expect the payload to contain the vendor directory")
	}

	// Write changes
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager)
	h.Must(errors.Wrap(err, "SafeWriter.Write failed"))

	// Verify file system changes
	if err := pc.ManifestShouldMatchGolden(safeWriterGoldenManifest); err != nil {
		t.Fatal(err)
	}
	if err := pc.LockShouldMatchGolden(safeWriterGoldenLock); err != nil {
		t.Fatal(err)
	}
	if err := pc.VendorShouldNotExist(); err != nil {
		t.Fatal(err)
	}
}

func TestSafeWriter_ManifestAndUnmodifiedLockWithForceVendor(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()
	pc.CopyFile(ManifestName, safeWriterGoldenManifest)
	pc.CopyFile(LockName, safeWriterGoldenLock)
	pc.Load()

	var sw SafeWriter
	sw.Prepare(pc.Project.Manifest, pc.Project.Lock, nil, true)

	// Verify prepared actions
	if !sw.Payload.HasManifest() {
		t.Fatal("Expected the payload to contain the manifest")
	}
	if !sw.Payload.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.Payload.HasVendor() {
		t.Fatal("Expected the payload to the vendor directory")
	}

	// Write changes
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager)
	h.Must(errors.Wrap(err, "SafeWriter.Write failed"))

	// Verify file system changes
	if err := pc.ManifestShouldMatchGolden(safeWriterGoldenManifest); err != nil {
		t.Fatal(err)
	}
	if err := pc.LockShouldMatchGolden(safeWriterGoldenLock); err != nil {
		t.Fatal(err)
	}
	if err := pc.VendorShouldExist(); err != nil {
		t.Fatal(err)
	}
}

func TestSafeWriter_UnmodifiedLock(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()
	pc.CopyFile(LockName, safeWriterGoldenLock)
	pc.Load()

	var sw SafeWriter
	sw.Prepare(nil, pc.Project.Lock, pc.Project.Lock, false)

	// Verify prepared actions
	if sw.Payload.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
	}
	if sw.Payload.HasLock() {
		t.Fatal("Did not expect the payload to contain the lock")
	}
	if sw.Payload.HasVendor() {
		t.Fatal("Did not expect the payload to contain the vendor directory")
	}

	// Write changes
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager)
	h.Must(errors.Wrap(err, "SafeWriter.Write failed"))

	// Verify file system changes
	// locks are equivalent, so nothing gets written
	if err := pc.ManifestShouldNotExist(); err != nil {
		t.Fatal(err)
	}
	if err := pc.VendorShouldNotExist(); err != nil {
		t.Fatal(err)
	}
}

func TestSafeWriter_ModifiedLockForceVendor(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()
	pc.CopyFile(LockName, safeWriterGoldenLock)
	pc.Load()

	var sw SafeWriter
	originalLock := new(Lock)
	*originalLock = *pc.Project.Lock
	originalLock.Memo = []byte{} // zero out the input hash to ensure non-equivalency
	sw.Prepare(nil, originalLock, pc.Project.Lock, true)

	// Verify prepared actions
	if sw.Payload.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
	}
	if !sw.Payload.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.Payload.HasVendor() {
		t.Fatal("Expected the payload to the vendor directory")
	}

	// Write changes
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager)
	h.Must(errors.Wrap(err, "SafeWriter.Write failed"))

	// Verify file system changes
	if err := pc.ManifestShouldNotExist(); err != nil {
		t.Fatal(err)
	}
	if err := pc.LockShouldMatchGolden(safeWriterGoldenLock); err != nil {
		t.Fatal(err)
	}
	if err := pc.VendorShouldExist(); err != nil {
		t.Fatal(err)
	}
	if err := pc.VendorFileShouldExist("github.com/sdboyer/dep-test"); err != nil {
		t.Fatal(err)
	}
}

func TestSafeWriter_ForceVendorWhenVendorAlreadyExists(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()
	pc.CopyFile(LockName, safeWriterGoldenLock)
	pc.Load()

	var sw SafeWriter
	// Populate vendor
	sw.Prepare(nil, pc.Project.Lock, nil, true)
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager)
	h.Must(errors.Wrap(err, "SafeWriter.Write failed"))

	// Verify prepared actions
	sw.Prepare(nil, nil, pc.Project.Lock, true)
	if sw.Payload.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
	}
	if !sw.Payload.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.Payload.HasVendor() {
		t.Fatal("Expected the payload to the vendor directory")
	}

	err = sw.Write(pc.Project.AbsRoot, pc.SourceManager)
	h.Must(errors.Wrap(err, "SafeWriter.Write failed"))

	// Verify file system changes
	if err := pc.ManifestShouldNotExist(); err != nil {
		t.Fatal(err)
	}
	if err := pc.LockShouldMatchGolden(safeWriterGoldenLock); err != nil {
		t.Fatal(err)
	}
	if err := pc.VendorShouldExist(); err != nil {
		t.Fatal(err)
	}
	if err := pc.VendorFileShouldExist("github.com/sdboyer/dep-test"); err != nil {
		t.Fatal(err)
	}
}
