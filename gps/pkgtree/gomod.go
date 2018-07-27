package pkgtree

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// goModPath was taken, nearly, verbatim from: go1.10.3/src/cmd/go/internal/load/pkg.go

var (
	modulePrefix       = []byte("\nmodule ")
	goModPathCache     = make(map[string]string)
	goModPathCacheLock sync.RWMutex
)

// goModPath returns the module path in the go.mod in dir, if any.
func goModPath(dir string) (path string) {
	goModPathCacheLock.RLock()
	path, ok := goModPathCache[dir]
	goModPathCacheLock.RUnlock()
	if ok {
		return path
	}
	defer func() {
		goModPathCacheLock.Lock()
		goModPathCache[dir] = path
		goModPathCacheLock.Unlock()
	}()

	data, err := ioutil.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	var i int
	if bytes.HasPrefix(data, modulePrefix[1:]) {
		i = 0
	} else {
		i = bytes.Index(data, modulePrefix)
		if i < 0 {
			return ""
		}
		i++
	}
	line := data[i:]

	// Cut line at \n, drop trailing \r if present.
	if j := bytes.IndexByte(line, '\n'); j >= 0 {
		line = line[:j]
	}
	if line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	line = line[len("module "):]

	// If quoted, unquote.
	path = strings.TrimSpace(string(line))
	if path != "" && path[0] == '"' {
		s, err := strconv.Unquote(path)
		if err != nil {
			return ""
		}
		path = s
	}
	return path
}
