---
title: Import path deduction
---

Deduction is dep's algorithm for looking at an import path and determining the portion of the path that corresponds to the source root. The algorithm has a static component, by which a small set of known, popular hosts like GitHub and Bitbucket have their roots deduced:

- `github.com/golang/dep/gps` -> `github.com/golang/dep`
- `bitbucket.org/foo/bar/baz` -> `bitbucket.org/foo/bar`

The set of hosts supported by static deduction are the same as [those supported by `go get`]().

If the static logic cannot identify the root for a given import path, the algorithm continues to a dynamic component, where it makes an HTTP(S) request to the import path, and a server is expected to return a response that indicates the root import path. This, again, is equivalent to the behavior of `go get`.