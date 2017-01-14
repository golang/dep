package gps

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"text/tabwriter"
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
		t.Errorf("Hashes are not equal. Inputs:\n%s", diffHashingInputs(s, elems))
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
		t.Errorf("Hashes are not equal. Inputs:\n%s", diffHashingInputs(s, elems))
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
		t.Errorf("Hashes are not equal. Inputs:\n%s", diffHashingInputs(s, elems))
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
		t.Errorf("Hashes are not equal. Inputs:\n%s", diffHashingInputs(s, elems))
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
		t.Errorf("Hashes are not equal. Inputs:\n%s", diffHashingInputs(s, elems))
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
		t.Errorf("Hashes are not equal. Inputs:\n%s", diffHashingInputs(s, elems))
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
		t.Errorf("Hashes are not equal. Inputs:\n%s", diffHashingInputs(s, elems))
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
		t.Errorf("Hashes are not equal. Inputs:\n%s", diffHashingInputs(s, elems))
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
		t.Errorf("Hashes are not equal. Inputs:\n%s", diffHashingInputs(s, elems))
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
		t.Errorf("Hashes are not equal. Inputs:\n%s", diffHashingInputs(s, elems))
	}
}

func diffHashingInputs(s Solver, wnt []string) string {
	actual := HashingInputsAsString(s)
	got := strings.Split(actual, "\n")

	lg, lw := len(got), len(wnt)

	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 4, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "  (GOT)  \t  (WANT)  \t")

	if lg == lw {
		// same length makes the loop pretty straightforward
		for i := 0; i < lg; i++ {
			fmt.Fprintf(tw, "%s\t%s\t\n", got[i], wnt[i])
		}
	} else if lg > lw {
		offset := 0
		for i := 0; i < lg; i++ {
			if lw <= i-offset {
				fmt.Fprintf(tw, "%s\t\t\n", got[i])
			} else if got[i] != wnt[i-offset] && got[i] == wnt[i-offset-1] {
				// if the next slot is a match, realign by skipping this one and
				// bumping the offset
				fmt.Fprintf(tw, "%s\t\t\n", got[i])
				offset++
			} else {
				fmt.Fprintf(tw, "%s\t%s\t\n", got[i], wnt[i-offset])
			}
		}
	} else {
		offset := 0
		for i := 0; i < lw; i++ {
			if lg <= i-offset {
				fmt.Fprintf(tw, "\t%s\t\n", wnt[i])
			} else if got[i-offset] != wnt[i] && got[i-offset-1] == wnt[i] {
				// if the next slot is a match, realign by skipping this one and
				// bumping the offset
				fmt.Fprintf(tw, "\t%s\t\n", wnt[i])
				offset++
			} else {
				fmt.Fprintf(tw, "%s\t%s\t\n", got[i-offset], wnt[i])
			}
		}
	}

	tw.Flush()
	return buf.String()
}
