// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/dep/test"
	"github.com/sdboyer/gps"
)

func TestReadLock(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	_, err := readLock(h.GetTestFile("lock/error.json"))
	if err == nil {
		t.Error("Reading lock with invalid props should have caused error, but did not")
	} else if !strings.Contains(err.Error(), "both a branch") {
		t.Errorf("Unexpected error %q; expected multiple version error", err)
	}

	l, err := readLock(h.GetTestFile("lock/golden.json"))
	if err != nil {
		t.Fatalf("Should have read Lock correctly, but got err %q", err)
	}

	b, _ := hex.DecodeString("2252a285ab27944a4d7adcba8dbd03980f59ba652f12db39fa93b927c345593e")
	l2 := &Lock{
		Memo: b,
		P: []gps.LockedProject{
			gps.NewLockedProject(
				gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/gps")},
				gps.NewBranch("master").Is(gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb")),
				[]string{"."},
			),
		},
	}

	if !reflect.DeepEqual(l, l2) {
		t.Error("Valid lock did not parse as expected")
	}
}

func TestWriteLock(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	lg := h.GetTestFileString("lock/golden.json")
	memo, _ := hex.DecodeString("2252a285ab27944a4d7adcba8dbd03980f59ba652f12db39fa93b927c345593e")
	l := &Lock{
		Memo: memo,
		P: []gps.LockedProject{
			gps.NewLockedProject(
				gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/gps")},
				gps.NewBranch("master").Is(gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb")),
				[]string{"."},
			),
		},
	}

	b, err := l.MarshalJSON()
	if err != nil {
		t.Fatalf("Error while marshaling valid lock to JSON: %q", err)
	}

	if string(b) != lg {
		t.Errorf("Valid lock did not marshal to JSON as expected:\n\t(GOT): %s\n\t(WNT): %s", string(b), lg)
	}
}
