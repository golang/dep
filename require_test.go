package main

import "testing"

func TestRequire(t *testing.T) {
	needsExternalNetwork(t)
	needsGit(t)

	tg := testgo(t)
	defer tg.cleanup()

	tg.tempDir("src")
	tg.setenv("GOPATH", tg.path("."))

	m := `package main

import (
	"fmt"
)

func main() {
	fmt.Println("hello world")
}`

	tg.tempFile("src/thing/thing.go", m)
	tg.cd(tg.path("src/thing"))

	tg.run("init")
	tg.run("require", "github.com/jessfraz/weather/geocode")
	tg.run("require", "github.com/jessfraz/reg/registry")

	// manifest should not show the dependency as required
	expectedManifest := `{
    "required": [
        "github.com/jessfraz/weather/geocode",
        "github.com/jessfraz/reg/registry"
    ]
}
`

	manifest := tg.readManifest()
	if manifest != expectedManifest {
		t.Fatalf("expected %s, got %s", expectedManifest, manifest)
	}

	tg.mustExist(tg.path("src/thing/vendor/github.com/jessfraz/weather/geocode"))
	tg.mustExist(tg.path("src/thing/vendor/github.com/jessfraz/reg/registry"))

	expectedLock := `{
    "memo": "8eafbe7fd7b5a490d309f25ec3e4acd995806e4cea557452bfafeedaf0e1fb5a",
    "projects": [
        {
            "name": "github.com/Sirupsen/logrus",
            "version": "v0.11.0",
            "revision": "d26492970760ca5d33129d2d799e34be5c4782eb",
            "packages": [
                "."
            ]
        },
        {
            "name": "github.com/docker/distribution",
            "version": "v2.6.0",
            "revision": "325b0804fef3a66309d962357aac3c2ce3f4d329",
            "packages": [
                "digest",
                "manifest/schema1",
                "manifest/schema2"
            ]
        },
        {
            "name": "github.com/docker/engine-api",
            "version": "v0.4.0",
            "revision": "3d1601b9d2436a70b0dfc045a23f6503d19195df",
            "packages": [
                "types"
            ]
        },
        {
            "name": "github.com/docker/go-connections",
            "version": "v0.2.1",
            "revision": "990a1a1a70b0da4c4cb70e117971a4f0babfbf1a",
            "packages": [
                "nat"
            ]
        },
        {
            "name": "github.com/docker/go-units",
            "version": "v0.3.1",
            "revision": "f2d77a61e3c169b43402a0a1e84f06daf29b8190",
            "packages": [
                "."
            ]
        },
        {
            "name": "github.com/docker/libtrust",
            "branch": "master",
            "revision": "aabc10ec26b754e797f9028f4589c5b7bd90dc20",
            "packages": [
                "."
            ]
        },
        {
            "name": "github.com/gorilla/context",
            "version": "v1.1",
            "revision": "1ea25387ff6f684839d82767c1733ff4d4d15d0a",
            "packages": [
                "."
            ]
        },
        {
            "name": "github.com/gorilla/mux",
            "version": "v1.3.0",
            "revision": "392c28fe23e1c45ddba891b0320b3b5df220beea",
            "packages": [
                "."
            ]
        },
        {
            "name": "github.com/jessfraz/reg",
            "branch": "master",
            "revision": "7d3217e55266e66c19943e99847b0ab56568117b",
            "packages": [
                "registry"
            ]
        },
        {
            "name": "github.com/jessfraz/weather",
            "version": "v0.9.1",
            "revision": "a69f1b14ff98663e524100cdd164e5b9ecc306d9",
            "packages": [
                "geocode"
            ]
        },
        {
            "name": "github.com/peterhellberg/link",
            "version": "v1.0.0",
            "revision": "d1cebc7ea14a5fc0de7cb4a45acae773161642c6",
            "packages": [
                "."
            ]
        },
        {
            "name": "golang.org/x/net",
            "branch": "master",
            "revision": "f2499483f923065a842d38eb4c7f1927e6fc6e6d",
            "packages": [
                "context"
            ]
        },
        {
            "name": "golang.org/x/sys",
            "branch": "master",
            "revision": "d75a52659825e75fff6158388dddc6a5b04f9ba5",
            "packages": [
                "unix"
            ]
        }
    ]
}
`
	lock := tg.readLock()
	if lock != expectedLock {
		t.Fatalf("expected %s, got %s", expectedLock, lock)
	}
}
