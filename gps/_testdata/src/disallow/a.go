package disallow

import (
	"sort"
	"disallow/testdata"

	"github.com/golang/dep/gps"
)

var (
	_ = sort.Strings
	_ = gps.Solve
	_ = testdata.H
)
