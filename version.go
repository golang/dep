package main

import (
	"fmt"
	"runtime"
)

var (
	// VERSION indicates which version of the binary is running.
	VERSION string

	// GITCOMMIT indicates which git hash the binary was built off of
	GITCOMMIT string
)

var versionCmd = &command{
	fn:   runVersion,
	name: "version",
	short: `
	Version prints the version, git commit, runtime OS and ARCH.
	`,
	long: `Version prints the version, git commit, runtime OS and ARCH.`,
}

func runVersion(args []string) error {
	fmt.Printf("dep version %s %s %s/%s\n", VERSION, GITCOMMIT, runtime.GOOS, runtime.GOARCH)
	return nil
}
