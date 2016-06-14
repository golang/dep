package vsolver

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestHashInputs(t *testing.T) {
	fix := basicFixtures[2]

	opts := SolveOpts{
		// TODO path is ignored right now, but we'll have to deal with that once
		// static analysis is in
		Root: "foo",
		N:    ProjectName("root"),
		M:    fix.ds[0],
	}

	dig := opts.HashInputs()

	h := sha256.New()
	for _, v := range []string{"a", "a", "1.0.0", "b", "b", "1.0.0"} {
		h.Write([]byte(v))
	}
	correct := h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}
}
