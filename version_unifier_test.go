package gps

import (
	"testing"

	"github.com/sdboyer/gps/pkgtree"
)

type lvFixBridge []Version

var lvfb1 lvFixBridge

func init() {
	rev1 := Revision("revision-one")
	rev2 := Revision("revision-two")
	rev3 := Revision("revision-three")

	lvfb1 = lvFixBridge{
		NewBranch("master").Is(rev1),
		NewBranch("test").Is(rev2),
		NewVersion("1.0.0").Is(rev1),
		NewVersion("1.0.1").Is("other1"),
		NewVersion("v2.0.5").Is(rev3),
		NewVersion("2.0.5.2").Is(rev3),
		newDefaultBranch("unwrapped").Is(rev3),
		NewVersion("20.0.5.2").Is(rev1),
		NewVersion("v1.5.5-beta.4").Is("other2"),
		NewVersion("v3.0.1-alpha.1").Is(rev2),
	}
}

func (lb lvFixBridge) listVersions(ProjectIdentifier) ([]Version, error) {
	return lb, nil
}

func TestCreateTyepUnion(t *testing.T) {
	vu := versionUnifier{
		b:   lvfb1,
		mtr: newMetrics(),
	}

	rev1 := Revision("revision-one")
	rev2 := Revision("revision-two")
	id := mkPI("irrelevant")

	vtu := vu.createTypeUnion(id, rev1)
	if len(vtu) != 4 {
		t.Fatalf("wanted a type union with four elements, got %v: \n%#v", len(vtu), vtu)
	}

	vtu = vu.createTypeUnion(id, NewBranch("master"))
	if len(vtu) != 4 {
		t.Fatalf("wanted a type union with four elements, got %v: \n%#v", len(vtu), vtu)
	}

	vtu = vu.createTypeUnion(id, Revision("notexist"))
	if len(vtu) != 1 {
		t.Fatalf("wanted a type union with one elements, got %v: \n%#v", len(vtu), vtu)
	}

	vtu = vu.createTypeUnion(id, rev2)
	if len(vtu) != 3 {
		t.Fatalf("wanted a type union with three elements, got %v: \n%#v", len(vtu), vtu)
	}

	vtu = vu.createTypeUnion(id, nil)
	if vtu != nil {
		t.Fatalf("wanted a nil return on nil input, got %#v", vtu)
	}
}

func TestTypeUnionIntersect(t *testing.T) {
	vu := versionUnifier{
		b:   lvfb1,
		mtr: newMetrics(),
	}

	rev1 := Revision("revision-one")
	rev2 := Revision("revision-two")
	rev3 := Revision("revision-three")
	id := mkPI("irrelevant")

	c, _ := NewSemverConstraint("^2.0.0")
	gotc := vu.intersect(id, rev2, c)
	if gotc != none {
		t.Fatalf("wanted empty set from intersect, got %#v", gotc)
	}

	gotc = vu.intersect(id, c, rev1)
	if gotc != none {
		t.Fatalf("wanted empty set from intersect, got %#v", gotc)
	}

	gotc = vu.intersect(id, c, rev3)
	if gotc != NewVersion("v2.0.5").Is(rev3) {
		t.Fatalf("wanted v2.0.5, got %s from intersect", gotc.typedString())
	}
}

func (lb lvFixBridge) SourceExists(ProjectIdentifier) (bool, error) {
	panic("not implemented")
}

func (lb lvFixBridge) SyncSourceFor(ProjectIdentifier) error {
	panic("not implemented")
}

func (lb lvFixBridge) RevisionPresentIn(ProjectIdentifier, Revision) (bool, error) {
	panic("not implemented")
}

func (lb lvFixBridge) ListPackages(ProjectIdentifier, Version) (pkgtree.PackageTree, error) {
	panic("not implemented")
}

func (lb lvFixBridge) GetManifestAndLock(ProjectIdentifier, Version, ProjectAnalyzer) (Manifest, Lock, error) {
	panic("not implemented")
}

func (lb lvFixBridge) ExportProject(ProjectIdentifier, Version, string) error {
	panic("not implemented")
}

func (lb lvFixBridge) DeduceProjectRoot(ip string) (ProjectRoot, error) {
	panic("not implemented")
}

func (lb lvFixBridge) verifyRootDir(path string) error {
	panic("not implemented")
}

func (lb lvFixBridge) vendorCodeExists(ProjectIdentifier) (bool, error) {
	panic("not implemented")
}

func (lb lvFixBridge) breakLock() {
	panic("not implemented")
}
