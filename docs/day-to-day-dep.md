---
title: Day-to-day dep
---

In keeping with Go's philosophy of minimizing knobs, dep has a sparse interface: there are only two commands you're likely to run regularly. `dep ensure` is the primary workhorse command, and after the initial `dep init`, is the only thing you'll run that actually changes disk state. `dep status` is a read-only command that can help you understand the current state of your project. 

This guide primarily centers on  `dep ensure`, as that's the command you run to effect changes on your project. The [ensure mechanics](ensure-mechanics.md) document details how the command actually works, and is worth reading if you're encountering a confusing `dep ensure` behavior (or just curious!). This guide is more of a high-level tour for folks trying to get a handle on the basics of dep.

## Basics

Let's start with some semantics: the verb is "ensure" to emphasize that the action being taken is not only performing a single, discrete action (like adding a dependency), but rather enforcing a kind of broader guarantee. Expressing that guarantee In narrative terms, running `dep ensure` is like saying:

> Hey dep, please make sure that my project is [in sync](glossary.md#sync): that `Gopkg.lock` satisfies all the imports in my project, and all the rules in `Gopkg.toml`, and that `vendor/` contains exactly what `Gopkg.lock` says it should."

As the narrative indicates, `dep ensure` is a holistic operation; rather than offering a series of commands that you run in succession to incrementally achieve a some final state, each run of `dep ensure` delivers a complete, consistent final state with respect to the inputs of your project. You might think of this like a frog, hopping from lilypad to lilypad: `dep ensure` moves your project from one safe (transitively complete import graph, with all constraints satisfied, and a fully populated `vendor`) island to the the next, or it doesn't move at all. Barring critical, unknown bugs, there are no intermediate failure states. This makes `dep ensure` fine to run at most any time, as it will always drive towards a safe, known good state. 

General guidelines for using dep:

* Never directly edit anything in `vendor/`; dep will unconditionally overwrite such changes.
* `dep ensure` is almost never the wrong thing to run; if you're not sure what's going on, running it will bring you back to safety, or fail informatively.



## Using `dep ensure`

There are five basic times when you'll run `dep ensure` (with and without flags):

- We want to add a new dependency
- We want to upgdate an existing dependency
- We've imported a package for the first time, or removed the last import of a package
- We've made a change to a rule in `Gopkg.toml`
- We're not quite sure if one of the above has happened

Let's explore each of these. To play along at home, you'll need to `cd` into a project that's already managed by dep (by `dep init` - there are separate guides for [new projects](new-project.md) and [migrations](migrating.md)).

### Adding a new dependency

Let's say that we want to introduce a new dependency on  `github.com/pkg/errors`. We can accomplish this with one command:

```
$ dep ensure -add github.com/pkg/errors
```

_Much like git, `dep status` and `dep ensure` can also be run from any subdirectory of your project root, which is determined by the presence of a `Gopkg.toml` file._

This should succeed, resulting in an updated `Gopkg.lock` and `vendor/` directory, as well as injecting a best-guess version constraint for `github.com/pkg/errors` into our `Gopkg.toml`. But, it will also report a warning:

```
"github.com/pkg/errors" is not imported by your project, and has been temporarily added to Gopkg.lock and vendor/.
If you run "dep ensure" again before actually importing it, it will disappear from Gopkg.lock and vendor/.
```

As the warning suggests, you should introduce an `import "github.com/pkg/errors"` in your code, ideally right away. If you don't, a later `dep ensure` run will interpret your newly-added dependency as unused, and automatically get rid of it.

Note that it is not _required_ to use `dep ensure -add` to add new dependencies - you can also just add an appropriate `import` statement in your code, then run `dep ensure`. This approach doesn't always play nicely with  [`goimports`](https://godoc.org/golang.org/x/tools/cmd/goimports), and also won't append a `[[constraint]]` into `Gopkg.toml`. Still, it can be useful at times, often for rapid iteration and off-the-cuff experimenting.

The [ensure mechanics section on `-add`](ensure-mechanics.md#add) has more detail on internals, as well as some subtle variations in `dep ensure -add`'s behavior.

### Updating dependencies

Ideally, updating a dependency to a newer version is a single command:

```
$ dep ensure -update github.com/foo/bar
```

This also works without arguments to try to update all dependencies, though it's generally not recommended:

```
$ dep ensure -update
```

The behavior of `dep ensure -update` is heavily dependent on [the type of constraints in use](ensure-mechanics.md#update-and-constraint-types). For semver range and branch cases, the above CLI-driven approach works. For other types - non-semver releases and revisions (e.g. git hashes) - the only option to achieve an analogous "update" is to manually update `Gopkg.toml` with a new constraints, then run `dep ensure`.

### Adding and removing package imports

As described in the 

### Rule changes in `Gopkg.toml`

`Gopkg.toml` files contain five basic types of rules. The  [`Gopkg.toml` docs](#gopkg.toml.md) explain them in detail, but here's an overview:

* `required`, which are mostly equivalent to import statements in code, except it's OK to include a `main` package
* `ignored`, which causes dep to black hole an import path (and any imports it uniquely introduces)
* `[[constraint]]`, stanzas that express version constraints and some other rules on a per-project dependency basis
* `[[override]]`, stanzas identical to `[[constraint]]` except that only the current project can express them and they supersede `[[constraint]]` in both the current project and dependencies
* `[prune]`, global and per-project rules that govern what kinds of files should be removed from `vendor/`

Changes to any one of these rules will likely necessitate changes in `Gopkg.lock` and `vendor/`; a single successful `dep ensure` run will effect all such changes at once, bringing your project back in sync.

### Or, just, any time

So, you're humming along, working on a project, and 


