package main

import (
	"os"
	"path/filepath"

	"github.com/golang/dep"
	"github.com/golang/dep/internal/gps"
)

const godepJsonName = "Godeps.json"

type godepImporter struct {
	loggers *dep.Loggers
	sm      gps.SourceManager
}

func newGodepImporter(loggers *dep.Loggers, sm gps.SourceManager) *godepImporter {
	return &godepImporter{loggers: loggers, sm: sm}
}

func (i godepImporter) HasConfig(dir string) bool {
	y := filepath.Join(dir, "Godeps", godepJsonName)
	if _, err := os.Stat(y); err != nil {
		return false
	}

	return true
}

func (i godepImporter) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	file := newGodepFile(i.loggers)
	err := file.load(dir)
	if err != nil {
		return nil, nil, err
	}

	return file.convert(string(pr), i.sm)
}
