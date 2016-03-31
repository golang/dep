# vsolver

`vsolver` is a specialized [SAT
solver](https://www.wikiwand.com/en/Boolean_satisfiability_problem), designed
as an engine for Go package management. The initial plan is integration into
[glide](https://github.com/Masterminds/glide), but `vsolver` could be used by
any tool interested in [fully solving](www.mancoosi.org/edos/manager/) [the
package management
problem](https://medium.com/@sdboyer/so-you-want-to-write-a-package-manager-4ae9c17d9527).

**NOTE - `vsolver` is super-extra-much not functional yet :)**

The current implementation is based heavily on the solver used in
Dart's
[pub](https://github.com/dart-lang/pub/tree/master/lib/src/solver)
package management tool. Significant changes are planned to suit Go's
particular constraints; in pursuit of those, we also may refactor to
adapt from a
[more fully general SAT-solving approach](https://github.com/openSUSE/libsolv).

## Assumptions

Package management is far too complex to be assumption-less. `vsolver`
tries to keep its assumptions to the minimum, supporting as many
situations as is possible while still maintaining a predictable,
well-formed system.

* Go 1.6, or 1.5 with `GO15VENDOREXPERIMENT = 1`. While the solver
  mostly doesn't touch vendor directories themselves, it's basically
  insane to try to solve this problem without them.
* A manifest-and-lock approach to tracking project manifest data. The
  solver takes manifest (and, optionally, lock)-type information as
  inputs, and produces lock-type information as its output. (An
  implementing tool gets to decide whether these are represented as
  one or two files).
* A **project** concept, where projects comprise the set of Go
  packages in a rooted tree on the filesystem. (Generally, the root
  should be where the manifest/lock are, but that's up to the tool.)
* You don't manually change what's under `vendor/` - leave it up to
  the `vsolver`-driven tool.

Yes, we also think it'd be swell if we didn't need metadata files. We
love the idea of Go packages as standalone, self-describing
code. Unfortunately, though, that idea goes off the rails as soon as
versioning and cross-project/repository dependencies happen, because
[universe alignment is hard](https://medium.com/@sdboyer/so-you-want-to-write-a-package-manager-4ae9c17d9527).

Some folks are against using a solver in Go - even just the concept. Their
reasons for it often include things like *"(Tool X) uses a solver and I don't
like that tool’s UX!"* or *"It seems complicated, and idiomatic Go things are
simple!"* But that’s just shooting the messenger. Dependency resolution is a
well-understood, NP-complete problem. It’s that problem that’s the enemy, not solvers.
And especially not this one! It’s a friendly solver - one that aims for
transparency in the choices it makes, and the resolution failures it
encounters.

## Features

Yes, most people will probably find most of this list incomprehensible
right now. We'll improve/add explanatory links as we go!

* [ ] Actually working/passing tests
* [x] Dependency constraints based on [SemVer](http://semver.org/),
      branches, and revisions. AKA, "all the ways you might depend on
      Go code now, but coherently organized."
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
