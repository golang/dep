---
title: Setup
menu: main
weight: 2
---

Grab the latest binary from the [releases](https://github.com/golang/dep/releases) page.

On macOS you can install or upgrade to the latest released version with Homebrew:

```sh
$ brew install dep
$ brew upgrade dep
```

If you're interested in hacking on `dep`, you can install via `go get`:

```sh
go get -u github.com/golang/dep/cmd/dep
```

To start managing dependencies using dep, run the following from your project's root directory:


```sh
$ dep init
```

This does the following:

1. Look for [existing dependency management files](docs/FAQ.md#what-external-tools-are-supported) to convert
1. Check if your dependencies use dep
1. Identify your dependencies
1. Back up your existing `vendor/` directory (if you have one) to
`_vendor-TIMESTAMP/`
1. Pick the highest compatible version for each dependency
1. Generate [`Gopkg.toml`](docs/Gopkg.toml.md) ("manifest") and `Gopkg.lock` files
1. Install the dependencies in `vendor/`
