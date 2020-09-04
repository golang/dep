<p align="center"><img src="docs/assets/DigbyShadows.png" width="360"></p>
<p align="center">
  <a href="https://travis-ci.org/golang/dep"><img src="https://travis-ci.org/golang/dep.svg?branch=master" alt="Build Status"></img></a>
  <a href="https://ci.appveyor.com/project/golang/dep"><img src="https://ci.appveyor.com/api/projects/status/github/golang/dep?svg=true&branch=master&passingText=Windows%20-%20OK&failingText=Windows%20-%20failed&pendingText=Windows%20-%20pending" alt="Windows Build Status"></a>
  <a href="https://goreportcard.com/report/github.com/golang/dep"><img src="https://goreportcard.com/badge/github.com/golang/dep" /></a>
</p>

## Dep

`dep` is a dependency management tool for Go. It requires Go 1.9 or newer to compile.

**NOTE:** Dep was an official experiment to implement a package manager for Go.
As of 2020, Dep is deprecated and archived in favor of Go modules, which have
had official support since Go 1.11. For more details, see https://golang.org/ref/mod.

For guides and reference materials about `dep`, see [the documentation](https://golang.github.io/dep).

## Installation

You should use an officially released version. Release binaries are available on
the [releases](https://github.com/golang/dep/releases) page.

On MacOS you can install or upgrade to the latest released version with Homebrew:

```sh
$ brew install dep
$ brew upgrade dep
```

On Debian platforms you can install or upgrade to the latest version with apt-get:

```sh
$ sudo apt-get install go-dep
```

On Windows, you can download a tarball from
[go.equinox.io](https://go.equinox.io/github.com/golang/dep/cmd/dep).

On other platforms you can use the `install.sh` script:

```sh
$ curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
```

It will install into your `$GOPATH/bin` directory by default or any other directory you specify using the `INSTALL_DIRECTORY` environment variable.

If your platform is not supported, you'll need to build it manually or let the team know and we'll consider adding your platform
to the release builds.

If you're interested in getting the source code, or hacking on `dep`, you can
install via `go get`:

```sh
go get -u github.com/golang/dep/cmd/dep
```
