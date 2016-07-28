# gps
![map-marker-icon copy](https://cloud.githubusercontent.com/assets/21599/16779217/4f5cdc6c-483f-11e6-9de3-661f13d9b215.png)
--

[![CircleCI](https://circleci.com/gh/sdboyer/gps.svg?style=svg)](https://circleci.com/gh/sdboyer/gps) [![Go Report Card](https://goreportcard.com/badge/github.com/sdboyer/gps)](https://goreportcard.com/report/github.com/sdboyer/gps) [![GoDoc](https://godoc.org/github.com/sdboyer/gps?status.svg)](https://godoc.org/github.com/sdboyer/gps)

`gps` is the Go Packaging Solver. It is an engine for tackling dependency
management problems in Go. It is trivial - [about 35 lines of
code](https://github.com/sdboyer/gps/blob/master/example.go) - to replicate the
fetching bits of `go get` using `gps`.

`gps` is _not_ Yet Another Go Package Management Tool. Rather, it's a library
that package management (and adjacent) tools can use to solve the
[hard](https://en.wikipedia.org/wiki/Boolean_satisfiability_problem) parts of
the problem in a consistent,
[holistic](https://medium.com/@sdboyer/so-you-want-to-write-a-package-manager-4ae9c17d9527)
way. It is a distillation of the ideas behind language package managers like
[bundler](http://bundler.io), [npm](https://www.npmjs.com/),
[elm-package](https://github.com/elm-lang/elm-package),
[cargo](https://crates.io/) (and others) into a library, artisanally
handcrafted with ❤️ for Go's specific requirements.

`gps` is [on track](https://github.com/Masterminds/glide/pull/384) to become the engine behind [glide](https://glide.sh).

The wiki has a [general introduction to the `gps`
approach](https://github.com/sdboyer/gps/wiki/Introduction-to-gps), as well
as guides for folks [implementing
tools](https://github.com/sdboyer/gps/wiki/gps-for-Implementors) or [looking
to contribute](https://github.com/sdboyer/gps/wiki/gps-for-Contributors).

**`gps` is progressing rapidly, but still in beta, with a concomitantly liberal sprinkling of panics.**

## Wait...a package management _library_?!

Yup. See [the rationale](https://github.com/sdboyer/gps/wiki/Rationale).

## Features

A feature list for a package management library is a bit different than one for
a package management tool. Instead of listing the things an end-user can do,
we list the choices a tool *can* make and offer, in some form, to its users, as
well as the non-choices/assumptions/constraints that `gps` imposes on a tool.

### Non-Choices

We'd love for `gps`'s non-choices to be noncontroversial. But that's not always
the case.

Nevertheless, these non-choices remain because, taken as a whole, they make
experiments and discussion around Go package management coherent and
productive.

* Go >=1.6, or 1.5 with `GO15VENDOREXPERIMENT = 1` set
* Everything under `vendor/` is volatile and controlled solely by the tool
* A central cache of repositories is used (cannot be `GOPATH`)
* A [**project**](https://godoc.org/github.com/sdboyer/gps#ProjectRoot) concept:
  a tree of packages, all covered by one `vendor` directory
* A [**manifest** and
  **lock**](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#manifests-and-locks)
  approach to tracking version and constraint information
* Source repositories can be `git`, `bzr`, `hg` or `svn` (Most of the work here is through a [separate lib](https://github.com/Masterminds/vcs))
* What the available versions are for a given project/repository (all branches, tags, or revs are eligible)
  * In general, semver tags are preferred to plain tags, are preferred to branches
* The actual packages required (determined through import graph static analysis)
  * How the import graph is statically analyzed (Similar to `go/build`, but with a combinatorial view of build tags)
* Package import cycles are not allowed ([not yet implemented](https://github.com/sdboyer/gps/issues/66))

There are also some current non-choices that we would like to push into the realm of choice:

* Different versions of packages from the same repository cannot be used
* Importable projects that are not bound to the repository root

### Choices

These choices represent many of the ways that `gps`-based tools could
substantively differ from each other.

Some of these are choices designed to encompass all options for topics on which
reasonable people have disagreed. Others are simply important controls that no
general library could know _a priori_.

* How to store manifest and lock information (file(s)? a db?)
* Which of the other package managers to interoperate with
* Which types of version constraints to allow the user to specify (e.g., allowing [semver ranges](https://docs.npmjs.com/misc/semver) or not)
* Whether or not to strip nested `vendor` directories
* Which packages in the import graph to [ignore](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#ignoring-packages) (if any)
* What constraint [overrides](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#overrides) to apply (if any)
* What [informational output](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#trace-and-tracelogger) to show the end user
* What dependency version constraints are declared by the [root project](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#manifest-data)
* What dependency version constraints are declared by [all dependencies](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#the-projectanalyzer)
* Given a [previous solution](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#lock-data), [which versions to let change, and how](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#tochange-changeall-and-downgrade)
  * In the absence of a previous solution, whether or not to use [preferred versions](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#preferred-versions)
* Allowing, or not, the user to [swap in different network names](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#projectidentifier) for import paths (e.g. forks)
* Specifying additional input/source packages not reachable from the root import graph ([not complete](https://github.com/sdboyer/gps/issues/42))

This list may not be exhaustive - see the
[implementor's guide](https://github.com/sdboyer/gps/wiki/gps-for-Implementors)
for a proper treatment.

## Contributing

Yay, contributing! Please see
[CONTRIBUTING.md](https://github.com/sdboyer/gps/blob/master/CONTRIBUTING.md).
Note that `gps` also abides by a [Code of
Conduct](https://github.com/sdboyer/gps/blob/master/CODE_OF_CONDUCT.md), and is MIT-licensed.
