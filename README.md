# vsolver

`vsolver` is a specialized [SAT
solver](https://en.wikipedia.org/wiki/Boolean_satisfiability_problem),
designed as an engine for Go package management. The initial plan is
integration into [glide](https://github.com/Masterminds/glide), but
`vsolver` could be used by any tool interested in [fully
solving](www.mancoosi.org/edos/manager/) [the package management
problem](https://medium.com/@sdboyer/so-you-want-to-write-a-package-manager-4ae9c17d9527).

**NOTE - `vsolver` isn’t ready yet, but it’s getting close.**

The implementation is derived from the solver used in Dart's
[pub](https://github.com/dart-lang/pub/tree/master/lib/src/solver)
package management tool.

## Assumptions

Package management is far too complex to be assumption-less. `vsolver`
tries to keep its assumptions to the minimum, supporting as many
situations as is possible while still maintaining a predictable,
well-formed system.

* Go 1.6, or 1.5 with `GO15VENDOREXPERIMENT = 1` set. `vendor`
  directories are a requirement.
* You don't manually change what's under `vendor/`. That’s tooling’s
  job.
* A **project** concept, where projects comprise the set of Go packages
  in a rooted tree on the filesystem.  By happy (not) accident, that
  rooted tree is exactly the same set of packages covered by a `vendor/`
  directory.
* A manifest-and-lock approach to tracking project manifest data. The
  solver takes manifest (and, optionally, lock)-type data as inputs, and
  produces lock-type data as its output. Tools decide how to actually
  store this data, but these should generally be at the root of the
  project tree.

Manifests? Locks? Eeew. Yes, we also think it'd be swell if we didn't need
metadata files. We love the idea of Go packages as standalone, self-describing
code. Unfortunately, the wheels come off that idea as soon as versioning and
cross-project/repository dependencies happen. [Universe alignment is
hard](https://medium.com/@sdboyer/so-you-want-to-write-a-package-manager-4ae9c17d9527);
trying to intermix version information directly with the code would only make
matters worse.

## Arguments

Some folks are against using a solver in Go. Even the concept is repellent.
These are some of the arguments that are raised:

> "It seems complicated, and idiomatic Go things are simple!"

Complaining about this is shooting the messenger.

Selecting acceptable versions out of a big dependency graph is a [boolean
satisfiability](https://en.wikipedia.org/wiki/Boolean_satisfiability_problem)
(or SAT) problem: given all possible combinations of valid dependencies, we’re
trying to find a set that satisfies all the mutual requirements. Obviously that
requires version numbers lining up, but it can also (and `vsolver` will/does)
enforce invariants like “no import cycles” and type compatibility between
packages. All of those requirements must be rechecked *every time* we discovery
and add a new project to the graph.

SAT was one of the very first problems to be proven NP-complete. **OF COURSE
IT’S COMPLICATED**. We didn’t make it that way. Truth is, though, solvers are
an ideal way of tackling this kind of problem: it lets us walk the line between
pretending like versions don’t exist (a la `go get`) and pretending like only
one version of a dep could ever work, ever (most of the current community
tools).

> "(Tool X) uses a solver and I don't like that tool’s UX!"

Sure, there are plenty of abstruse package managers relying on SAT
solvers out there. But that doesn’t mean they ALL have to be confusing.
`vsolver`’s algorithms are artisinally handcrafted with ❤️ for Go’s
use case, and we are committed to making Go dependency management a
grokkable process.

## Features

Yes, most people will probably find most of this list incomprehensible
right now. We'll improve/add explanatory links as we go!

* [x] [Passing bestiary of tests](https://github.com/sdboyer/vsolver/issues/1)
  brought over from dart
* [x] Dependency constraints based on [SemVer](http://semver.org/),
      branches, and revisions. AKA, "all the ways you might depend on
      Go code now, but coherently organized."
* [x] Define different network addresses for a given import path
* [ ] Global project aliasing. This is a bit different than the previous.
* [ ] Bi-modal analysis (project-level and package-level)
* [ ] Specific sub-package dependencies
* [ ] Enforcing an acyclic project graph (mirroring the Go compiler's
      enforcement of an acyclic package import graph)
* [ ] On-the-fly static analysis (e.g. for incompatibility assessment,
      type escaping)
* [ ] Optional package duplication as a conflict resolution mechanism
* [ ] Faaaast, enabled by aggressive caching of project metadata
* [ ] Lock information parameterized by build tags (including, but not
      limited to, `GOOS`/`GOARCH`)
* [ ] Non-repository root and nested manifest/lock pairs

Note that these goals are not fixed - we may drop some as we continue
working. Some are also probably out of scope for the solver itself,
but still related to the solver's operation.
