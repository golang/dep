// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gps

import (
	"bytes"
	"testing"
	"time"
)

func TestCacheEncodingUnpairedVersion(t *testing.T) {
	for _, test := range []struct {
		enc string
		uv  UnpairedVersion
	}{
		{"defaultBranch:test", newDefaultBranch("test")},
		{"branch:test", NewBranch("test")},
		{"ver:test", NewVersion("test")},
	} {
		t.Run(test.enc, func(t *testing.T) {
			b, err := cacheEncodeUnpairedVersion(test.uv)
			if err != nil {
				t.Error("failed to encode", err)
			} else if got := string(b); got != test.enc {
				t.Error("unexpected encoded result:", got)
			}

			got, err := cacheDecodeUnpairedVersion([]byte(test.enc))
			if err != nil {
				t.Error("failed to decode:", err)
			} else if !got.identical(test.uv) {
				t.Errorf("decoded non-identical UnpairedVersion:\n\t(GOT): %#v\n\t(WNT): %#v", got, test.uv)
			}
		})
	}
}

func TestCacheEncodingProjectProperties(t *testing.T) {
	for _, test := range []struct {
		k, v string
		ip   ProjectRoot
		pp   ProjectProperties
	}{
		{"root", "defaultBranch:test",
			"root", ProjectProperties{"", newDefaultBranch("test")}},
		{"root,source", "branch:test",
			"root", ProjectProperties{"source", NewBranch("test")}},
		{"root", "ver:^1.0.0",
			"root", ProjectProperties{"", testSemverConstraint(t, "^1.0.0")}},
		{"root,source", "rev:test",
			"root", ProjectProperties{"source", Revision("test")}},
	} {
		t.Run(test.k+"/"+test.v, func(t *testing.T) {
			kb, vb, err := cacheEncodeProjectProperties(test.ip, test.pp)
			k, v := string(kb), string(vb)
			if err != nil {
				t.Error("failed to encode", err)
			} else {
				if k != test.k {
					t.Error("unexpected encoded key:", k)
				}
				if v != test.v {
					t.Error("unexpected encoded value:", v)
				}
			}

			ip, pp, err := cacheDecodeProjectProperties([]byte(test.k), []byte(test.v))
			if err != nil {
				t.Error("failed to decode:", err)
			} else {
				if ip != test.ip {
					t.Errorf("decoded unexpected ProjectRoot:\n\t(GOT): %#v\n\t(WNT): %#v", ip, test.ip)
				}
				if pp.Source != test.pp.Source {
					t.Errorf("decoded unexpected ProjectRoot.Source:\n\t(GOT): %s\n\t (WNT): %s", pp.Source, test.pp.Source)
				}
				if !pp.Constraint.identical(test.pp.Constraint) {
					t.Errorf("decoded non-identical ProjectRoot.Constraint:\n\t(GOT): %#v\n\t(WNT): %#v", pp.Constraint, test.pp.Constraint)
				}
			}
		})
	}
}

func TestCacheEncodingTimestampedKey(t *testing.T) {
	for _, test := range []struct {
		ts     time.Time
		suffix []byte
	}{
		{time.Unix(0, 0), []byte{0, 0, 0, 0, 0, 0, 0, 0}},
		{time.Unix(100, 0), []byte{0, 0, 0, 0, 0, 0, 0, 100}},
		{time.Unix(255, 0), []byte{0, 0, 0, 0, 0, 0, 0, 255}},
		{time.Unix(1+1<<8+1<<16+1<<24, 0), []byte{0, 0, 0, 0, 1, 1, 1, 1}},
		{time.Unix(255<<48, 0), []byte{0, 255, 0, 0, 0, 0, 0, 0}},
	} {
		b := cacheTimestampedKey("pre:", test.ts)
		if !bytes.Equal(b, append([]byte("pre:"), test.suffix...)) {
			t.Errorf("unexpected suffix:\n\t(GOT):%v\n\t(WNT):%v", b[4:], test.suffix)
		}
	}
}
