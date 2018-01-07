---
title: Day-to-day dep
---

In keeping with Go's philosophy of minimizing knobs, dep has a sparse interface; there are only two commands you're likely to run regularly:

* `dep ensure` is the primary workhorse command, and is the only command that changes disk state.
* `dep status` reports on the state of your project, and the universe of Go dependencies.

This guide primarily centers on  `dep ensure`, as that's the command you run to effect changes on your project. The [ensure mechanics](ensure-mechanics.md) reference details how the command actually works, and is worth reading if you're encountering a confusing `dep ensure` behavior (or just curious!). This guide is more of a high-level tour for folks trying to get a basic handle on using dep effectively.

## Basics

Let's start with some semantics: the verb is "ensure" to emphasize that the action being taken is not only performing a single, discrete action (like adding a dependency), but rather enforcing a kind of broader guarantee. To put that guarantee in narrative terms, running `dep ensure` is like saying:

> Hey dep, please make sure that my project is [in sync](glossary.md#sync): that `Gopkg.lock` satisfies all the imports in my project, and all the rules in `Gopkg.toml`, and that `vendor/` contains exactly what `Gopkg.lock` says it should."

As the narrative indicates, `dep ensure` is a holistic operation; rather than offering a series of commands that you run in succession to incrementally achieve a some final state, each run of `dep ensure` delivers a complete, consistent final state with respect to the inputs of your project. It's a bit like a frog, hopping from lilypad to lilypad: `dep ensure` moves your project from one safe (transitively complete import graph, with all constraints satisfied, and a fully populated `vendor`) island to the the next, or it doesn't move at all. There are no known intermediate failure states. This makes `dep ensure` fine to run at most any time, as it will always drive towards a safe, known good state.

General guidelines for using dep:

* Never directly edit anything in `vendor/`; dep will unconditionally overwrite such changes.
* `dep ensure` is almost never the wrong thing to run; if you're not sure what's going on, running it will bring you back to safety, or fail informatively.



## Using `dep ensure`

There are four times when you'll run `dep ensure`:

- We want to add a new dependency
- We want to update an existing dependency
- We've imported a package for the first time, or removed the last import of a package
- We've made a change to a rule in `Gopkg.toml`

There's also an implicit fifth time: when you're not sure if one of the above has happened. Running `dep ensure` without any additional flags will get your project back in sync - a known good state. As such, it's generally safe to defensively run `dep ensure`  as a way of simply making sure that your project is in that state.

Let's explore each of moments. To play along, you'll need to `cd` into a project that's already been set up by `dep init`. If you haven't done that yet, check out the guides for [new projects](new-project.md) and [migrations](migrating.md).

### Adding a new dependency

Let's say that we want to introduce a new dependency on  `github.com/pkg/errors`. This can be accomplished with one command:

```
$ dep ensure -add github.com/pkg/errors
```

> Much like git, `dep status` and `dep ensure` can also be run from any subdirectory of your project root, which is determined by the presence of a `Gopkg.toml` file.

This should succeed, resulting in an updated `Gopkg.lock` and `vendor/` directory, as well as injecting a best-guess version constraint for `github.com/pkg/errors` into our `Gopkg.toml`. But, it will also report a warning:

```
"github.com/pkg/errors" is not imported by your project, and has been temporarily added to Gopkg.lock and vendor/.
If you run "dep ensure" again before actually importing it, it will disappear from Gopkg.lock and vendor/.
```

As the warning suggests, you should introduce an `import "github.com/pkg/errors"` in your code, the sooner the better. If you don't, a later `dep ensure` run will interpret your newly-added dependency as unused, and automatically remove it from `Gopkg.lock` and `vendor/`. This is because, in contrast to other dependency management tools that rely on a metadata file to indicate which dependencies are required, dep considers the import statements it discovers through static analysis of your project's code to be the canonical indicator of what dependencies must be present.

Note that you do not _have to_ use `dep ensure -add` to add new dependencies - you can also just add an appropriate `import` statement in your code, then run `dep ensure`. This approach doesn't always play nicely with  [`goimports`](https://godoc.org/golang.org/x/tools/cmd/goimports), and also won't append a `[[constraint]]` into `Gopkg.toml`. Still, it can be useful at times, often for rapid iteration and off-the-cuff experimenting.

The [ensure mechanics section on `-add`](ensure-mechanics.md#add) has more detail on internals, as well as some subtle variations in `dep ensure -add`'s behavior.

### Updating dependencies

Ideally, updating a dependency project to a newer version is a single command:

```
$ dep ensure -update github.com/foo/bar
```

This also works without arguments to try to update all dependencies, though it's generally not recommended:

```
$ dep ensure -update
```

`dep ensure -update` searches for versions that work with the `branch`, `version`, or `revision` constraint defined in `Gopkg.toml`. These constraint types have different semantics, some of which allow `dep ensure -update` to effectively find a "newer" version, while others will necessitate hand-updating the `Gopkg.toml`. The [ensure mechanics](ensure-mechanics.md#update-and-constraint-types) guide explains this in greater detail, but if you want to know what effect a `dep ensure -update` is likely to have for a particular project, the `LATEST` field in `dep status` output will tell you.

### Adding and removing `import` statements

As noted in [the section on adding dependencies](#adding-a-new-dependency), dep relies on the import statements in your code to figure out which dependencies your project actually needs. Thus, when you add or remove import statements, dep might need to care about it.

It's only "might," though, because most of the time, adding or removing imports doesn't matter to dep. Only if one of the following has occurred will a `dep ensure` be necessary to bring the project back in sync:

1. You've added the first `import` of a package, but already `import` other packages from that project.
2. You've removed the last `import` of a package, but still `import` other packages from that project.
3. You've added the first `import` of any package within a particular project. (Note: this is the [alternate adding approach](#adding-a-new-dependency))
4. You've removed the last `import` of a package from within a particular project.

In short, dep is concerned with the set of unique import paths across your entire project, and only cares when you make a change that adds or removes an import path from that set.

Of course, especially on large projects, it can be tough to keep track of whether adding or removing (especially removing) a particular import statement actually does change the overall set. Fortunately, you needn't keep close track, as you can run `dep ensure` and it will automatically pick up any additions or removals, and bring your project back in sync.

Only if it is the first/last import of a project being added/removed - cases 3 and 4 - are additional steps needed: `Gopkg.toml` should be updated to add/remove the corresponding project's `[[constraint]]`.

### Rule changes in `Gopkg.toml`

`Gopkg.toml` files contain five basic types of rules. The  [`Gopkg.toml` docs](#gopkg.toml.md) explain them in detail, but here's an overview:

* `required`, which are mostly equivalent to import statements in code, except it's OK to include a `main` package
* `ignored`, which causes dep to black hole an import path (and any imports it uniquely introduces)
* `[[constraint]]`, stanzas that express version constraints and some other rules on a per-project dependency basis
* `[[override]]`, stanzas identical to `[[constraint]]` except that only the current project can express them and they supersede `[[constraint]]` in both the current project and dependencies
* `[prune]`, global and per-project rules that govern what kinds of files should be removed from `vendor/`

Changes to any one of these rules will likely necessitate changes in `Gopkg.lock` and `vendor/`; a single successful `dep ensure` run will incorporate all such changes at once, bringing your project back in sync.


