// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/golang/dep"
	fb "github.com/golang/dep/internal/feedback"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
)

var (
	gomQx        = `'[^']*'|"[^"]*"`
	gomKx        = `:[a-z][a-z0-9_]*`
	gomAx        = `(?:\s*` + gomKx + `\s*|,\s*` + gomKx + `\s*)`
	gomReGroup   = regexp.MustCompile(`\s*group\s+((?:` + gomKx + `\s*|,\s*` + gomKx + `\s*)*)\s*do\s*$`)
	gomReEnd     = regexp.MustCompile(`\s*end\s*$`)
	gomReGom     = regexp.MustCompile(`^\s*gom\s+(` + gomQx + `)\s*((?:,\s*` + gomKx + `\s*=>\s*(?:` + gomQx + `|\s*\[\s*` + gomAx + `*\s*\]\s*))*)$`)
	gomReOptions = regexp.MustCompile(`(,\s*` + gomKx + `\s*=>\s*(?:` + gomQx + `|\s*\[\s*` + gomAx + `*\s*\]\s*)\s*)`)
)

const gomfileName = "Gomfile"

type gomImporter struct {
	goms []gomPackage

	logger  *log.Logger
	verbose bool
	sm      gps.SourceManager
}

func newGomImporter(logger *log.Logger, verbose bool, sm gps.SourceManager) *gomImporter {
	return &gomImporter{
		logger:  logger,
		verbose: verbose,
		sm:      sm,
	}
}

type gomPackage struct {
	name    string
	options map[string]interface{}
}

func (p *gomPackage) hasOption(name string) bool {
	_, ok := p.options[name]
	return ok
}

func (g *gomImporter) Name() string {
	return "gom"
}

func (g *gomImporter) HasDepMetadata(dir string) bool {
	y := filepath.Join(dir, gomfileName)
	if _, err := os.Stat(y); err != nil {
		return false
	}

	return true
}

func (g *gomImporter) Import(dir string, pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	err := g.load(dir)
	if err != nil {
		return nil, nil, err
	}

	return g.convert(pr)
}

func unquote(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > 2 {
		if (name[0] == '\'' && name[len(name)-1] == '\'') || (name[0] == '"' && name[len(name)-1] == '"') {
			return name[1 : len(name)-1]
		}
	}
	return name
}

func (g *gomImporter) has(c interface{}, key string) bool {
	if m, ok := c.(map[string]interface{}); ok {
		_, ok := m[key]
		return ok
	} else if a, ok := c.([]string); ok {
		for _, s := range a {
			if ok && s == key {
				return true
			}
		}
	}
	return false
}

func (g *gomImporter) parseOptions(line string, options map[string]interface{}) {
	ss := gomReOptions.FindAllStringSubmatch(line, -1)
	re := regexp.MustCompile(gomAx)
	for _, s := range ss {
		kvs := strings.SplitN(strings.TrimSpace(s[0])[1:], "=>", 2)
		kvs[0], kvs[1] = strings.TrimSpace(kvs[0]), strings.TrimSpace(kvs[1])
		if kvs[1][0] == '[' {
			as := re.FindAllStringSubmatch(kvs[1][1:len(kvs[1])-1], -1)
			a := []string{}
			for i := range as {
				it := strings.TrimSpace(as[i][0])
				if strings.HasPrefix(it, ",") {
					it = strings.TrimSpace(it[1:])
				}
				if strings.HasPrefix(it, ":") {
					it = strings.TrimSpace(it[1:])
				}
				a = append(a, it)
			}
			options[kvs[0][1:]] = a
		} else {
			options[kvs[0][1:]] = unquote(kvs[1])
		}
	}
}

// load the gomfile.
func (g *gomImporter) load(projectDir string) error {
	g.logger.Println("Detected Gomfile...")
	filename := filepath.Join(projectDir, gomfileName)
	f, err := os.Open(filename + ".lock")
	if err != nil {
		f, err = os.Open(filename)
		if err != nil {
			return err
		}
	}
	defer f.Close()
	br := bufio.NewReader(f)

	g.goms = make([]gomPackage, 0)

	n := 0
	skip := 0
	valid := true
	var envs []string
	for {
		n++
		lb, _, err := br.ReadLine()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line := strings.TrimSpace(string(lb))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		name := ""
		options := make(map[string]interface{})
		var items []string
		if gomReGroup.MatchString(line) {
			envs = strings.Split(gomReGroup.FindStringSubmatch(line)[1], ",")
			for i := range envs {
				envs[i] = strings.TrimSpace(envs[i])[1:]
			}
			valid = true
			continue
		} else if gomReEnd.MatchString(line) {
			if !valid {
				skip--
				if skip < 0 {
					return fmt.Errorf("Syntax Error at line %d", n)
				}
			}
			valid = false
			envs = nil
			continue
		} else if skip > 0 {
			continue
		} else if gomReGom.MatchString(line) {
			items = gomReGom.FindStringSubmatch(line)[1:]
			name = unquote(items[0])
			g.parseOptions(items[1], options)
		} else {
			return fmt.Errorf("Syntax Error at line %d", n)
		}
		if envs != nil {
			options["group"] = envs
		}
		g.goms = append(g.goms, gomPackage{name, options})
	}
}

// convert the gomfile into dep configuration files.
func (g *gomImporter) convert(pr gps.ProjectRoot) (*dep.Manifest, *dep.Lock, error) {
	g.logger.Println("Converting from Gomfile ...")

	manifest := &dep.Manifest{
		Constraints: make(gps.ProjectConstraints),
	}
	lock := &dep.Lock{}

	for _, pkg := range g.goms {
		// Obtain ProjectRoot. Required for avoiding sub-package imports.
		ip, err := g.sm.DeduceProjectRoot(pkg.name)
		if err != nil {
			return nil, nil, err
		}
		pkg.name = string(ip)

		// Check if it already existing in locked projects
		if projectExistsInLock(lock, pkg.name) {
			continue
		}

		rev := ""

		if pkg.hasOption("branch") {
			rev, _ = pkg.options["branch"].(string)
		}
		if pkg.hasOption("tag") {
			rev, _ = pkg.options["tag"].(string)
		}
		if pkg.hasOption("commit") {
			rev, _ = pkg.options["commit"].(string)
		}

		var pc gps.ProjectConstraint
		pc.Ident = gps.ProjectIdentifier{ProjectRoot: gps.ProjectRoot(pkg.name)}

		if rev != "" {
			pi := gps.ProjectIdentifier{
				ProjectRoot: gps.ProjectRoot(pkg.name),
			}
			revision := gps.Revision(rev)
			version, err := lookupVersionForRevision(revision, pi, g.sm)
			if err != nil {
				warn := errors.Wrapf(err, "Unable to lookup the version represented by %s in %s. Falling back to locking the revision only.", rev, pi.ProjectRoot)
				g.logger.Printf(warn.Error())
				version = revision
			}

			pc.Constraint, err = deduceConstraint(rev, pc.Ident, g.sm)
			if err != nil {
				return nil, nil, err
			}

			lp := gps.NewLockedProject(pi, version, nil)

			f := fb.NewLockedProjectFeedback(lp, fb.DepTypeImported)
			f.LogFeedback(g.logger)
			lock.P = append(lock.P, lp)
		}
		manifest.Constraints[pc.Ident.ProjectRoot] = gps.ProjectProperties{Source: pc.Ident.Source, Constraint: pc.Constraint}
	}

	return manifest, lock, nil
}
