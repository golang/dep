// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/dep/internal/test"
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

	sw, _ := NewSafeWriter(nil, nil, nil, VendorOnChanged)
	err := sw.Write("", pc.SourceManager, true, discardLogger)

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

	sw, _ := NewSafeWriter(nil, nil, pc.Project.Lock, VendorAlways)
	err := sw.Write(pc.Project.AbsRoot, nil, true, discardLogger)

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

	_, err := NewSafeWriter(nil, nil, nil, VendorAlways)
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

	_, err := NewSafeWriter(nil, pc.Project.Lock, nil, VendorAlways)
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

	sw, _ := NewSafeWriter(nil, nil, nil, VendorOnChanged)

	missingroot := filepath.Join(pc.Project.AbsRoot, "nonexistent")
	err := sw.Write(missingroot, pc.SourceManager, true, discardLogger)

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

	sw, _ := NewSafeWriter(nil, nil, nil, VendorOnChanged)

	fileroot := pc.CopyFile("fileroot", "txn_writer/badinput_fileroot")
	err := sw.Write(fileroot, pc.SourceManager, true, discardLogger)

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

	sw, _ := NewSafeWriter(pc.Project.Manifest, nil, nil, VendorOnChanged)

	// Verify prepared actions
	if !sw.HasManifest() {
		t.Fatal("Expected the payload to contain the manifest")
	}
	if sw.HasLock() {
		t.Fatal("Did not expect the payload to contain the lock")
	}
	if sw.writeVendor {
		t.Fatal("Did not expect the payload to contain the vendor directory")
	}

	// Write changes
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager, true, discardLogger)
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

	sw, _ := NewSafeWriter(pc.Project.Manifest, pc.Project.Lock, pc.Project.Lock, VendorOnChanged)

	// Verify prepared actions
	if !sw.HasManifest() {
		t.Fatal("Expected the payload to contain the manifest")
	}
	if !sw.HasLock() {
		t.Fatal("Expected the payload to contain the lock.")
	}
	if sw.writeLock {
		t.Fatal("Did not expect that the writer should plan to write the lock")
	}
	if sw.writeVendor {
		t.Fatal("Did not expect the payload to contain the vendor directory")
	}

	// Write changes
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager, true, discardLogger)
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

	sw, _ := NewSafeWriter(pc.Project.Manifest, pc.Project.Lock, pc.Project.Lock, VendorAlways)

	// Verify prepared actions
	if !sw.HasManifest() {
		t.Fatal("Expected the payload to contain the manifest")
	}
	if !sw.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if sw.writeLock {
		t.Fatal("Did not expect that the writer should plan to write the lock")
	}
	if !sw.writeVendor {
		t.Fatal("Expected the payload to contain the vendor directory")
	}

	// Write changes
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager, true, discardLogger)
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

	originalLock := new(Lock)
	*originalLock = *pc.Project.Lock
	originalLock.SolveMeta.InputsDigest = []byte{} // zero out the input hash to ensure non-equivalency
	sw, _ := NewSafeWriter(nil, originalLock, pc.Project.Lock, VendorOnChanged)

	// Verify prepared actions
	if sw.HasManifest() {
		t.Fatal("Did not expect the manifest to be written")
	}
	if !sw.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.writeLock {
		t.Fatal("Expected that the writer should plan to write the lock")
	}
	if !sw.writeVendor {
		t.Fatal("Expected that the writer should plan to write the vendor directory")
	}

	// Write changes
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager, true, discardLogger)
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

	originalLock := new(Lock)
	*originalLock = *pc.Project.Lock
	originalLock.SolveMeta.InputsDigest = []byte{} // zero out the input hash to ensure non-equivalency
	sw, _ := NewSafeWriter(nil, originalLock, pc.Project.Lock, VendorNever)

	// Verify prepared actions
	if sw.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
	}
	if !sw.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.writeLock {
		t.Fatal("Expected that the writer should plan to write the lock")
	}
	if sw.writeVendor {
		t.Fatal("Did not expect the payload to contain the vendor directory")
	}

	// Write changes
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager, true, discardLogger)
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

	sw, _ := NewSafeWriter(nil, pc.Project.Lock, pc.Project.Lock, VendorAlways)
	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager, true, discardLogger)
	h.Must(errors.Wrap(err, "SafeWriter.Write failed"))

	// Verify prepared actions
	sw, _ = NewSafeWriter(nil, nil, pc.Project.Lock, VendorAlways)
	if sw.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
	}
	if !sw.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.writeLock {
		t.Fatal("Expected that the writer should plan to write the lock")
	}
	if !sw.writeVendor {
		t.Fatal("Expected the payload to contain the vendor directory ")
	}

	err = sw.Write(pc.Project.AbsRoot, pc.SourceManager, true, discardLogger)
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

	lf := h.GetTestFile(safeWriterGoldenLock)
	defer lf.Close()
	newLock, err := readLock(lf)
	h.Must(err)
	sw, _ := NewSafeWriter(nil, nil, newLock, VendorOnChanged)

	// Verify prepared actions
	if sw.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
	}
	if !sw.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.writeLock {
		t.Fatal("Expected that the writer should plan to write the lock")
	}
	if !sw.writeVendor {
		t.Fatal("Expected the payload to contain the vendor directory")
	}

	// Write changes
	err = sw.Write(pc.Project.AbsRoot, pc.SourceManager, true, discardLogger)
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

	lf := h.GetTestFile(safeWriterGoldenLock)
	defer lf.Close()
	newLock, err := readLock(lf)
	h.Must(err)
	sw, _ := NewSafeWriter(nil, nil, newLock, VendorNever)

	// Verify prepared actions
	if sw.HasManifest() {
		t.Fatal("Did not expect the payload to contain the manifest")
	}
	if !sw.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if !sw.writeLock {
		t.Fatal("Expected that the writer should plan to write the lock")
	}
	if sw.writeVendor {
		t.Fatal("Did not expect the payload to contain the vendor directory")
	}

	// Write changes
	err = sw.Write(pc.Project.AbsRoot, pc.SourceManager, true, discardLogger)
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

	sw, _ := NewSafeWriter(nil, pc.Project.Lock, updatedLock, VendorOnChanged)

	// Verify lock diff
	diff := sw.lockDiff
	if diff == nil {
		t.Fatal("Expected the payload to contain a diff of the lock files")
	}

	output, err := formatLockDiff(*diff)
	h.Must(err)
	goldenOutput := "txn_writer/expected_diff_output.txt"
	if err = pc.ShouldMatchGolden(goldenOutput, output); err != nil {
		t.Fatal(err)
	}
}

func TestHasDotGit(t *testing.T) {
	// Create a tempdir with .git file
	td, err := ioutil.TempDir(os.TempDir(), "dotGitFile")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(td)

	os.OpenFile(td+string(filepath.Separator)+".git", os.O_CREATE, 0777)
	if !hasDotGit(td) {
		t.Fatal("Expected hasDotGit to find .git")
	}
}

func TestSafeWriter_VendorDotGitPreservedWithForceVendor(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	pc := NewTestProjectContext(h, safeWriterProject)
	defer pc.Release()

	gitDirPath := filepath.Join(pc.Project.AbsRoot, "vendor", ".git")
	os.MkdirAll(gitDirPath, 0777)
	dummyFile := filepath.Join("vendor", ".git", "badinput_fileroot")
	pc.CopyFile(dummyFile, "txn_writer/badinput_fileroot")
	pc.CopyFile(ManifestName, safeWriterGoldenManifest)
	pc.CopyFile(LockName, safeWriterGoldenLock)
	pc.Load()

	sw, _ := NewSafeWriter(pc.Project.Manifest, pc.Project.Lock, pc.Project.Lock, VendorAlways)

	// Verify prepared actions
	if !sw.HasManifest() {
		t.Fatal("Expected the payload to contain the manifest")
	}
	if !sw.HasLock() {
		t.Fatal("Expected the payload to contain the lock")
	}
	if sw.writeLock {
		t.Fatal("Did not expect that the writer should plan to write the lock")
	}
	if !sw.writeVendor {
		t.Fatal("Expected the payload to contain the vendor directory")
	}

	err := sw.Write(pc.Project.AbsRoot, pc.SourceManager, true, discardLogger)
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
	if err := pc.VendorFileShouldExist(".git/badinput_fileroot"); err != nil {
		t.Fatal(err)
	}
}
