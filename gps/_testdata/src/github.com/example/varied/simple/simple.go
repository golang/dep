package simple

import (
	"go/parser"

	"github.com/golang/dep/gps"
)

var (
	_ = parser.ParseFile
	S = gps.Prepare
)
