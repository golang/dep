---
id: glossary
title: Glossary
---

dep uses some specialized terminology. Learn about it here!

* [Atom](#atom)
* [Cache lock](#cache-lock)
* [Constraint](#constraint)
* [Current Project](#current-project)
* [Deducible](#deducible)
* [Deduction](#deduction)
* [Direct Dependency](#direct-dependency)
* [External Import](#external-import)
* [GPS](#gps)
* [Local cache](#local-cache)
* [Lock](#lock)
* [Manifest](#manifest)
* [Metadata Service](#metadata-service)
* [Override](#override)
* [Project](#project)
* [Project Root](#project-root)
* [Solver](#solver)
* [Source](#source)
* [Source Root](#source-root)
* [Sync](#sync)
* [Transitive Dependency](#transitive-dependency)
* [Vendor Verification](#vendor-verification)

---

### Atom

Atoms are a source at a particular version. In practice, this means a two-tuple of [project root](#project-root) and version, e.g. `github.com/foo/bar@master`. Atoms are primarily internal to the [solver](#solver), and the term is rarely used elsewhere.

### Cache lock

Also "cache lock file." A file, named `sm.lock`, used to ensure only a single dep process operates on the [local cache](#local-cache) at a time, as it is unsafe in dep's current design for multiple processes to access the local cache.

### Constraint

Constraints have both a narrow and a looser meaning. The narrow sense refers to a [`[[constraint]]`](Gopkg.toml.md#constraint) stanza in `Gopkg.toml`. However, in some contexts, the word may be used more loosely to refer to the idea of applying rules and requirements to dependency management in general.

### Current Project

The project on which dep is operating - writing its `Gopkg.lock` and populating its `vendor` directory.

Also called the "root project."

### Deducible

A shorthand way of referring to whether or not import path [deduction](#deduction) will return successfully for a given import path. "Undeducible" is also often used, to refer to an import path for which deduction fails.

### Deduction

Deduction is the process of determining the subset of an import path that corresponds to a source root. Some patterns are known a priori (static); others must be discovered via network requests (dynamic). See the reference on [import path deduction](deduction.md) for specifics.

### Direct Dependency

A project's direct dependencies are those that it _imports_ from one or more of its packages, or includes in its [`required`](Gopkg.toml.md#required) list in `Gopkg.toml`.

If each letter in `A -> B -> C -> D` represents a distinct project containing only a single package, and `->` indicates an import statement, then `B` is `A`'s direct dependency, whereas `C` and `D` are [transitive dependencies](#transitive-dependency) of `A`.

Dep only incorporates the `required` rules from the [current project's](#current-project) `Gopkg.toml`. Therefore, if `=>` represents `required` rather than a standard import, and `A -> B => C`, then `C` is a direct dependency of `B` _only_ when `B` is the current project. Because the `B`-to-`C` link does not exist when `A` is the current project, then `C` won't actually be in the graph at all.

### External Import

An `import` statement that points to a package in a project other than the one in which it originates. For example, an `import` in package `github.com/foo/bar` will be considered an external import if it points to anything _other_ than stdlib or `github.com/foo/bar/*`.

### GPS

Acronym for "Go packaging solver", it is [a subtree of library-style packages within dep](https://godoc.org/github.com/golang/dep/gps), and is the engine around which dep is built. Most commonly referred to as "gps."

### Local cache

dep maintains its own, pristine set of upstream sources (so, generally, git repository clones). This is kept separate from `$GOPATH/src` so that there is no obligation to maintain disk state within `$GOPATH`, as dep frequently needs to change disk state in order to do its work.

By default, the local cache lives at `$GOPATH/pkg/dep`. If you have multiple `$GOPATH` entries, dep will use whichever is the logical parent of the process' working directory. Alternatively, the location can be forced via the [`DEPCACHEDIR` environment variable](env-vars.md#depcachedir).

### Lock

A generic term, used across many language package managers, for the kind of information dep keeps in a `Gopkg.lock` file.

### Manifest

A generic term, used across many language package managers, for the kind of information dep keeps in a `Gopkg.toml` file.

### Metadata Service

An HTTP service that, when it receives an HTTP request containing a `go-get=1` in the query string, treats interprets the path portion of the request as an import path, and responds by embedding data in HTML `<meta>` tags that indicate the type and URL of of the underlying source root. This is the server-side component of dynamic [deduction](#deduction).

The behavior of metadata services is defined in the [Go documentation on remote import paths](https://golang.org/cmd/go/#hdr-Remote_import_paths).

Variously referenced as "HTTP metadata service", "`go-get` HTTP metadata service", "`go-get` service", etc.

### Override

An override is a [`[[override]]`](Gopkg.toml.md#override) stanza in `Gopkg.toml`.

### Project

A project is a tree of Go packages. Projects cannot be nested. See [Project Root](#project-root) for more information about how the root of the tree is determined.

### Project Root

The root import path for a project. A project root is defined as:

* For the current project, the location of the `Gopkg.toml` file defines the project root
* For dependencies, the root of the network [source](#source) (VCS repository) is treated as the project root

These are generally one and the same, though not always. When using dep inside a monorepo, multiple `Gopkg.toml` files may exist at subpaths for discrete projects, designating each of those import paths as Project Roots. This works fine when working directly on those projects. If, however, any project not in the repository seeks to import the monorepo, dep will treat the monorepo as one big Project, with the root directory being the Project Root; it will disregard any and all `Gopkg.toml` files in subdirectories.

This may also be referred to as the "import root" or "root import path."

### Solver

"The solver" is a reference to the domain-specific SAT solver contained in [gps](#gps). More detail can be found on its [reference page](the-solver.md).

### Source

The remote entities that hold versioned code. Sources are specifically the entity containing the code, not any particular version of the code itself.

"Source" is used in lieu of "VCS" because Go package management tools will soon learn to use more than just VCS systems.

### Source Root

The portion of an import path that corresponds to the network location of a source. This is similar to [Project Root](#project-root), but refers strictly to the second, network-oriented definition.

### Sync

Dep is designed around a well-defined relationship between four states:

1. `import` statements in `.go` files
2. `Gopkg.toml`
3. `Gopkg.lock`
4. The `vendor` directory

If any aspect of the relationship is unfulfilled (e.g., there is an `import` not reflected in `Gopkg.lock`, or a project that's missing from `vendor`), then dep considers the project to be "out of sync."

This concept is explored in detail in [ensure mechanics](ensure-mechanics.md#staying-in-sync).

### Transitive Dependency

A project's transitive dependencies are those dependencies that it does not import itself, but are imported by one of its dependencies.

If each letter in `A -> B -> C -> D` represents a distinct project containing only a single package, and `->` indicates an import statement, then `C` and `D` are `A`'s transitive dependencies, whereas `B` is a [direct dependency](#transitive-dependency) of `A`.

### Vendor Verification

Dep guarantees that `vendor/` contains exactly the expected code by hashing the contents of each project and storing the resulting [digest in Gopkg.lock](Gopkg.lock.md#digest). This digest is computed _after_ pruning rules are applied.

The digest is used to determine if the contents of `vendor/` need to be regenerated during a `dep ensure` run, and `dep check` uses it to determine whether `Gopkg.lock` and `vendor/` are in [sync](#sync). The [`noverify`](Gopkg.toml.md#noverify) list in `Gopkg.toml` can be used to bypass most of these verification behaviors.