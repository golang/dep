package gps

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestStringDiff_NoChange(t *testing.T) {
	diff := StringDiff{Previous: "foo", Current: "foo"}
	want := "foo"
	got := diff.String()
	if got != want {
		t.Fatalf("Expected '%s', got '%s'", want, got)
	}
}

func TestStringDiff_Add(t *testing.T) {
	diff := StringDiff{Current: "foo"}
	got := diff.String()
	if got != "+ foo" {
		t.Fatalf("Expected '+ foo', got '%s'", got)
	}
}

func TestStringDiff_Remove(t *testing.T) {
	diff := StringDiff{Previous: "foo"}
	want := "- foo"
	got := diff.String()
	if got != want {
		t.Fatalf("Expected '%s', got '%s'", want, got)
	}
}

func TestStringDiff_Modify(t *testing.T) {
	diff := StringDiff{Previous: "foo", Current: "bar"}
	want := "foo -> bar"
	got := diff.String()
	if got != want {
		t.Fatalf("Expected '%s', got '%s'", want, got)
	}
}

func TestDiffProjects_NoChange(t *testing.T) {
	p1 := NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v0.10.0"), []string{"gps"})
	p2 := NewLockedProject(mkPI("github.com/sdboyer/gps"), NewVersion("v0.10.0"), []string{"gps"})

	diff := DiffProjects(p1, p2)
	if diff != nil {
		t.Fatal("Expected the diff to be nil")
	}
}

func TestDiffProjects_Modify(t *testing.T) {
	p1 := LockedProject{
		pi:   ProjectIdentifier{ProjectRoot: "github.com/foo/bar"},
		v:    NewBranch("master"),
		r:    "abc123",
		pkgs: []string{"baz", "qux"},
	}

	p2 := LockedProject{
		pi:   ProjectIdentifier{ProjectRoot: "github.com/foo/bar", Source: "https://github.com/mcfork/gps.git"},
		v:    NewVersion("v1.0.0"),
		r:    "def456",
		pkgs: []string{"baz", "derp"},
	}

	diff := DiffProjects(p1, p2)
	if diff == nil {
		t.Fatal("Expected the diff to be populated")
	}

	wantSource := "+ https://github.com/mcfork/gps.git"
	gotSource := diff.Source.String()
	if gotSource != wantSource {
		t.Fatalf("Expected diff.Source to be '%s', got '%s'", wantSource, diff.Source)
	}

	wantVersion := "+ v1.0.0"
	gotVersion := diff.Version.String()
	if gotVersion != wantVersion {
		t.Fatalf("Expected diff.Version to be '%s', got '%s'", wantVersion, gotVersion)
	}

	wantRevision := "abc123 -> def456"
	gotRevision := diff.Revision.String()
	if gotRevision != wantRevision {
		t.Fatalf("Expected diff.Revision to be '%s', got '%s'", wantRevision, gotRevision)
	}

	wantBranch := "- master"
	gotBranch := diff.Branch.String()
	if gotBranch != wantBranch {
		t.Fatalf("Expected diff.Branch to be '%s', got '%s'", wantBranch, gotBranch)
	}

	fmtPkgs := func(pkgs []StringDiff) string {
		b := bytes.NewBufferString("[")
		for _, pkg := range pkgs {
			b.WriteString(pkg.String())
			b.WriteString(",")
		}
		b.WriteString("]")
		return b.String()
	}

	wantPackages := "[+ derp,- qux,]"
	gotPackages := fmtPkgs(diff.Packages)
	if gotPackages != wantPackages {
		t.Fatalf("Expected diff.Packages to be '%s', got '%s'", wantPackages, gotPackages)
	}
}

func TestDiffProjects_AddPackages(t *testing.T) {
	p1 := LockedProject{
		pi:   ProjectIdentifier{ProjectRoot: "github.com/foo/bar"},
		v:    NewBranch("master"),
		r:    "abc123",
		pkgs: []string{"foobar"},
	}

	p2 := LockedProject{
		pi:   ProjectIdentifier{ProjectRoot: "github.com/foo/bar", Source: "https://github.com/mcfork/gps.git"},
		v:    NewVersion("v1.0.0"),
		r:    "def456",
		pkgs: []string{"bazqux", "foobar", "zugzug"},
	}

	diff := DiffProjects(p1, p2)
	if diff == nil {
		t.Fatal("Expected the diff to be populated")
	}

	if len(diff.Packages) != 2 {
		t.Fatalf("Expected diff.Packages to have 2 packages, got %d", len(diff.Packages))
	}

	want0 := "+ bazqux"
	got0 := diff.Packages[0].String()
	if got0 != want0 {
		t.Fatalf("Expected diff.Packages[0] to contain %s, got %s", want0, got0)
	}

	want1 := "+ zugzug"
	got1 := diff.Packages[1].String()
	if got1 != want1 {
		t.Fatalf("Expected diff.Packages[1] to contain %s, got %s", want1, got1)
	}
}

func TestDiffProjects_RemovePackages(t *testing.T) {
	p1 := LockedProject{
		pi:   ProjectIdentifier{ProjectRoot: "github.com/foo/bar"},
		v:    NewBranch("master"),
		r:    "abc123",
		pkgs: []string{"athing", "foobar"},
	}

	p2 := LockedProject{
		pi:   ProjectIdentifier{ProjectRoot: "github.com/foo/bar", Source: "https://github.com/mcfork/gps.git"},
		v:    NewVersion("v1.0.0"),
		r:    "def456",
		pkgs: []string{"bazqux"},
	}

	diff := DiffProjects(p1, p2)
	if diff == nil {
		t.Fatal("Expected the diff to be populated")
	}

	if len(diff.Packages) > 3 {
		t.Fatalf("Expected diff.Packages to have 3 packages, got %d", len(diff.Packages))
	}

	want0 := "- athing"
	got0 := diff.Packages[0].String()
	if got0 != want0 {
		t.Fatalf("Expected diff.Packages[0] to contain %s, got %s", want0, got0)
	}

	// diff.Packages[1] is '+ bazqux'

	want2 := "- foobar"
	got2 := diff.Packages[2].String()
	if got2 != want2 {
		t.Fatalf("Expected diff.Packages[2] to contain %s, got %s", want2, got2)
	}
}

func TestDiffLocks_NoChange(t *testing.T) {
	l1 := safeLock{
		h: []byte("abc123"),
		p: []LockedProject{
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bar"}, v: NewVersion("v1.0.0")},
		},
	}
	l2 := safeLock{
		h: []byte("abc123"),
		p: []LockedProject{
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bar"}, v: NewVersion("v1.0.0")},
		},
	}

	diff := DiffLocks(l1, l2)
	if diff != nil {
		t.Fatal("Expected the diff to be nil")
	}
}

func TestDiffLocks_AddProjects(t *testing.T) {
	l1 := safeLock{
		h: []byte("abc123"),
		p: []LockedProject{
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bar"}, v: NewVersion("v1.0.0")},
		},
	}
	l2 := safeLock{
		h: []byte("abc123"),
		p: []LockedProject{
			{
				pi:   ProjectIdentifier{ProjectRoot: "github.com/baz/qux", Source: "https://github.com/mcfork/bazqux.git"},
				v:    NewVersion("v0.5.0"),
				r:    "def456",
				pkgs: []string{"p1", "p2"},
			},
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bar"}, v: NewVersion("v1.0.0")},
			{pi: ProjectIdentifier{ProjectRoot: "github.com/zug/zug"}, v: NewVersion("v1.0.0")},
		},
	}

	diff := DiffLocks(l1, l2)
	if diff == nil {
		t.Fatal("Expected the diff to be populated")
	}

	if len(diff.Add) != 2 {
		t.Fatalf("Expected diff.Add to have 2 projects, got %d", len(diff.Add))
	}

	want0 := "github.com/baz/qux"
	got0 := string(diff.Add[0].Name)
	if got0 != want0 {
		t.Fatalf("Expected diff.Add[0] to contain %s, got %s", want0, got0)
	}

	want1 := "github.com/zug/zug"
	got1 := string(diff.Add[1].Name)
	if got1 != want1 {
		t.Fatalf("Expected diff.Add[1] to contain %s, got %s", want1, got1)
	}

	add0 := diff.Add[0]
	wantSource := "https://github.com/mcfork/bazqux.git"
	gotSource := add0.Source.String()
	if gotSource != wantSource {
		t.Fatalf("Expected diff.Add[0].Source to be '%s', got '%s'", wantSource, add0.Source)
	}

	wantVersion := "v0.5.0"
	gotVersion := add0.Version.String()
	if gotVersion != wantVersion {
		t.Fatalf("Expected diff.Add[0].Version to be '%s', got '%s'", wantVersion, gotVersion)
	}

	wantRevision := "def456"
	gotRevision := add0.Revision.String()
	if gotRevision != wantRevision {
		t.Fatalf("Expected diff.Add[0].Revision to be '%s', got '%s'", wantRevision, gotRevision)
	}

	wantBranch := ""
	gotBranch := add0.Branch.String()
	if gotBranch != wantBranch {
		t.Fatalf("Expected diff.Add[0].Branch to be '%s', got '%s'", wantBranch, gotBranch)
	}

	fmtPkgs := func(pkgs []StringDiff) string {
		b := bytes.NewBufferString("[")
		for _, pkg := range pkgs {
			b.WriteString(pkg.String())
			b.WriteString(",")
		}
		b.WriteString("]")
		return b.String()
	}

	wantPackages := "[p1,p2,]"
	gotPackages := fmtPkgs(add0.Packages)
	if gotPackages != wantPackages {
		t.Fatalf("Expected diff.Add[0].Packages to be '%s', got '%s'", wantPackages, gotPackages)
	}
}

func TestDiffLocks_RemoveProjects(t *testing.T) {
	l1 := safeLock{
		h: []byte("abc123"),
		p: []LockedProject{
			{
				pi:   ProjectIdentifier{ProjectRoot: "github.com/a/thing", Source: "https://github.com/mcfork/athing.git"},
				v:    NewBranch("master"),
				r:    "def456",
				pkgs: []string{"p1", "p2"},
			},
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bar"}, v: NewVersion("v1.0.0")},
		},
	}
	l2 := safeLock{
		h: []byte("abc123"),
		p: []LockedProject{
			{pi: ProjectIdentifier{ProjectRoot: "github.com/baz/qux"}, v: NewVersion("v1.0.0")},
		},
	}

	diff := DiffLocks(l1, l2)
	if diff == nil {
		t.Fatal("Expected the diff to be populated")
	}

	if len(diff.Remove) != 2 {
		t.Fatalf("Expected diff.Remove to have 2 projects, got %d", len(diff.Remove))
	}

	want0 := "github.com/a/thing"
	got0 := string(diff.Remove[0].Name)
	if got0 != want0 {
		t.Fatalf("Expected diff.Remove[0] to contain %s, got %s", want0, got0)
	}

	want1 := "github.com/foo/bar"
	got1 := string(diff.Remove[1].Name)
	if got1 != want1 {
		t.Fatalf("Expected diff.Remove[1] to contain %s, got %s", want1, got1)
	}

	remove0 := diff.Remove[0]
	wantSource := "https://github.com/mcfork/athing.git"
	gotSource := remove0.Source.String()
	if gotSource != wantSource {
		t.Fatalf("Expected diff.Remove[0].Source to be '%s', got '%s'", wantSource, remove0.Source)
	}

	wantVersion := ""
	gotVersion := remove0.Version.String()
	if gotVersion != wantVersion {
		t.Fatalf("Expected diff.Remove[0].Version to be '%s', got '%s'", wantVersion, gotVersion)
	}

	wantRevision := "def456"
	gotRevision := remove0.Revision.String()
	if gotRevision != wantRevision {
		t.Fatalf("Expected diff.Remove[0].Revision to be '%s', got '%s'", wantRevision, gotRevision)
	}

	wantBranch := "master"
	gotBranch := remove0.Branch.String()
	if gotBranch != wantBranch {
		t.Fatalf("Expected diff.Remove[0].Branch to be '%s', got '%s'", wantBranch, gotBranch)
	}

	fmtPkgs := func(pkgs []StringDiff) string {
		b := bytes.NewBufferString("[")
		for _, pkg := range pkgs {
			b.WriteString(pkg.String())
			b.WriteString(",")
		}
		b.WriteString("]")
		return b.String()
	}

	wantPackages := "[p1,p2,]"
	gotPackages := fmtPkgs(remove0.Packages)
	if gotPackages != wantPackages {
		t.Fatalf("Expected diff.Remove[0].Packages to be '%s', got '%s'", wantPackages, gotPackages)
	}
}

func TestDiffLocks_ModifyProjects(t *testing.T) {
	l1 := safeLock{
		h: []byte("abc123"),
		p: []LockedProject{
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bar"}, v: NewVersion("v1.0.0")},
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bu"}, v: NewVersion("v1.0.0")},
			{pi: ProjectIdentifier{ProjectRoot: "github.com/zig/zag"}, v: NewVersion("v1.0.0")},
		},
	}
	l2 := safeLock{
		h: []byte("abc123"),
		p: []LockedProject{
			{pi: ProjectIdentifier{ProjectRoot: "github.com/baz/qux"}, v: NewVersion("v1.0.0")},
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bar"}, v: NewVersion("v2.0.0")},
			{pi: ProjectIdentifier{ProjectRoot: "github.com/zig/zag"}, v: NewVersion("v2.0.0")},
			{pi: ProjectIdentifier{ProjectRoot: "github.com/zug/zug"}, v: NewVersion("v1.0.0")},
		},
	}

	diff := DiffLocks(l1, l2)
	if diff == nil {
		t.Fatal("Expected the diff to be populated")
	}

	if len(diff.Modify) != 2 {
		t.Fatalf("Expected diff.Remove to have 2 projects, got %d", len(diff.Remove))
	}

	want0 := "github.com/foo/bar"
	got0 := string(diff.Modify[0].Name)
	if got0 != want0 {
		t.Fatalf("Expected diff.Modify[0] to contain %s, got %s", want0, got0)
	}

	want1 := "github.com/zig/zag"
	got1 := string(diff.Modify[1].Name)
	if got1 != want1 {
		t.Fatalf("Expected diff.Modify[1] to contain %s, got %s", want1, got1)
	}
}

func TestDiffLocks_ModifyHash(t *testing.T) {
	h1, _ := hex.DecodeString("abc123")
	l1 := safeLock{
		h: h1,
		p: []LockedProject{
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bar"}, v: NewVersion("v1.0.0")},
		},
	}

	h2, _ := hex.DecodeString("def456")
	l2 := safeLock{
		h: h2,
		p: []LockedProject{
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bar"}, v: NewVersion("v1.0.0")},
		},
	}

	diff := DiffLocks(l1, l2)
	if diff == nil {
		t.Fatal("Expected the diff to be populated")
	}

	want := "abc123 -> def456"
	got := diff.HashDiff.String()
	if got != want {
		t.Fatalf("Expected diff.HashDiff to be '%s', got '%s'", want, got)
	}
}

func TestDiffLocks_EmptyInitialLock(t *testing.T) {
	h2, _ := hex.DecodeString("abc123")
	l2 := safeLock{
		h: h2,
		p: []LockedProject{
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bar"}, v: NewVersion("v1.0.0")},
		},
	}

	diff := DiffLocks(nil, l2)

	wantHash := "+ abc123"
	gotHash := diff.HashDiff.String()
	if gotHash != wantHash {
		t.Fatalf("Expected diff.HashDiff to be '%s', got '%s'", wantHash, gotHash)
	}

	if len(diff.Add) != 1 {
		t.Fatalf("Expected diff.Add to contain 1 project, got %d", len(diff.Add))
	}
}

func TestDiffLocks_EmptyFinalLock(t *testing.T) {
	h1, _ := hex.DecodeString("abc123")
	l1 := safeLock{
		h: h1,
		p: []LockedProject{
			{pi: ProjectIdentifier{ProjectRoot: "github.com/foo/bar"}, v: NewVersion("v1.0.0")},
		},
	}

	diff := DiffLocks(l1, nil)

	wantHash := "- abc123"
	gotHash := diff.HashDiff.String()
	if gotHash != wantHash {
		t.Fatalf("Expected diff.HashDiff to be '%s', got '%s'", wantHash, gotHash)
	}

	if len(diff.Remove) != 1 {
		t.Fatalf("Expected diff.Remove to contain 1 project, got %d", len(diff.Remove))
	}
}

func TestDiffLocks_EmptyLocks(t *testing.T) {
	diff := DiffLocks(nil, nil)
	if diff != nil {
		t.Fatal("Expected the diff to be empty")
	}
}
