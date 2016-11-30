// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/sdboyer/gps"
)

const le = `{
	"memo": "2252a285ab27944a4d7adcba8dbd03980f59ba652f12db39fa93b927c345593e",
	"projects": [
		{
			"name": "github.com/sdboyer/gps",
			"branch": "master",
			"version": "v0.12.0",
			"revision": "d05d5aca9f895d19e9265839bffeadd74a2d2ecb",
			"packages": [
				"."
			]
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
			"packages": [
				"."
			]
		}
	]
}`

func TestReadLock(t *testing.T) {
	_, err := readLock(strings.NewReader(le))
	if err == nil {
		t.Error("Reading lock with invalid props should have caused error, but did not")
	} else if !strings.Contains(err.Error(), "both a branch") {
		t.Errorf("Unexpected error %q; expected multiple version error", err)
	}

	l, err := readLock(strings.NewReader(lg))
	if err != nil {
		t.Fatalf("Should have read Lock correctly, but got err %q", err)
	}

	b, _ := hex.DecodeString("2252a285ab27944a4d7adcba8dbd03980f59ba652f12db39fa93b927c345593e")
	l2 := &lock{
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
	memo, _ := hex.DecodeString("2252a285ab27944a4d7adcba8dbd03980f59ba652f12db39fa93b927c345593e")
	l := &lock{
		Memo: memo,
		P: []gps.LockedProject{
			gps.NewLockedProject(
				gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot("github.com/sdboyer/gps")},
				gps.NewBranch("master").Is(gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb")),
				[]string{"."},
			),
		},
	}

	b, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("Error while marshaling valid lock to JSON: %q", err)
	}

	var out bytes.Buffer
	json.Indent(&out, b, "", "\t")

	s := out.String()
	if s != lg {
		t.Errorf("Valid lock did not marshal to JSON as expected:\n\t(GOT): %s\n\t(WNT): %s", s, lg)
	}
}
