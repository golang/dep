package main

import "go/build"

// ctx defines the supporting context of the tool.
type ctx struct {
	GOPATH string // Go path
}

func newContext() *ctx {
	// this way we get the default GOPATH that was added in 1.8
	buildContext := build.Default
	return &ctx{
		GOPATH: buildContext.GOPATH,
	}
}
