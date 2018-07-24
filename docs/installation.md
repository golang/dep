---
title: Installation
---

It is strongly recommended that you use a released version of dep. While tip is never purposefully broken, its stability is not guaranteed.

## Binary Installation

Pre-compiled binaries are available on the [releases](https://github.com/golang/dep/releases) page. You can use the `install.sh` script to automatically install one for your local platform:

```sh
$ curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
```

## MacOS

Install or upgrade to the latest released version with Homebrew:

```sh
$ brew install dep
$ brew upgrade dep
```

## Arch Linux

Install the `dep` package:

```sh
pacman -S dep
```

## Install From Source

The snippet below installs the latest release of dep from source and sets the
version in the binary so that `dep version` works as expected.

Note that this approach is not recommended for general use. We don't try to
break tip, but we also don't guarantee its stability. At the same time, we love
our users who are willing to be experimental and provide us with fast feedback!

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

## Uninstalling

Looking for a way to uninstall `dep`? There's a separate [doc page](uninstalling.md) for that!
