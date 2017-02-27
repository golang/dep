// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/dep/test"
	"github.com/pkg/errors"
)

const safeWriterProject = "safewritertest"
const safeWriterGoldenManifest = "txn_writer/expected_manifest.json"
const safeWriterGoldenLock = "txn_writer/expected_lock.json"

func TestSafeWriter_BadInput_MissingRoot(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()

	var sw SafeWriter
	sw.Prepare(nil, nil, nil, false)

	err := sw.Write("", pc.SourceManager)

	if err == nil {
		t.Fatal("should have errored without a root path, but did not")
	} else if !strings.Contains(err.Error(), "root path") {
		t.Fatalf("expected root path error, got %s", err.Error())
	}
}

func TestSafeWriter_BadInput_MissingSourceManager(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()

	var sw SafeWriter
	sw.Prepare(nil, nil, nil, true)

	err := sw.Write(pc.Project.AbsRoot, nil)

	if err == nil {
		t.Fatal("should have errored without a source manager when forceVendor is true, but did not")
	} else if !strings.Contains(err.Error(), "SourceManager") {
		t.Fatalf("expected SourceManager error, got %s", err.Error())
	}
}

func TestSafeWriter_BadInput_MissingLock(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()

	var sw SafeWriter
	sw.Prepare(nil, nil, nil, true)

	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager)

	if err == nil {
		t.Fatal("should have errored without a lock when forceVendor is true, but did not")
	} else if !strings.Contains(err.Error(), "lock") {
		t.Fatalf("expected lock error, got %s", err.Error())
	}
}

func TestSafeWriter_BadInput_NonexistentRoot(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()

	var sw SafeWriter
	sw.Prepare(nil, nil, nil, true)

	missingroot := filepath.Join(pc.Project.AbsRoot, "nonexistent")
	err := sw.Write(missingroot, pc.SourceManager)

	if err == nil {
		t.Fatal("should have errored with nonexistent dir for root path, but did not")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected does not exist error, got %s", err.Error())
	}
}

func TestSafeWriter_BadInput_RootIsFile(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()

	var sw SafeWriter
	sw.Prepare(nil, nil, nil, true)

	fileroot := pc.CopyFile("fileroot", "txn_writer/badinput_fileroot")
	err := sw.Write(fileroot, pc.SourceManager)

	if err == nil {
		t.Fatal("should have errored when root path is a file, but did not")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected does not exist error, got %s", err.Error())
	}
}

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
