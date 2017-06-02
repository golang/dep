# Dep

Linux: [![Build Status](https://travis-ci.org/golang/dep.svg?branch=master)](https://travis-ci.org/golang/dep) | Windows: [![Build status](https://ci.appveyor.com/api/projects/status/4pu2xnnrikol2gsf/branch/master?svg=true)](https://ci.appveyor.com/project/golang/dep/branch/master) | [![Code Climate](https://codeclimate.com/github/golang/dep/badges/gpa.svg)](https://codeclimate.com/github/golang/dep)

Dep is a prototype dependency management tool. It requires Go 1.7 or newer to compile.

`dep` is NOT an official tool. Yet. Check out the [Roadmap](https://github.com/golang/dep/wiki/Roadmap)!

## Current status

**Alpha**.
Functionality is known to be broken, missing or incomplete. Changes are planned
to the CLI commands soon. *It would be unwise to write scripts atop `dep` before then.*
The repository is open to solicit feedback and contributions from the community.
Please see below for feedback and contribution guidelines.

`Gopkg.toml` and `Gopkg.lock` have reached a stable structure, and it is safe to
commit them in your projects. We plan to add more to these files, but we
guarantee these changes will be backwards-compatible.

## Context

- [The Saga of Go Dependency Management](https://blog.gopheracademy.com/advent-2016/saga-go-dependency-management/)
- Official Google Docs
  - [Go Packaging Proposal Process](https://docs.google.com/document/d/18tNd8r5DV0yluCR7tPvkMTsWD_lYcRO7NhpNSDymRr8/edit)
  - [User Stories](https://docs.google.com/document/d/1wT8e8wBHMrSRHY4UF_60GCgyWGqvYye4THvaDARPySs/edit)
  - [Features](https://docs.google.com/document/d/1JNP6DgSK-c6KqveIhQk-n_HAw3hsZkL-okoleM43NgA/edit)
  - [Design Space](https://docs.google.com/document/d/1TpQlQYovCoX9FkpgsoxzdvZplghudHAiQOame30A-v8/edit)
- [Frequently Asked Questions](FAQ.md)

## Usage

Get the tool via

```sh
$ go get -u github.com/golang/dep/cmd/dep
```

Typical usage on a new repo might be

```sh
$ dep init
$ dep ensure -update
```

To update a dependency to a new version, you might run

```sh
$ dep ensure github.com/pkg/errors@^0.8.0
```

See the help text for more detailed usage instructions.

## Feedback

Feedback is greatly appreciated.
At this stage, the maintainers are most interested in feedback centered on the user experience (UX) of the tool.
Do you have workflows that the tool supports well, or doesn't support at all?
Do any of the commands have surprising effects, output, or results?
Please check the existing issues and [FAQ](FAQ.md) to see if your feedback has already been reported.
If not, please file an issue, describing what you did or wanted to do, what you expected to happen, and what actually happened.

## Contributing

Contributions are greatly appreciated.
The maintainers actively manage the issues list, and try to highlight issues suitable for newcomers.
The project follows the typical GitHub pull request model.
See [CONTRIBUTING.md](CONTRIBUTING.md) for more details.
Before starting any work, please either comment on an existing issue, or file a new one.

