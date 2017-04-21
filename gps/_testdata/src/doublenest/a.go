package base

import (
	"go/parser"

	"github.com/sdboyer/gps"
)

var (
	_ = parser.ParseFile
	_ = gps.Solve
)
