# Dep

Linux: [![Build Status](https://travis-ci.org/golang/dep.svg?branch=master)](https://travis-ci.org/golang/dep) | Windows: [![Build status](https://ci.appveyor.com/api/projects/status/4pu2xnnrikol2gsf/branch/master?svg=true)](https://ci.appveyor.com/project/golang/dep/branch/master)

Dep is a prototype dependency management tool. It requires Go 1.7 or newer to compile.

### Dep is NOT an official tool. It is not (yet) blessed by the Go team.

It IS, however, the consensus effort of most of the Go community, and being integrated into the `go` toolchain is the goal.

We're working on a roadmap with more details.

## Current status

**Pre-alpha**.
Lots of functionality is knowingly missing or broken.
The repository is open to solicit feedback and contributions from the community.
Please see below for feedback and contribution guidelines.

## Context

- [The Saga of Go Dependency Management](https://blog.gopheracademy.com/advent-2016/saga-go-dependency-management/)
- Official Google Docs
  - [Go Packaging Proposal Process](https://docs.google.com/document/d/18tNd8r5DV0yluCR7tPvkMTsWD_lYcRO7NhpNSDymRr8/edit)
  - [User Stories](https://docs.google.com/document/d/1wT8e8wBHMrSRHY4UF_60GCgyWGqvYye4THvaDARPySs/edit)
  - [Features](https://docs.google.com/document/d/1JNP6DgSK-c6KqveIhQk-n_HAw3hsZkL-okoleM43NgA/edit)
  - [Design Space](https://docs.google.com/document/d/1TpQlQYovCoX9FkpgsoxzdvZplghudHAiQOame30A-v8/edit)

## Usage

Get the tool via

```sh
$ go get -u github.com/golang/dep/...
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

See the help text for much more detailed usage instructions.

Note that **the manifest and lock file formats are not finalized**, and will likely change before the tool is released.
We make no compatibility guarantees for the time being.
Please don't commit any code or files created with the tool.

## Feedback

Feedback is greatly appreciated.
At this stage, the maintainers are most interested in feedback centered on the user experience (UX) of the tool.
Do you have workflows that the tool supports well, or doesn't support at all?
Do any of the commands have surprising effects, output, or results?
Please check the existing issues to see if your feedback has already been reported.
If not, please file an issue, describing what you did or wanted to do, what you expected to happen, and what actually happened.

## Contributing

Contributions are greatly appreciated.
The maintainers actively manage the issues list, and try to highlight issues suitable for newcomers.
The project follows the typical GitHub pull request model.
See [CONTRIBUTING.md](CONTRIBUTING.md) for more details.
Before starting any work, please either comment on an existing issue, or file a new one.

