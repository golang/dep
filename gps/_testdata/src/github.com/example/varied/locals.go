package main

import (
	"github.com/example/varied/namemismatch"
	"github.com/example/varied/otherpath"
	"github.com/example/varied/simple"
)

var (
	_ = simple.S
	_ = nm.V
	_ = otherpath.O
)
