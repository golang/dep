package gps

import (
	"strings"
)

var (
	osList     []string
	archList   []string
	ignoreTags = []string{} //[]string{"appengine", "ignore"} //TODO: appengine is a special case for now: https://github.com/tools/godep/issues/353
)

func init() {
	// The supported systems are listed in
	// https://github.com/golang/go/blob/master/src/go/build/syslist.go
	// The lists are not exported, so we need to duplicate them here.
	osListString := "android darwin dragonfly freebsd linux nacl netbsd openbsd plan9 solaris windows"
	osList = strings.Split(osListString, " ")

	archListString := "386 amd64 amd64p32 arm armbe arm64 arm64be ppc64 ppc64le mips mipsle mips64 mips64le mips64p32 mips64p32le ppc s390 s390x sparc sparc64"
	archList = strings.Split(archListString, " ")
}

// Stored as a var so that tests can swap it out. Ugh globals, ugh.
var isStdLib = doIsStdLib

// This was lovingly lifted from src/cmd/go/pkg.go in Go's code
// (isStandardImportPath).
func doIsStdLib(path string) bool {
	i := strings.Index(path, "/")
	if i < 0 {
		i = len(path)
	}

	return !strings.Contains(path[:i], ".")
}
