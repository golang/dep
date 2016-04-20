package vsolver

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/Sirupsen/logrus"
)

func TestHashInputs(t *testing.T) {
	fix := fixtures[2]
	sm := newdepspecSM(fix.ds, true)

	l := logrus.New()
	if testing.Verbose() {
		l.Level = logrus.DebugLevel
	}

	s := NewSolver(sm, l)
	// TODO path is ignored right now, but we'll have to deal with that once
	// static analysis is in

	p, err := sm.GetProjectInfo(fix.ds[0].name)
	if err != nil {
		t.Error("couldn't find root project in fixture, aborting")
	}
	dig := s.HashInputs("", p.Manifest)

	h := sha256.New()
	for _, v := range []string{"a", "1.0.0", "b", "1.0.0"} {
		h.Write([]byte(v))
	}
	correct := h.Sum(nil)

	if !bytes.Equal(dig, correct) {
		t.Errorf("Hashes are not equal")
	}
}
