// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/sdboyer/gps"
)

func TestReadLock(t *testing.T) {
	const le = `{
    "memo": "2252a285ab27944a4d7adcba8dbd03980f59ba652f12db39fa93b927c345593e",
    "projects": [
        {
            "name": "github.com/sdboyer/gps",
            "branch": "master",
			"version": "v0.12.0",
            "revision": "d05d5aca9f895d19e9265839bffeadd74a2d2ecb",
            "packages": ["."]
        }
    ]
}`
	const lg = `{
    "memo": "2252a285ab27944a4d7adcba8dbd03980f59ba652f12db39fa93b927c345593e",
    "projects": [
        {
            "name": "github.com/sdboyer/gps",
            "branch": "master",
            "revision": "d05d5aca9f895d19e9265839bffeadd74a2d2ecb",
            "packages": ["."]
        }
    ]
}`

	_, err := ReadLock(strings.NewReader(le))
	if err == nil {
		t.Error("Reading lock with invalid props should have caused error, but did not")
	} else if !strings.Contains(err.Error(), "both a branch") {
		t.Errorf("Unexpected error %q; expected multiple version error", err)
	}

	l, err := ReadLock(strings.NewReader(lg))
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
