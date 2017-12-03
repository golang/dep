---
title: dep
type: homepage
menu: main
weight: 1
---

This is a prototype dependency management tool for Go. It requires Go 1.8 or newer to compile.

The dep project is the official _experiment_, but not yet the official tool. Check out the [Roadmap](https://github.com/golang/dep/wiki/Roadmap) for more on what this means!

## Current status

It is safe to use dep in production. That means two things:

* Any valid metadata file (`Gopkg.toml` and `Gopkg.lock`) will be readable and considered valid by any future version of `dep`.
* The CLI UI is mostly stable. `dep init` and `dep ensure` are mostly set; `dep status` is likely to change a fair bit, and `dep prune` is [going to be absorbed into `dep ensure`](https://github.com/golang/dep/issues/944).

That said, keep in mind the following:

* `dep init` on an existing project can be a rocky experience - we try to automatically convert from other tools' metadata files, and that process is often complex and murky. Once your project is converted and you're using `dep ensure`, its behavior is quite stable.
* `dep` still has nasty bugs, but in general these are comparable to or fewer than other tools out there.
* `dep` is [pretty slow right now](https://github.com/golang/dep/blob/master/docs/FAQ.md#why-is-dep-slow), especially on the first couple times you run it. Just know that there is a _lot_ of headroom for improvement, and work is actively underway.
* `dep` is still changing rapidly. If you need stability (e.g. for CI), it's best to rely on a released version, not tip.
* `dep`'s exported API interface will continue to change in unpredictable, backwards-incompatible ways until we tag a v1.0.0 release.

## Context

- [The Saga of Go Dependency Management](https://blog.gopheracademy.com/advent-2016/saga-go-dependency-management/)
- Official Google Docs
  - [Go Packaging Proposal Process](https://docs.google.com/document/d/18tNd8r5DV0yluCR7tPvkMTsWD_lYcRO7NhpNSDymRr8/edit)
  - [User Stories](https://docs.google.com/document/d/1wT8e8wBHMrSRHY4UF_60GCgyWGqvYye4THvaDARPySs/edit)
  - [Features](https://docs.google.com/document/d/1JNP6DgSK-c6KqveIhQk-n_HAw3hsZkL-okoleM43NgA/edit)
  - [Design Space](https://docs.google.com/document/d/1TpQlQYovCoX9FkpgsoxzdvZplghudHAiQOame30A-v8/edit)
- [Frequently Asked Questions](docs/FAQ.md)

# Semantic Versioning

`dep ensure` uses an external [semver library](https://github.com/Masterminds/semver) to interpret the version constraints you specify in the manifest. The comparison operators are:

* `=`: equal
* `!=`: not equal
* `>`: greater than
* `<`: less than
* `>=`: greater than or equal to
* `<=`: less than or equal to
* `-`: literal range. Eg: 1.2 - 1.4.5 is equivalent to >= 1.2, <= 1.4.5
* `~`: minor range. Eg: ~1.2.3 is equivalent to >= 1.2.3, < 1.3.0
* `^`: major range. Eg: ^1.2.3 is equivalent to >= 1.2.3, < 2.0.0
* `[xX*]`: wildcard. Eg: 1.2.x is equivalent to >= 1.2.0, < 1.3.0

You might, for example, include a constraint in your manifest that specifies `version = "=2.0.0"` to pin a dependency to version 2.0.0, or constrain to minor releases with: `version = "2.*"`. Refer to the [semver library](https://github.com/Masterminds/semver) documentation for more info.

**Note**: When you specify a version *without an operator*, `dep` automatically uses the `^` operator by default. `dep ensure` will interpret the given version as the min-boundry of a range, for example:

* `1.2.3` becomes the range `>=1.2.3, <2.0.0`
* `0.2.3` becomes the range `>=0.2.3, <0.3.0`
* `0.0.3` becomes the range `>=0.0.3, <0.1.0`

# Feedback

Feedback is greatly appreciated.
At this stage, the maintainers are most interested in feedback centered on the user experience (UX) of the tool.
Do you have workflows that the tool supports well, or doesn't support at all?
Do any of the commands have surprising effects, output, or results?
Please check the existing issues and [FAQ](docs/FAQ.md) to see if your feedback has already been reported.
If not, please file an issue, describing what you did or wanted to do, what you expected to happen, and what actually happened.

# Contributing

Contributions are greatly appreciated.
The maintainers actively manage the issues list, and try to highlight issues suitable for newcomers.
The project follows the typical GitHub pull request model.
See [CONTRIBUTING.md](CONTRIBUTING.md) for more details.
Before starting any work, please either comment on an existing issue, or file a new one.
