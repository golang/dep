---
title: Import Path Deduction
---

Deduction is dep's algorithm for looking at an import path and determining the portion of the path that corresponds to the source root. The algorithm has a static component, by which a small set of known, popular hosts like GitHub and Bitbucket have their roots deduced:

* `github.com/golang/dep/gps` -> `github.com/golang/dep`
* `bitbucket.org/foo/bar/baz` -> `bitbucket.org/foo/bar`

The set of hosts supported by static deduction are the same as [those supported by `go get`](https://golang.org/cmd/go/#hdr-Remote_import_paths):

* GitHub
* Bitbucket
* Launchpad
* IBM DevOps Services

In addition, dep also handles [gopkg.in](http://gopkg.in) directly with static deduction because, owing to internal implementation details, it is the easiest way of also attaching filters to adapt the versioning semantics of gopkg.in import paths into dep's versioning model. This turns out fine, as gopkg.in's rules mapping rules are themselves entirely static.

If the static logic cannot identify the root for a given import path, the algorithm continues to a dynamic component: dep makes an HTTP(S) request to the import path, and a server is expected to send back the root import path embedded within the HTML response. Again, this directly emulates the behavior of `go get`.

Import path deduction is applied to all of the following:

* `import` statements found in all `.go` files
* Import paths in the [`required`](Gopkg.toml.md#required) list in `Gopkg.toml`
* `name` properties in both [`[[constraint]]`](Gopkg.toml.md#constraint) and [`[[override]]`](Gopkg.toml.md#override) stanzas in `Gopkg.toml`. This is solely for validation purposes, enforcing that these names correspond only to project/source roots.
