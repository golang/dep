package main

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
)

func TestGodepImport(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	cacheDir := "gps-repocache"
	h.TempDir(cacheDir)
	h.TempDir("src")
	h.TempDir(filepath.Join("src", testGlideProjectRoot))
	h.TempCopy(filepath.Join(testGodepProjectRoot, "Godeps", godepJsonName), "Godeps.json")

	loggers := &dep.Loggers{
		Out:     log.New(os.Stdout, "", 0),
		Err:     log.New(os.Stderr, "", 0),
		Verbose: true,
	}
	projectRoot := h.Path(testGodepProjectRoot)
	sm, err := gps.NewSourceManager(h.Path(cacheDir))
	h.Must(err)

	i := newGodepImporter(loggers, sm)
	if !i.HasConfig(projectRoot) {
		t.Fatal("Expected the importer to detect the godep configuration file")
	}

	m, l, err := i.Import(projectRoot, gps.ProjectRoot(testGodepProjectRoot))
	h.Must(err)

	if m == nil {
		t.Fatal("Expected the manifest to be generated")
	}

	if l == nil {
		t.Fatal("Expected the lock to be generated")
	}
}
