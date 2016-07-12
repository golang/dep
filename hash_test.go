package gps

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestHashInputs(t *testing.T) {
	fix := basicFixtures["shared dependency with overlapping constraints"]

	params := SolveParameters{
		RootDir:    string(fix.ds[0].n),
		ImportRoot: fix.ds[0].n,
		Manifest:   fix.ds[0],
		Ignore:     []string{"foo", "bar"},
	}

	s, err := Prepare(params, newdepspecSM(fix.ds, nil))

	dig, err := s.HashInputs()
	if err != nil {
		t.Fatalf("HashInputs returned unexpected err: %s", err)
	}

	h := sha256.New()
	for _, v := range []string{"a", "a", "1.0.0", "b", "b", "1.0.0", stdlibPkgs, appenginePkgs, "root", "", "root", "a", "b", "bar", "foo"} {
		h.Write([]byte(v))
	}
	correct := h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}
}
