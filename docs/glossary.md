---
id: glossary
title: Glossary
---

dep uses some specialized terminology. Learn about it here!

* [Atom](#atom)
* [Constraint](#constraint)
* [Current Project](#current-project)
* [Deduction](#deduction)
* [Direct Dependency](#direct-dependency)
* [GPS](#gps)
* [Lock](#lock)
* [Manifest](#manifest)
* [Override](#override)
* [Project](#project)
* [Project Root](#project-root)
* [Solver](#solver)
* [Source](#source)
* [Source Root](#source-root)
* [Sync](#sync)
* [Transitive Dependency](#transitive-dependency)

---

## Atom

Atoms are a source at a particular version. In practice, this means a two-tuple of [project root](#project-root) and version, e.g. `github.com/foo/bar@master`. Atoms are primarily internal to the [solver](#solver), and the term is rarely used elsewhere.

## Constraint

Constraints have both a narrow and a looser meaning. The narrow sense refers to a [`[[constraint]]`](Gopkg.toml.md#constraint) stanza in `Gopkg.toml`. However, in some contexts, the word may be used more loosely to refer to the idea of applying rules and requirements to dependency management in general.

## Current Project

The project on which dep is operating - writing its `Gopkg.lock` and populating its `vendor` directory.

Also called the "root project."

## Deduction

Deduction is the process of determining a source root from an import path. See the reference on [import path deduction](deduction.md) for specifics.

## Direct Dependency

A project's direct dependencies are those that it imports from one or more of its packages, or includes in its [`required`](Gopkg.toml.md#required) list in `Gopkg.toml`. If each letter in `A -> B -> C -> D` represents a project, then only `B` is  `A`'s direct dependency.

## GPS

Stands for "Go packaging solver", it is [a subtree of library-style packages within dep](https://godoc.org/github.com/golang/dep/gps), and is the engine around which dep is built. Most commonly referred to as "gps." 

## Lock

A generic term, used across many language package managers, for the kind of information dep keeps in a `Gopkg.lock` file.

## Manifest

A generic term, used across many language package managers, for the kind of information dep keeps in a `Gopkg.toml` file.

## Override

An override is a [`[[override]]`](Gopkg.toml.md#override) stanza in `Gopkg.toml`. 

## Project

A project is a tree of Go packages. Projects cannot be nested. See [Project Root](#project-root) for more information about how the root of the tree is determined.

## Project Root

The root import path for a project. A project root is defined as:

* For the current project, the location of the `Gopkg.toml` file defines the project root
* For dependencies, the root of the network [source](#source) (VCS repository) is treated as the project root

These are generally one and the same, though not always. When using dep inside a monorepo, multiple `Gopkg.toml` files may exist at subpaths for discrete projects, designating each of those import paths as Project Roots. This works fine when working directly on those projects. If, however, any project not in the repository seeks to import the monorepo, dep will treat the monorepo's as one big Project, with the root directory being the Project Root; it will disregard any and all  `Gopkg.toml` files in subdirectories.

This may also be referred to as the "import root" or "root import path."

## Solver

"The solver" is a reference to the domain-specific SAT solver contained in [gps](#gps). More detail can be found on its [reference page](the-solver.md).

## Source

The remote entities that hold versioned code. Sources are specifically the entity containing the code, not any particular version of thecode itself.

"Source" is used in lieu of "VCS" because Go package management tools will soon learn to use more than just VCS systems.

## Source Root

The portion of an import path that corresponds to the network location of a source. This is similar to [Project Root](#project-root), but refers strictly to the second definition, network-oriented.

## Sync

dep's interaction model is based around the idea of maintaining a well-defined relationship between your project's import statements and `Gopkg.toml`, and your project's `Gopkg.lock` - keeping them "in sync". When the `Gopkg.lock` has more or fewer entries than are necessary, or entries that are incompatible with constraint rules established in `Gopkg.toml`, your project is "out of sync".

When your project is out of sync, running `dep ensure` will always either fail with an informative message, or bring your project back in sync.

## Transitive Dependency

A project's transitive dependencies are those dependencies that it does not import itself, but are imported by one of its dependencies. If each letter in `A -> B -> C -> D` represents a project, then `C` and `D` are  `A`'s transitive dependencies.