<p align="center"><img src="docs/assets/DigbyShadows.png" width="360"></p>
<p align="center">
  <a href="https://travis-ci.org/golang/dep"><img src="https://travis-ci.org/golang/dep.svg?branch=master" alt="Build Status"></img></a>
  <a href="https://ci.appveyor.com/project/golang/dep"><img src="https://ci.appveyor.com/api/projects/status/github/golang/dep?svg=true&branch=master&passingText=Windows%20-%20OK&failingText=Windows%20-%20failed&pendingText=Windows%20-%20pending" alt="Windows Build Status"></a>
  <a href="https://goreportcard.com/report/github.com/golang/dep"><img src="https://goreportcard.com/badge/github.com/golang/dep" /></a>
</p>

<i>Would you like to see your company name here? <a
href="https://github.com/golang/dep/issues/2165">We're looking for a stable source of
funding.</a></i>

## Dep

`dep` is a dependency management tool for Go. It requires Go 1.9 or newer to compile.

`dep` was initially developed to experiment with a new dependency management system
for Go; but, as of Go 1.11, **the Go project has officially adopted a different approach, 
based on the concept of Modules**, which is described in a [series of posts](https://blog.golang.org/using-go-modules) 
published on [blog.golang.org](https://blog.golang.org).

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

On Windows, you can install or upgrade to the latest released version with Choco

```sh
$ choco install dep
$ choco upgrade dep
```

or you can download a tarball from

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

## Feedback

Feedback is greatly appreciated.
At this stage, the maintainers are most interested in feedback centered on the user experience (UX) of the tool.
Do you have workflows that the tool supports well, or doesn't support at all?
Do any of the commands have surprising effects, output, or results?
Let us know by filing an issue, describing what you did or wanted to do, what you expected to happen, and what actually happened.

## Contributing

Contributions are greatly appreciated.
The maintainers actively manage the issues list, and try to highlight issues suitable for newcomers.
The project follows the typical GitHub pull request model.
See [CONTRIBUTING.md](CONTRIBUTING.md) for more details.
Before starting any work, please either comment on an existing issue, or file a new one.
