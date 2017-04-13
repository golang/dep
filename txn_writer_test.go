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
const safeWriterGoldenManifest = "txn_writer/expected_manifest.toml"
const safeWriterGoldenLock = "txn_writer/expected_lock.toml"

func TestSafeWriter_BadInput_MissingRoot(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()

	var sw SafeWriter
	sw.Prepare(nil, nil, nil, VendorOnChanged)

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
	pc.CopyFile(LockName, safeWriterGoldenLock)
	pc.Load()

	var sw SafeWriter
	sw.Prepare(nil, nil, pc.Project.Lock, VendorAlways)

	err := sw.Write(pc.Project.AbsRoot, nil)

	if err == nil {
		t.Fatal("should have errored without a source manager when forceVendor is true, but did not")
	} else if !strings.Contains(err.Error(), "SourceManager") {
		t.Fatalf("expected SourceManager error, got %s", err.Error())
	}
}

func TestSafeWriter_BadInput_ForceVendorMissingLock(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()

	var sw SafeWriter
	err := sw.Prepare(nil, nil, nil, VendorAlways)

	if err == nil {
		t.Fatal("should have errored without a lock when forceVendor is true, but did not")
	} else if !strings.Contains(err.Error(), "newLock") {
		t.Fatalf("expected newLock error, got %s", err.Error())
	}
}

func TestSafeWriter_BadInput_OldLockOnly(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()
	pc.CopyFile(LockName, safeWriterGoldenLock)
	pc.Load()

	var sw SafeWriter
	err := sw.Prepare(nil, pc.Project.Lock, nil, VendorAlways)

	if err == nil {
		t.Fatal("should have errored with only an old lock, but did not")
	} else if !strings.Contains(err.Error(), "oldLock") {
		t.Fatalf("expected oldLock error, got %s", err.Error())
	}
}

func TestSafeWriter_BadInput_NonexistentRoot(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()

	var sw SafeWriter
	sw.Prepare(nil, nil, nil, VendorOnChanged)

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
	sw.Prepare(nil, nil, nil, VendorOnChanged)

	fileroot := pc.CopyFile("fileroot", "txn_writer/badinput_fileroot")
	err := sw.Write(fileroot, pc.SourceManager)

	if err == nil {
		t.Fatal("should have errored when root path is a file, but did not")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected does not exist error, got %s", err.Error())
	}
}

func TestSafeWriter_Manifest(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()
	pc.CopyFile(ManifestName, safeWriterGoldenManifest)
	pc.Load()

	var sw SafeWriter
	sw.Prepare(pc.Project.Manifest, nil, nil, VendorOnChanged)

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
	sw.Prepare(pc.Project.Manifest, pc.Project.Lock, pc.Project.Lock, VendorOnChanged)

	// Verify prepared actions
	if !sw.Payload.HasManifest() {
		t.Fatal("Expected the payload to contain the manifest")
	}
	if !sw.Payload.HasLock() {
		t.Fatal("Expected the payload to contain the lock.")
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
	sw.Prepare(pc.Project.Manifest, pc.Project.Lock, pc.Project.Lock, VendorAlways)

	// Verify prepared actions
	if !sw.Payload.HasManifest() {
		t.Fatal("Expected the payload to contain the manifest")
	}
	if !sw.Payload.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.Payload.HasVendor() {
		t.Fatal("Expected the payload to contain the vendor directory")
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
	if err := pc.VendorFileShouldExist("github.com/sdboyer/dep-test"); err != nil {
		t.Fatal(err)
	}
}

func TestSafeWriter_ModifiedLock(t *testing.T) {
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
	sw.Prepare(nil, originalLock, pc.Project.Lock, VendorOnChanged)

	// Verify prepared actions
	if sw.Payload.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
	}
	if !sw.Payload.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.Payload.HasVendor() {
		t.Fatal("Expected the payload to contain the vendor directory")
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

func TestSafeWriter_ModifiedLockSkipVendor(t *testing.T) {
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
	sw.Prepare(nil, originalLock, pc.Project.Lock, VendorNever)

	// Verify prepared actions
	if sw.Payload.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
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
	if err := pc.ManifestShouldNotExist(); err != nil {
		t.Fatal(err)
	}
	if err := pc.LockShouldMatchGolden(safeWriterGoldenLock); err != nil {
		t.Fatal(err)
	}
	if err := pc.VendorShouldNotExist(); err != nil {
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
	sw.Prepare(nil, pc.Project.Lock, pc.Project.Lock, VendorAlways)
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager)
	h.Must(errors.Wrap(err, "SafeWriter.Write failed"))

	// Verify prepared actions
	sw.Prepare(nil, nil, pc.Project.Lock, VendorAlways)
	if sw.Payload.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
	}
	if !sw.Payload.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.Payload.HasVendor() {
		t.Fatal("Expected the payload to contain the vendor directory ")
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

func TestSafeWriter_NewLock(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()
	pc.Load()

	var sw SafeWriter
	lf := h.GetTestFile(safeWriterGoldenLock)
	defer lf.Close()
	newLock, err := readLock(lf)
	h.Must(err)
	sw.Prepare(nil, nil, newLock, VendorOnChanged)

	// Verify prepared actions
	if sw.Payload.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
	}
	if !sw.Payload.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.Payload.HasVendor() {
		t.Fatal("Expected the payload to contain the vendor directory")
	}

	// Write changes
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
}

func TestSafeWriter_NewLockSkipVendor(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()
	pc.Load()

	var sw SafeWriter
	lf := h.GetTestFile(safeWriterGoldenLock)
	defer lf.Close()
	newLock, err := readLock(lf)
	h.Must(err)
	sw.Prepare(nil, nil, newLock, VendorNever)

	// Verify prepared actions
	if sw.Payload.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
	}
	if !sw.Payload.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if sw.Payload.HasVendor() {
		t.Fatal("Did not expect the payload to contain the vendor directory")
	}

	// Write changes
	err = sw.Write(pc.Project.AbsRoot, pc.SourceManager)
	h.Must(errors.Wrap(err, "SafeWriter.Write failed"))

	// Verify file system changes
	if err := pc.ManifestShouldNotExist(); err != nil {
		t.Fatal(err)
	}
	if err := pc.LockShouldMatchGolden(safeWriterGoldenLock); err != nil {
		t.Fatal(err)
	}
	if err := pc.VendorShouldNotExist(); err != nil {
		t.Fatal(err)
	}
}

func TestSafeWriter_DiffLocks(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()
	pc.CopyFile(LockName, "txn_writer/original_lock.toml")
	pc.Load()

	ulf := h.GetTestFile("txn_writer/updated_lock.toml")
	defer ulf.Close()
	updatedLock, err := readLock(ulf)
	h.Must(err)

	var sw SafeWriter
	sw.Prepare(nil, pc.Project.Lock, updatedLock, VendorOnChanged)

	// Verify lock diff
	diff := sw.Payload.LockDiff
	if diff == nil {
		t.Fatal("Expected the payload to contain a diff of the lock files")
	}
	if diff.HashDiff == nil {
		t.Fatalf("Expected the lock diff to contain the updated hash: expected %s, got %s", pc.Project.Lock.Memo, updatedLock.Memo)
	}

	if len(diff.Add) != 2 {
		t.Fatalf("Expected the lock diff to contain 2 added projects, got %d", len(diff.Add))
	} else {
		add1 := diff.Add[0]
		if add1.Name != "github.com/sdboyer/deptest" {
			t.Errorf("expected new project[0] github.com/sdboyer/deptest, got %s", add1.Name)
		}
		add2 := diff.Add[1]
		if add2.Name != "github.com/stuff/realthing" {
			t.Errorf("expected new project[1] github.com/stuff/realthing, got %s", add2.Name)
		}
	}

	if len(diff.Remove) != 1 {
		t.Fatalf("Expected the lock diff to contain 1 removed project, got %d", len(diff.Remove))
	} else {
		remove := diff.Remove[0]
		if remove.Name != "github.com/stuff/placeholder" {
			t.Fatalf("expected new project github.com/stuff/placeholder, got %s", remove.Name)
		}
	}

	if len(diff.Modify) != 1 {
		t.Fatalf("Expected the lock diff to contain 1 modified project, got %d", len(diff.Modify))
	} else {
		modify := diff.Modify[0]
		if modify.Name != "github.com/foo/bar" {
			t.Fatalf("expected new project github.com/foo/bar, got %s", modify.Name)
		}
	}

	output, err := diff.Format()
	h.Must(err)
	goldenOutput := "txn_writer/expected_diff_output.txt"
	if err = pc.ShouldMatchGolden(goldenOutput, output); err != nil {
		t.Fatal(err)
	}
}
