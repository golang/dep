// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package foo

import "sort"

var _ = sort.Strings

// yes, this is dumb, don't use ".go" in your directory names
// See https://github.com/golang/dep/issues/550 for more information
