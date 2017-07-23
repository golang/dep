# Dep

Linux: [![Build Status](https://travis-ci.org/golang/dep.svg?branch=master)](https://travis-ci.org/golang/dep) | Windows: [![Build status](https://ci.appveyor.com/api/projects/status/4pu2xnnrikol2gsf/branch/master?svg=true)](https://ci.appveyor.com/project/golang/dep/branch/master) | [![Code Climate](https://codeclimate.com/github/golang/dep/badges/gpa.svg)](https://codeclimate.com/github/golang/dep)

Dep is a prototype dependency management tool. It requires Go 1.7 or newer to compile.

`dep` is NOT an official tool. Yet. Check out the [Roadmap](https://github.com/golang/dep/wiki/Roadmap)!

## Current status

`dep` is safe for production use. That means two things:

* Any valid metadata file (`Gopkg.toml` and `Gopkg.lock`) will be readable and considered valid by any future version of `dep`.
* Generally speaking, it has comparable or fewer bugs than other tools out there.

That said, keep in mind the following:

* `dep` is still changing rapidly. If you need stability (e.g. for CI), it's best to rely on a released version, not tip.
* [Some changes](https://github.com/golang/dep/pull/489) are pending to the CLI interface. Scripting on dep before they land is unwise.
* `dep`'s exported API interface will continue to change in unpredictable, backwards-incompatible ways until we tag a v1.0.0 release.

## Context

- [The Saga of Go Dependency Management](https://blog.gopheracademy.com/advent-2016/saga-go-dependency-management/)
- Official Google Docs
  - [Go Packaging Proposal Process](https://docs.google.com/document/d/18tNd8r5DV0yluCR7tPvkMTsWD_lYcRO7NhpNSDymRr8/edit)
  - [User Stories](https://docs.google.com/document/d/1wT8e8wBHMrSRHY4UF_60GCgyWGqvYye4THvaDARPySs/edit)
  - [Features](https://docs.google.com/document/d/1JNP6DgSK-c6KqveIhQk-n_HAw3hsZkL-okoleM43NgA/edit)
  - [Design Space](https://docs.google.com/document/d/1TpQlQYovCoX9FkpgsoxzdvZplghudHAiQOame30A-v8/edit)
- [Frequently Asked Questions](docs/FAQ.md)

## Setup

Get the tool via

```sh
$ go get -u github.com/golang/dep/cmd/dep
```

To start managing dependencies using dep, run the following from your project root directory:

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
1. Generate [`Gopkg.toml`](Gopkg.toml.md) ("manifest") and `Gopkg.lock` files
1. Install the dependencies in `vendor/`

## Usage

There is one main subcommand you will use: `dep ensure`. `ensure` first makes
sure `Gopkg.lock` is consistent with your `import`s and `Gopkg.toml`. If any
changes are detected, it then populates `vendor/` with exactly what's described
in `Gopkg.lock`.

`dep ensure` is safe to run early and often. See the help text for more detailed
usage instructions.

```sh
$ dep help ensure
```

### Installing dependencies

(if your `vendor/` directory isn't [checked in with your code](docs/FAQ.md#should-i-commit-my-vendor-directory))

<!-- may change with https://github.com/golang/dep/pull/489 -->

```sh
$ dep ensure
```

If a dependency already exists in your `vendor/` folder, dep will ensure it
matches the constraints from the manifest. If the dependency is missing from
`vendor/`, the latest version allowed by your manifest will be installed.

### Adding a dependency

1. `import` the package in your `*.go` source code file(s).
1. Run the following command to update your `Gopkg.lock` and populate `vendor/` with the new dependency.

    ```sh
    $ dep ensure
    ```

### Changing dependencies

If you want to:

* Change the allowed `version`/`branch`/`revision`
* Switch to using a fork

for one or more dependencies, do the following:

1. Modify your `Gopkg.toml`.
1. Run

    ```sh
    $ dep ensure
    ```

### Checking the status of dependencies

```sh
$ dep status
PROJECT                             CONSTRAINT     VERSION        REVISION  LATEST
github.com/Masterminds/semver       branch 2.x     branch 2.x     139cc09   c2e7f6c
github.com/Masterminds/vcs          ^1.11.0        v1.11.1        3084677   3084677
github.com/armon/go-radix           *              branch master  4239b77   4239b77
```

### Updating dependencies

(to the latest version allowed by the manifest)

```sh
$ dep ensure -update
```

### Removing dependencies

1. Remove the `import`s and all usage from your code.
1. Run

    ```sh
    $ dep ensure
    ```

1. Remove from `Gopkg.toml`, if it was in there.

### Testing changes to a dependency

Making changes in your `vendor/` directory directly is not recommended, as dep
will overwrite any changes. Instead:

1. Delete the dependency from the `vendor/` directory.

    ```sh
    rm -rf vendor/<dependency>
    ```

1. Add that dependency to your `GOPATH`, if it isn't already.

    ```sh
    $ go get <dependency>
    ```

1. Modify the dependency in `$GOPATH/src/<dependency>`.
1. Test, build, etc.

Don't run `dep ensure` until you're done. `dep ensure` will reinstall the
dependency into `vendor/` based on your manifest, as if you were installing from
scratch.

This solution works for short-term use, but for something long-term, take a look
at [virtualgo](https://github.com/GetStream/vg).

To test out code that has been pushed as a new version, or to a branch or fork,
see [changing dependencies](#changing-dependencies).

## Feedback

Feedback is greatly appreciated.
At this stage, the maintainers are most interested in feedback centered on the user experience (UX) of the tool.
Do you have workflows that the tool supports well, or doesn't support at all?
Do any of the commands have surprising effects, output, or results?
Please check the existing issues and [FAQ](docs/FAQ.md) to see if your feedback has already been reported.
If not, please file an issue, describing what you did or wanted to do, what you expected to happen, and what actually happened.

## Contributing

Contributions are greatly appreciated.
The maintainers actively manage the issues list, and try to highlight issues suitable for newcomers.
The project follows the typical GitHub pull request model.
See [CONTRIBUTING.md](CONTRIBUTING.md) for more details.
Before starting any work, please either comment on an existing issue, or file a new one.
