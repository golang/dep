package simple

import (
	"go/parser"

	"github.com/sdboyer/vsolver"
)

var (
	_ = parser.ParseFile
	S = vsolver.Prepare
)
