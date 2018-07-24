---
title: The Solver
---

At the heart of dep is a constraint solving engine - a [CDCL](https://en.wikipedia.org/wiki/Conflict-Driven_Clause_Learning)-style solver (albeit light on the "CL" part), tailored specifically to the domain of Go package management. It lives in the `github.com/golang/dep/gps` package, and is where the work of determining a valid, transitively complete dependency graph (aka, the contents of `Gopkg.lock`) is performed.

This page will eventually detail the solver's mechanics, but in the meantime, there are [docs for an older version of the solver](https://github.com/sdboyer/gps/wiki/gps-for-Contributors) that are still accurate enough to provide a rough picture of its behavior.

## Solving invariants

The solver guarantees certain invariants in every complete solution it returns. Each invariant is explored in detail later, but they can be summarized as follows:

* All rules specified in activated `[[constraint]]` stanzas in both the current project and dependency projects will be satisfied, unless superseded by a `[[override]]` stanza in the current project.
* For all import paths pointing into a given project, the version of the project selected will contain "valid" Go packages in the corresponding directory.
* If an [import comment](https://golang.org/cmd/go/#hdr-Import_path_checking) is specified by a package, any import paths addressing that package will be of the form specified in the comment.
* For any given import path, all instances of that import path will use the exact same casing.

The solver is an iterative algorithm, working its way project-by-project through possible dependency graphs. In order to select a project, it must first prove that, to the best of its current knowledge, all of the above conditions are met. When the solver cannot find a solution, failure is defined in terms of a project's version's inability to meet one of the above criteria.

### `[[constraint]]` rules

As described in the `Gopkg.toml` docs, each [`[[constraint]]`](Gopkg.toml.md#constraint) stanza is associated with a single project, and each stanza can contain both [a version rule](Gopkg.toml.md#version-rules) and a [source rule](Gopkg.toml.md#source). For any given project `P`, all dependers on `P` whose constraint rules are "activated" must express mutually compatible rules. That means:

* For version rules, all activated constraints on `P` must [intersect](<https://en.wikipedia.org/wiki/Intersection_(set_theory)>), and and there must be at least one published version must exist in the intersecting space. Intersection varies depending on version rule type:
  * For `revision` and `branch`, it must be a string-literal match.
  * For `version`, if the string is not a valid semantic version, then it must be a string-literal match.
  * For `version` that are valid semantic version ranges, intersection is standard set-theoretic intersection of the possible values in each range range. Semantic versions without ranges are treated as a single element set (e.g., `version = "=v1.0.0"`) for intersection purposes.
* For `source` rules, all projects with a particular dependency must either express a string-equal `source` value, or have no `source` value at all. This allows one dependency to specify an alternate `source`, and other dependencies to play along if they have no opinion. (NB: this play-along behavior may be removed in a future version.)

If the current project's `Gopkg.toml` has an [`[[override]]`](Gopkg.toml.md#override) on `P`, then all `[[constraint]]` declarations (including any in the current project) are ignored, obviating the possibility of conflict.

#### Activated constraints

Just because a `[[constraint]]` on `P` appears in `D`'s `Gopkg.toml` doesn't necessarily mean the constraint on `P` is considered active. A package in `P` must be imported by a package in `D` - and, if `D` is not the current project, then one of its packages importing `P` must also be imported.

Given the following dependency graph, where `C` is the current project:

```
C -> D
C -> P
D/subpkg -> P
```

Even though `C` imports `D`, because `D/subpkg` is not reachable through `C`'s imports, any `[[constraint]]` declared in `D`'s `Gopkg.toml`' on `P` will not be active.

The reasoning behind this behavior is explained further [in this gist](https://gist.github.com/sdboyer/b0813bf2b9dba58a335a85092085472f).

### Package validity

dep does only superficial validation of code in packages, but it does do some. For a package to be considered valid, three things must be true:

* There must be at least one `.go` file.
* No errors are reported from [`parser.ParseFile()`](https://golang.org/pkg/go/parser/#ParseFile) when called with [`parser.ImportsOnly|parser.ParseComments`](https://golang.org/pkg/go/parser/#Mode) on any file in the package directory.

- The package must not contain any [local imports](https://golang.org/pkg/go/build/#IsLocalImport). Note: this disallows something the standard toolchain compiler does allow, which is normally means dep must support it. However, local imports are already strongly discouraged in the toolchain, and skipping them allows dep to avoid [dot-dot hell](https://9p.io/sys/doc/lexnames.html).

If any of the above are untrue, the code in a package is considered malformed, and cannot be used in a solution.

It is not immediately disqualifying for a project to merely contain some invalid packages; they must be imported for the invariant to be broken. So, if `P/invalid` is a subpackage with invalid code in it, then it is still acceptable if `C -> P`. However, internal imports within `P` are also considered, so this import chain:

```
C -> P
P -> invalid
```

will result in an error, as `C` imports a package that will necessarily result in the import of an invalid package.

### Import comments

Go 1.4 introduced [import comments](https://golang.org/cmd/go/#hdr-Import_path_checking), which allow a package to specify the import path that must be used when addressing it. For example, `import "github.com/golang/net/dict"` would point to a valid package, but because [it uses an import comment](https://github.com/golang/net/blob/42fe2e1c20de1054d3d30f82cc9fb5b41e2e3767/dict/dict.go#L7) to enforce that it must be imported as `golang.org/x/net/dict`, dep would reject any project attempting to import it directly through its github address.

Because most projects are consistent about their import comment use over time, this issue typically only occurs when adding a new dependency or attempting to revive an older project.

> Note: dep does not currently enforce this rule, but [it needs to](https://github.com/golang/dep/issues/902).

**Remediation:** change the code by fixing the offending import paths. If the offending import paths are not in the current project and you don't directly control the dependency, you'll have to fork and fix it yourself, then use `source` to point to your fork.

### Import path casing

The standard Go toolchain compiler [does not](https://github.com/golang/go/issues/4773) [allow](https://github.com/golang/go/issues/20264) import paths that vary only in case to exist in the same build. For example, either of `github.com/sirupsen/logrus` or `github.com/Sirupsen/logrus` are fine (GitHub treats usernames as case-insensitive) individually, but they cannot exist in the same project.

The solver keeps track of the accepted case variant for each import path it's processed. Any subsequent projects it sees that introduces a case-only variation for a known import path will be rejected.

**Remediation:** Pick a casing variation (all lowercase is usually the right answer), and enforce it universally across the depgraph. As it has to be respected in all dependencies, as well, this may necessitate pull requests and possibly forking of dependencies, if you don't control them directly.
