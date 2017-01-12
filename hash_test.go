package gps

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"
)

func TestHashInputs(t *testing.T) {
	fix := basicFixtures["shared dependency with overlapping constraints"]

	params := SolveParameters{
		RootDir:         string(fix.ds[0].n),
		RootPackageTree: fix.rootTree(),
		Manifest:        fix.rootmanifest(),
	}

	s, err := Prepare(params, newdepspecSM(fix.ds, nil))
	if err != nil {
		t.Errorf("Unexpected error while prepping solver: %s", err)
		t.FailNow()
	}

	dig := s.HashInputs()
	h := sha256.New()

	elems := []string{
		"a",
		"1.0.0",
		"b",
		"1.0.0",
		"root",
		"root",
		"a",
		"b",
		"depspec-sm-builtin",
		"1.0.0",
	}
	for _, v := range elems {
		h.Write([]byte(v))
	}
	correct := h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Error("Hashes are not equal")
	}

	fixstr, hisstr := strings.Join(elems, "\n")+"\n", HashingInputsAsString(s)
	if fixstr != hisstr {
		t.Errorf("Hashing inputs not equal:\n\t(GOT) %s\n\t(WNT) %s", hisstr, fixstr)
	}
}

func TestHashInputsReqsIgs(t *testing.T) {
	fix := basicFixtures["shared dependency with overlapping constraints"]

	rm := fix.rootmanifest().(simpleRootManifest).dup()
	rm.ig = map[string]bool{
		"foo": true,
		"bar": true,
	}

	params := SolveParameters{
		RootDir:         string(fix.ds[0].n),
		RootPackageTree: fix.rootTree(),
		Manifest:        rm,
	}

	s, err := Prepare(params, newdepspecSM(fix.ds, nil))
	if err != nil {
		t.Errorf("Unexpected error while prepping solver: %s", err)
		t.FailNow()
	}

	dig := s.HashInputs()
	h := sha256.New()

	elems := []string{
		"a",
		"1.0.0",
		"b",
		"1.0.0",
		"root",
		"",
		"root",
		"a",
		"b",
		"bar",
		"foo",
		"depspec-sm-builtin",
		"1.0.0",
	}
	for _, v := range elems {
		h.Write([]byte(v))
	}
	correct := h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}

	fixstr, hisstr := strings.Join(elems, "\n")+"\n", HashingInputsAsString(s)
	if fixstr != hisstr {
		t.Errorf("Hashing inputs not equal:\n\t(GOT) %s\n\t(WNT) %s", hisstr, fixstr)
	}

	// Add requires
	rm.req = map[string]bool{
		"baz": true,
		"qux": true,
	}

	params.Manifest = rm

	s, err = Prepare(params, newdepspecSM(fix.ds, nil))
	if err != nil {
		t.Errorf("Unexpected error while prepping solver: %s", err)
		t.FailNow()
	}

	dig = s.HashInputs()
	h = sha256.New()

	elems = []string{
		"a",
		"1.0.0",
		"b",
		"1.0.0",
		"root",
		"",
		"root",
		"a",
		"b",
		"baz",
		"qux",
		"bar",
		"foo",
		"depspec-sm-builtin",
		"1.0.0",
	}
	for _, v := range elems {
		h.Write([]byte(v))
	}
	correct = h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}

	fixstr, hisstr = strings.Join(elems, "\n")+"\n", HashingInputsAsString(s)
	if fixstr != hisstr {
		t.Errorf("Hashing inputs not equal:\n\t(GOT) %s\n\t(WNT) %s", hisstr, fixstr)
	}

	// remove ignores, just test requires alone
	rm.ig = nil
	params.Manifest = rm

	s, err = Prepare(params, newdepspecSM(fix.ds, nil))
	if err != nil {
		t.Errorf("Unexpected error while prepping solver: %s", err)
		t.FailNow()
	}

	dig = s.HashInputs()
	h = sha256.New()

	elems = []string{
		"a",
		"1.0.0",
		"b",
		"1.0.0",
		"root",
		"",
		"root",
		"a",
		"b",
		"baz",
		"qux",
		"depspec-sm-builtin",
		"1.0.0",
	}
	for _, v := range elems {
		h.Write([]byte(v))
	}
	correct = h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}

	fixstr, hisstr = strings.Join(elems, "\n")+"\n", HashingInputsAsString(s)
	if fixstr != hisstr {
		t.Errorf("Hashing inputs not equal:\n\t(GOT) %s\n\t(WNT) %s", hisstr, fixstr)
	}
}

func TestHashInputsOverrides(t *testing.T) {
	fix := basicFixtures["shared dependency with overlapping constraints"]

	rm := fix.rootmanifest().(simpleRootManifest).dup()
	// First case - override something not in the root, just with network name
	rm.ovr = map[ProjectRoot]ProjectProperties{
		"c": ProjectProperties{
			Source: "car",
		},
	}
	params := SolveParameters{
		RootDir:         string(fix.ds[0].n),
		RootPackageTree: fix.rootTree(),
		Manifest:        rm,
	}

	s, err := Prepare(params, newdepspecSM(fix.ds, nil))
	if err != nil {
		t.Errorf("Unexpected error while prepping solver: %s", err)
		t.FailNow()
	}

	dig := s.HashInputs()
	h := sha256.New()

	elems := []string{
		"a",
		"1.0.0",
		"b",
		"1.0.0",
		"root",
		"",
		"root",
		"a",
		"b",
		"c",
		"car",
		"depspec-sm-builtin",
		"1.0.0",
	}
	for _, v := range elems {
		h.Write([]byte(v))
	}
	correct := h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}

	fixstr, hisstr := strings.Join(elems, "\n")+"\n", HashingInputsAsString(s)
	if fixstr != hisstr {
		t.Errorf("Hashing inputs not equal:\n\t(GOT) %s\n\t(WNT) %s", hisstr, fixstr)
	}

	// Override not in root, just with constraint
	rm.ovr["d"] = ProjectProperties{
		Constraint: NewBranch("foobranch"),
	}
	dig = s.HashInputs()
	h = sha256.New()

	elems = []string{
		"a",
		"1.0.0",
		"b",
		"1.0.0",
		"root",
		"",
		"root",
		"a",
		"b",
		"c",
		"car",
		"d",
		"foobranch",
		"depspec-sm-builtin",
		"1.0.0",
	}
	for _, v := range elems {
		h.Write([]byte(v))
	}
	correct = h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}

	fixstr, hisstr = strings.Join(elems, "\n")+"\n", HashingInputsAsString(s)
	if fixstr != hisstr {
		t.Errorf("Hashing inputs not equal:\n\t(GOT) %s\n\t(WNT) %s", hisstr, fixstr)
	}

	// Override not in root, both constraint and network name
	rm.ovr["e"] = ProjectProperties{
		Source:     "groucho",
		Constraint: NewBranch("plexiglass"),
	}
	dig = s.HashInputs()
	h = sha256.New()

	elems = []string{
		"a",
		"1.0.0",
		"b",
		"1.0.0",
		"root",
		"",
		"root",
		"a",
		"b",
		"c",
		"car",
		"d",
		"foobranch",
		"e",
		"groucho",
		"plexiglass",
		"depspec-sm-builtin",
		"1.0.0",
	}
	for _, v := range elems {
		h.Write([]byte(v))
	}
	correct = h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}

	fixstr, hisstr = strings.Join(elems, "\n")+"\n", HashingInputsAsString(s)
	if fixstr != hisstr {
		t.Errorf("Hashing inputs not equal:\n\t(GOT) %s\n\t(WNT) %s", hisstr, fixstr)
	}

	// Override in root, just constraint
	rm.ovr["a"] = ProjectProperties{
		Constraint: NewVersion("fluglehorn"),
	}
	dig = s.HashInputs()
	h = sha256.New()

	elems = []string{
		"a",
		"fluglehorn",
		"b",
		"1.0.0",
		"root",
		"",
		"root",
		"a",
		"b",
		"a",
		"fluglehorn",
		"c",
		"car",
		"d",
		"foobranch",
		"e",
		"groucho",
		"plexiglass",
		"depspec-sm-builtin",
		"1.0.0",
	}
	for _, v := range elems {
		h.Write([]byte(v))
	}
	correct = h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}

	fixstr, hisstr = strings.Join(elems, "\n")+"\n", HashingInputsAsString(s)
	if fixstr != hisstr {
		t.Errorf("Hashing inputs not equal:\n\t(GOT) %s\n\t(WNT) %s", hisstr, fixstr)
	}

	// Override in root, only network name
	rm.ovr["a"] = ProjectProperties{
		Source: "nota",
	}
	dig = s.HashInputs()
	h = sha256.New()

	elems = []string{
		"a",
		"nota",
		"1.0.0",
		"b",
		"1.0.0",
		"root",
		"",
		"root",
		"a",
		"b",
		"a",
		"nota",
		"c",
		"car",
		"d",
		"foobranch",
		"e",
		"groucho",
		"plexiglass",
		"depspec-sm-builtin",
		"1.0.0",
	}
	for _, v := range elems {
		h.Write([]byte(v))
	}
	correct = h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}

	fixstr, hisstr = strings.Join(elems, "\n")+"\n", HashingInputsAsString(s)
	if fixstr != hisstr {
		t.Errorf("Hashing inputs not equal:\n\t(GOT) %s\n\t(WNT) %s", hisstr, fixstr)
	}

	// Override in root, network name and constraint
	rm.ovr["a"] = ProjectProperties{
		Source:     "nota",
		Constraint: NewVersion("fluglehorn"),
	}
	dig = s.HashInputs()
	h = sha256.New()

	elems = []string{
		"a",
		"nota",
		"fluglehorn",
		"b",
		"1.0.0",
		"root",
		"",
		"root",
		"a",
		"b",
		"a",
		"nota",
		"fluglehorn",
		"c",
		"car",
		"d",
		"foobranch",
		"e",
		"groucho",
		"plexiglass",
		"depspec-sm-builtin",
		"1.0.0",
	}
	for _, v := range elems {
		h.Write([]byte(v))
	}
	correct = h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}

	fixstr, hisstr = strings.Join(elems, "\n")+"\n", HashingInputsAsString(s)
	if fixstr != hisstr {
		t.Errorf("Hashing inputs not equal:\n\t(GOT) %s\n\t(WNT) %s", hisstr, fixstr)
	}
}
