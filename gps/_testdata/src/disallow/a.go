package disallow

import (
	"disallow/testdata"
	"sort"

	"github.com/golang/dep/gps"
)

var (
	_ = sort.Strings
	_ = gps.Solve
	_ = testdata.H
)
