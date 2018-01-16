---
title: Import path deduction
---

Deduction is dep's algorithm for looking at an import path and determining the portion of the path that corresponds to the source root. The algorithm has a static component, by which a small set of known, popular hosts like GitHub and Bitbucket have their roots deduced:

- `github.com/golang/dep/gps` -> `github.com/golang/dep`
- `bitbucket.org/foo/bar/baz` -> `bitbucket.org/foo/bar`

The set of hosts supported by static deduction are the same as [those supported by `go get`]().

If the static logic cannot identify the root for a given import path, the algorithm continues to a dynamic component, where it makes an HTTP(S) request to the import path, and a server is expected to return a response that indicates the root import path. This, again, is equivalent to the behavior of `go get`.



Import path deduction is applied to all of the following:

* `import` statements found in all `.go` files
* Import paths named in the [`required`](gopkg.toml.md#required) property in `Gopkg.toml`
* `name` properties in both [`[[constraint]]`](Gopkg.toml.md#constraint) and [`[[override]]`](Gopkg.toml.md#override) stanzas in `Gopkg.toml`. This is solely for validation purposes, enforcing that these names correspond strictly to source roots.




The results of import path deduction are, in practice, almost entirely fixed; it's easy to imagine why. In the public ecosystem, even dynamic deductions rarely change in practice, as it would either require:

- a `go-get` metadata service to intentionally change its mappings, or
- a `go-get` metadata service to disappear.

`go get` itself is only partially resilient to these cases, but each is potentially catastrophic for a package's retrievability across the ecosystem, sooner rather than later. This steep and abrupt downside makes it nearly impossible for projects only accessible via an unreliable metadata service to ever become popular or widely used in the ecosystem. Thus, in the public ecosystem, we almost only ever see reliable, well-behaved services.










Because deduction has a dynamic component, the deducibility of any given path necessarily cannot be fixed. However, because the 

## Static deduction