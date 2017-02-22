package simple

import (
	"go/parser"

	"github.com/sdboyer/gps"
)

var (
	_ = parser.ParseFile
	S = gps.Prepare
)
