package vsolver

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestHashInputs(t *testing.T) {
	fix := basicFixtures[2]

	args := SolveArgs{
		Root:     string(fix.ds[0].Name()),
		Name:     fix.ds[0].Name(),
		Manifest: fix.ds[0],
	}

	// prep a fixture-overridden solver
	si, err := Prepare(args, SolveOpts{}, newdepspecSM(fix.ds))
	s := si.(*solver)
	if err != nil {
		t.Fatalf("Could not prepare solver due to err: %s", err)
	}

	fixb := &depspecBridge{
		s.b.(*bridge),
	}
	s.b = fixb

	dig, err := s.HashInputs()
	if err != nil {
		t.Fatalf("HashInputs returned unexpected err: %s", err)
	}

	h := sha256.New()
	for _, v := range []string{"a", "a", "1.0.0", "b", "b", "1.0.0", stdlibPkgs, "root", "", "root", "a", "b"} {
		h.Write([]byte(v))
	}
	correct := h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}
}
