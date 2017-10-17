// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"github.com/d4l3k/messagediff"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// Diff compares structs, arrays and strings and provides pretty output.
// Return values are diff which is the difference between values in
// string format meant to be output in the console. equal is boolean flag
// indicating if the values are equal.
func Diff(a, b interface{}) (diff string, equal bool) {
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		// if both a and b are strings, compare them as such
		dmp := diffmatchpatch.New()
		diff := dmp.DiffMain(as, bs, false)
		return dmp.DiffPrettyText(diff), as == bs
	}
	// otherwise compare them as structs
	return messagediff.PrettyDiff(a, b)
}
