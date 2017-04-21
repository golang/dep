package base

import (
	"go/parser"

	"github.com/golang/dep/gps"
)

var (
	_ = parser.ParseFile
	_ = gps.Solve
)
