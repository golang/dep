// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package pkgtree

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkListPackages(b *testing.B) {
	b.StopTimer()

	cwd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	root := filepath.Join(cwd, "..", "..", "..")

	b.StartTimer()

	for n := 0; n < b.N; n++ {
		_, err := ListPackages(root, "github.com/golang/dep")
		if err != nil {
			b.Error(err)
		}
	}
}
