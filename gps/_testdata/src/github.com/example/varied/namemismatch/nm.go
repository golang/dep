package nm

import (
	"os"

	"github.com/Masterminds/semver"
)

var (
	V = os.FileInfo
	_ = semver.Constraint
)
