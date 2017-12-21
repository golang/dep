//+build mage

// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/magefile/mage/sh"
)

// This script will build dep and calculate hash for each
// (DEP_BUILD_PLATFORMS, DEP_BUILD_ARCHS) pair.
// DEP_BUILD_PLATFORMS="linux" DEP_BUILD_ARCHS="amd64" ./hack/build-all.sh
// can be called to build only for linux-amd64
func Build() error {
	platforms := []string{"linux", "windows", "darwin"}
	if v := os.Getenv("DEP_BUILD_PLATFORMS"); v != "" {
		platforms = strings.Split(v, " ")
	}

	arches := []string{"amd64"}
	if v := os.Getenv("DEP_BUILD_ARCHS"); v != "" {
		arches = strings.Split(v, " ")
	}

	if err := os.MkdirAll("release", 0700); err != nil {
		return err
	}

	for _, platform := range platforms {
		for _, arch := range arches {
			name := fmt.Sprintf("dep-%s-%s", platform, arch)
			if platform == "windows" {
				name += ".exe"
			}
			name = filepath.Join("release", name)
			fmt.Printf("Building for %s/%s\n", platform, arch)
			env := map[string]string{
				"GOOS":        platform,
				"GOARCH":      arch,
				"CGO_ENABLED": "0",
			}
			err := sh.RunWith(env, "go", "build", "-a", "-installsuffix", "cgo", "-ldflags", flags(), "-o", name, "./cmd/dep/")
			if err != nil {
				return err
			}
			f, err := os.Open(name)
			if err != nil {
				return fmt.Errorf("can't open built file for hashing: %v", err)
			}
			defer f.Close()
			h := sha256.New()
			if _, err := io.Copy(h, f); err != nil {
				return fmt.Errorf("error hashing built file: %v", err)
			}
			if err := ioutil.WriteFile(name+".sha256", h.Sum(nil), 0600); err != nil {
				return err
			}
		}
	}
	return nil
}

// Runs test coverage over all non-vendor subdirectories.  Writes to
// coverate.txt.
func Coverage() error {
	pkgs, err := pkgs()
	if err != nil {
		return err
	}
	cov, err := os.Create("coverage.txt")
	if err != nil {
		return err
	}
	defer cov.Close()
	for _, pkg := range pkgs {
		err := sh.Run("go", "test", "-race", "-coverprofile=profile.out", "-covermode=atomic", pkg)
		if err != nil {
			return err
		}
		prof, err := os.Open("profile.out")
		if err != nil {
			return err
		}
		defer prof.Close()
		if _, err := io.Copy(cov, prof); err != nil {
			return err
		}
		prof.Close()
		os.Remove("profile.out")
	}
	return nil
}

// Validates code with various linters.
func Lint() error {
	pkgs, err := pkgs()
	if err != nil {
		return err
	}
	err = sh.Run("go", append([]string{"vet"}, pkgs...)...)
	if err != nil {
		return err
	}
	err = sh.Run("golint", pkgs...)
	if err != nil {
		return err
	}
	args := []string{"-unused.exported", "-ignore", "github.com/golang/dep/internal/test/test.go:U1000 github.com/golang/dep/gps/prune.go:U1000 github.com/golang/dep/manifest.go:U1000"}
	args = append(args, pkgs...)
	return sh.Run("megacheck", args...)
}

// Validate all files are formatted.
func ChkFormat() error {
	pkgs, err := pkgs()
	if err != nil {
		return err
	}
	tld := os.Getenv("REPO_TLD")
	if tld == "" {
		tld = "github.com/golang/dep"
	}
	ignorePkgs := os.Getenv("IGNORE_PKGS")
	ignores := []string{".", "./gps"}
	if ignorePkgs != "" {
		ignores = strings.Split(ignorePkgs, string(os.PathListSeparator))
	}

	for _, pkg := range pkgs {
		rel := "." + strings.TrimPrefix(pkg, tld)
		if contains(ignores, rel) {
			continue
		}

		fmt.Println("Processing gofmt for:", pkg)
		s, err := sh.Output("gofmt", "-s", "-l", rel)
		if err != nil {
			return err
		}
		if s != "" {
			return fmt.Errorf("GO FMT FAILURE: %v", pkg)
		}
	}
	return nil
}

// Checks all *.go and *.proto file (outside the vendor directory) for a license.
func ChkLicense() error {
	copyright := []byte("copyright")

	failed := false
	// process at most 1000 files in parallel
	ch := make(chan string, 1000)
	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for file := range ch {
			wg.Add(1)
			go func(s string) {
				defer wg.Done()
				f, err := os.Open(s)
				if err != nil {
					failed = true
					fmt.Fprintf(os.Stderr, "%s: %v\n", s, err)
					return
				}
				defer f.Close()
				b, err := ioutil.ReadAll(io.LimitReader(f, 100))
				if err != nil {
					failed = true
					fmt.Fprintf(os.Stderr, "%s: %v\n", s, err)
					return
				}
				if !bytes.Contains(bytes.ToLower(b), copyright) {
					failed = true
					fmt.Println(s)
				}
			}(file)
		}
		wg.Wait()
		close(done)
	}()

	// this won't ever fail because we never return a non-skip error inside
	_ = filepath.Walk(".", func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			failed = true
			fmt.Fprintf(os.Stderr, "%s error: %v\n", path, err)
			return nil
		}
		if fi.IsDir() {
			if fi.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(fi.Name(), ".pb.go") {
			return nil
		}
		if !strings.HasSuffix(fi.Name(), ".proto") && !strings.HasSuffix(fi.Name(), ".go") {
			return nil
		}
		ch <- path
		return nil
	})
	close(ch)
	<-done
	if failed {
		return errors.New("failed license check")
	}
	return nil
}

// Checks if we changed anything with regard to dependency management
// for our repo and makes sure that it was done in a valid way.
func ChkVendor() error {

	if os.Getenv("VALIDATE_UPSTREAM") == "" {
		repo := "https://github.com/golang/dep.git"
		branch := "master"

		sh.Run("git", "fetch", "-q", repo, "refs/heads/"+branch)

		head, err := sh.Output("git", "rev-parse", "--verify", "HEAD")
		if err != nil {
			return err
		}
		upstream, err := sh.Output("git", "rev-parse", "--verify", "FETCH_HEAD")
		if err != nil {
			return err
		}

		if head != upstream {
			sh.Run("git", "diff", upstream+"..."+head)
		}

		// 	validate_diff() {
		// 		if [ "$VALIDATE_UPSTREAM" != "$VALIDATE_HEAD" ]; then
		// 			git diff "$VALIDATE_COMMIT_DIFF" "$@"
		// 		fi
		// 	}
		// fi
	}
	// IFS=$'\n'
	// files=( $(validate_diff --diff-filter=ACMR --name-only -- 'Gopkg.toml' 'Gopkg.lock' 'vendor/' || true) )
	// unset IFS

	// if [ ${#files[@]} -gt 0 ]; then
	// 	go build ./cmd/dep
	// 	./dep ensure -vendor-only
	// 	./dep prune
	// 	# Let see if the working directory is clean
	// 	diffs="$(git status --porcelain -- vendor Gopkg.toml Gopkg.lock 2>/dev/null)"
	// 	if [ "$diffs" ]; then
	// 		{
	// 			echo 'The contents of vendor differ after "dep ensure && dep prune":'
	// 			echo
	// 			echo "$diffs"
	// 			echo
	// 			echo 'Make sure these commands have been run before committing.'
	// 			echo
	// 		} >&2
	// 		false
	// 	else
	// 		echo 'Congratulations! All vendoring changes are done the right way.'
	// 	fi
	// else
	// 	echo 'No vendor changes in diff.'
	// fi
	return nil
}

// flags returns the properly formatted value for ldflags, setting the current
// date, commit hash, and tag (version).
func flags() string {
	timestamp := time.Now().Format("2006-01-02")
	hash := hash()
	tag := tag()
	if tag == "" {
		tag = "dev"
	}
	return fmt.Sprintf(`-X -s -w -X "main.commitHash=%s" -X "main.buildDate=%s" -X "main.version=%s"`, hash, timestamp, tag)
}

// tag returns the git tag for the current branch or "" if none.
func tag() string {
	s, _ := sh.Output("git", "describe", "--tags", "--dirty")
	return s
}

// hash returns the git hash for the current repo or "" if none.
func hash() string {
	hash, _ := sh.Output("git", "rev-parse", "--short", "HEAD")
	return hash
}

func pkgs() ([]string, error) {
	s, err := sh.Output("go", "list", "./...")
	if err != nil {
		return nil, err
	}
	pkgs := strings.Split(s, "\n")
	novendor := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		if strings.Contains(pkg, "/vendor/") {
			continue
		}
		novendor = append(novendor, pkg)
	}
	return novendor, nil
}

func contains(vals []string, val string) bool {
	for _, v := range vals {
		if val == v {
			return true
		}
	}
	return false
}
