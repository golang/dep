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

	golden := "lock/golden0.json"
	l, err := readLock(h.GetTestFile(golden))
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

	golden = "lock/golden1.json"
	l, err = readLock(h.GetTestFile(golden))
	if err != nil {
		t.Fatalf("Should have read Lock correctly, but got err %q", err)
	}

	b, _ = hex.DecodeString("2252a285ab27944a4d7adcba8dbd03980f59ba652f12db39fa93b927c345593e")
	l2 = &Lock{
		Memo: b,
		P: []gps.LockedProject{
			gps.NewLockedProject(
				gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/gps")},
				gps.NewVersion("0.12.2").Is(gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb")),
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

	golden := "lock/golden0.json"
	lg := h.GetTestFileString(golden)
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
		if *test.UpdateGolden {
			if err = h.WriteTestFile(golden, string(b)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("Valid lock did not marshal to JSON as expected:\n\t(GOT): %s\n\t(WNT): %s", lg, string(b))
		}
	}

	golden = "lock/golden1.json"
	lg = h.GetTestFileString(golden)
	memo, _ = hex.DecodeString("2252a285ab27944a4d7adcba8dbd03980f59ba652f12db39fa93b927c345593e")
	l = &Lock{
		Memo: memo,
		P: []gps.LockedProject{
			gps.NewLockedProject(
				gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/gps")},
				gps.NewVersion("0.12.2").Is(gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb")),
				[]string{"."},
			),
		},
	}

	b, err = l.MarshalJSON()
	if err != nil {
		t.Fatalf("Error while marshaling valid lock to JSON: %q", err)
	}

	if string(b) != lg {
		if *test.UpdateGolden {
			if err = h.WriteTestFile(golden, string(b)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("Valid lock did not marshal to JSON as expected:\n\t(GOT): %s\n\t(WNT): %s", lg, string(b))
		}
	}
}

func TestReadLockErrors(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	var err error

	tests := []struct {
		name string
		file string
	}{
		{"specified both", "lock/error0.json"},
		{"invalid hash", "lock/error1.json"},
		{"no branch or version", "lock/error2.json"},
	}

	for _, tst := range tests {
		_, err = readLock(h.GetTestFile(tst.file))
		if err == nil {
			t.Errorf("Reading lock with %s should have caused error, but did not", tst.name)
		} else if !strings.Contains(err.Error(), tst.name) {
			t.Errorf("Unexpected error %q; expected %s error", err, tst.name)
		}
	}

	// _, err = readLock(h.GetTestFile("lock/error0.json"))
	// if err == nil {
	// 	t.Error("Reading lock with invalid props should have caused error, but did not")
	// } else if !strings.Contains(err.Error(), "both a branch") {
	// 	t.Errorf("Unexpected error %q; expected multiple version error", err)
	// }
	//
	// _, err = readLock(h.GetTestFile("lock/error1.json"))
	// if err == nil {
	// 	t.Error("Reading lock with invalid hash should have caused error, but did not")
	// } else if !strings.Contains(err.Error(), "invalid hash") {
	// 	t.Errorf("Unexpected error %q; expected invalid hash error", err)
	// }
	//
	// _, err = readLock(h.GetTestFile("lock/error2.json"))
	// if err == nil {
	// 	t.Error("Reading lock with invalid props should have caused error, but did not")
	// } else if !strings.Contains(err.Error(), "no version") {
	// 	t.Errorf("Unexpected error %q; expected no version error", err)
	// }

}
