---
title: Installation
---

It is strongly recommended that you use a released version of dep. While tip is never purposefully broken, its stability is not guaranteed.


## Binary Installation

Pre-compiled binaries are available on the [releases](https://github.com/golang/dep/releases) page.

## MacOS

Install or upgrade to the latest released version with Homebrew:

```sh
$ brew install dep
$ brew upgrade dep
```

## Arch Linux

Install the `golang-dep` package [from the AUR](https://aur.archlinux.org/packages/golang-dep/)

```sh
git clone https://aur.archlinux.org/golang-dep.git
cd golang-dep
makepkg -si
```

## Install From Source
The snippet below installs the latest release of dep from source and
sets the version in the binary so that `dep version` works as expected.

```sh
go get -d -u github.com/golang/dep
cd $(go env GOPATH)/src/github.com/golang/dep
DEP_LATEST=$(git describe --abbrev=0 --tags)
git checkout $DEP_LATEST
go install -ldflags="-X main.version=$DEP_LATEST" ./cmd/dep
git checkout master
```

## Development

If you want to hack on dep, you can install via `go get`:

```sh
go get -u github.com/golang/dep/cmd/dep
```
Note that dep requires a functioning Go workspace and GOPATH. If you're unfamiliar with Go workspaces and GOPATH, have a look at [the language documentation](https://golang.org/doc/code.html#Organization) and get your local workspace set up. Dep's model could lead to being able to work without GOPATH, but we're not there yet.
