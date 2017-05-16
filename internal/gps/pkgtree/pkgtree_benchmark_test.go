// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package pkgtree

import "testing"

func BenchmarkListPackages(b *testing.B) {
	b.StopTimer()

	j := func(s ...string) string {
		return testDir(b, s...)
	}

	table := []string{
		"dotgodir",
		"buildtag",
		"varied",
	}

	b.StartTimer()

	for _, name := range table {
		for n := 0; n < b.N; n++ {
			ListPackages(j(name), name)
		}
	}
}
