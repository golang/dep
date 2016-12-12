package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"testing"
)

func TestContains(t *testing.T) {
	a := []string{"a", "b", "abcd"}

	if !contains(a, "a") {
		t.Fatal("expected array to contain 'a'")
	}
	if contains(a, "d") {
		t.Fatal("expected array to not contain 'd'")
	}
}

func TestIsStdLib(t *testing.T) {
	tests := map[string]bool{
		"github.com/Sirupsen/logrus": false,
		"encoding/json":              true,
		"golang.org/x/net/context":   false,
		"net/context":                true,
		".":                          false,
	}

	for p, e := range tests {
		b := isStdLib(p)
		if b != e {
			t.Fatalf("%s: expected %t got %t", p, e, b)
		}
	}
}

func TestInit(t *testing.T) {
	needsExternalNetwork(t)
	needsGit(t)

	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("src")
	tg.setenv("GOPATH", tg.path("."))

	importPaths := map[string]string{
		"github.com/pkg/errors":      "v0.8.0",                                   // semver
		"github.com/Sirupsen/logrus": "42b84f9ec624953ecbf81a94feccb3f5935c5edf", // random sha
	}

	// checkout the specified revisions
	for ip, rev := range importPaths {
		tg.runGo("get", ip)
		repoDir := tg.path("src/" + ip)
		tg.runGit(repoDir, "checkout", rev)
	}

	// Build a fake consumer of these packages.
	const root = "github.com/golang/notexist"
	m := `package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"

	"` + root + `/foo/bar"
)

func main() {
	err := nil
	if err != nil {
		errors.Wrap(err, "thing")
	}
	logrus.Info(bar.Qux)
}`

	tg.tempFile("src/"+root+"/foo/thing.go", m)

	m = `package bar

const Qux = "yo yo!"
`
	tg.tempFile("src/"+root+"/foo/bar/bar.go", m)

	tg.cd(tg.path("src/" + root))
	tg.run("init")

	expectedManifest := `{
    "dependencies": {
        "github.com/Sirupsen/logrus": {
            "revision": "42b84f9ec624953ecbf81a94feccb3f5935c5edf"
        },
        "github.com/pkg/errors": {
            "version": ">=0.8.0, <1.0.0"
        }
    }
}
`
	manifest := tg.readManifest()
	if manifest != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	sysCommit, err := getRepoLatestCommit("golang/sys")
	tg.must(err)
	expectedLock := `{
    "projects": [
        {
            "name": "github.com/Sirupsen/logrus",
            "revision": "42b84f9ec624953ecbf81a94feccb3f5935c5edf",
            "packages": [
                "."
            ]
        },
        {
            "name": "github.com/pkg/errors",
            "version": "v0.8.0",
            "revision": "645ef00459ed84a119197bfb8d8205042c6df63d",
            "packages": [
                "."
            ]
        },
        {
            "name": "golang.org/x/sys",
            "branch": "master",
            "revision": "` + sysCommit + `",
            "packages": [
                "unix"
            ]
        }
    ]
}
`
	lock := wipeMemo(tg.readLock())
	if lock != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, lock)
	}
}

var memoRE = regexp.MustCompile(`\s+"memo": "[a-z0-9]+",`)

func wipeMemo(s string) string {
	return memoRE.ReplaceAllString(s, "")
}

type commit struct {
	Sha string `json:"sha"`
}

func getRepoLatestCommit(repo string) (string, error) {
	resp, err := http.Get(fmt.Sprintf("https://api.github.com/repos/%s/commits?per_page=1", repo))
	if err != nil {
		return "", err
	}

	var commits []commit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return "", err
	}

	if len(commits) < 1 {
		return "", errors.New("got no commits")
	}
	return commits[0].Sha, nil
}
