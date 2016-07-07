package base

import (
	"go/parser"

	"github.com/sdboyer/vsolver"
)

var (
	_ = parser.ParseFile
	_ = vsolver.Solve
)
