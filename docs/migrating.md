---
title: Migrating to Dep
---

Ideally, migrating an existing Go project to dep is straightforward:

```bash
$ cd $GOPATH/src/path/to/project/root
$ dep init
```

For many projects, this will just work. `dep init` will make educated guesses about what versions to use for your dependencies, generate sane `Gopkg.toml`, `Gopkg.lock`, and `vendor/`, and if your tests pass and builds work, then you're probably done. (If so, congratulations! You should check out [Daily Dep](daily-dep.md) next.)

The migration process is still difficult for some projects. If you're trying dep for the first time, this can be particularly frustrating, as you're trying to simultaneously learn how to use dep, and how your project _should_ be managed in dep. The good news is, `dep init` is usually the big difficulty hump; once you're over it, things get much easier.

The goal of this guide is to provide enough information for you to reason about what's happening during `dep init`, so that you can at least understand what class of problems you're encountering, and what steps you might take to address them. To that end, we'll start with an overview of what `dep init` is doing.

> Note: the first run of `dep init` can take quite a long time, as dep is creating fresh clones of all your dependencies into a special location, `$GOPATH/pkg/dep/sources/`. This is necessary for dep's normal operations, and is largely a one-time cost.

## `dep init` mechanics

When migrating existing projects, the primary goal of `dep init` is to automate as much of the work of creating a `Gopkg.toml` as possible. This is necessarily a heuristic goal, as dep may not have a 1:1 correspondence for everything you may have done before. As such, it's important to only expect that `dep init`'s automated migrations are operating on a best-effort basis.

The behavior of `dep init` varies depending on what's in your existing codebase, and the flags that are passed to it. However, it always proceeds in two phases:

1.  _Inference phase:_ Infer, from various sources, rules and hints about which versions of dependencies to use.
2.  _Solving phase:_ Work out a solution that is acceptable under dep's model, while incorporating the above inferences as much as possible.

### The Inference Phase

The inference phase is where `dep init`'s behavior varies. By default, `dep init` will look in your codebase for metadata files from [other Go package management tools that it understands](https://github.com/golang/dep/tree/master/internal/importers), and attempt to automatically migrate the data in these files into concepts that make sense in a dep. Depending on the tool and the particular values dep finds, metadata from the tool may be treated as either:

* A hint: information that dep will try to honor in the solving phase, but will discard if it cannot find a solution that respects the hint.
* A rule: information that must obeyed in the solving phase, and will ultimately appear in `Gopkg.toml` as a `[[constraint]]`. If the solving phase cannot find a solution that satisfies the rules, it will fail with an informative message.

There are three circumstances that can lead dep not to make any tool-based inferences:

* Your project doesn't use a package management tool
* dep doesn't yet support the tool you use yet
* You tell it not to, by running `dep init -skip-tools`

After tool-based inference is complete, dep will normally proceed to the solving phase. However, if the user passes the `-gopath` flag, dep will first try to fill in any holes in the inferences drawn from tool metadata by checking the current project's containing GOPATH. Only hints are gleaned from GOPATH, and they will never supersede inferences from tool metadata. If you want to put GOPATH fully in charge, pass both flags: `dep init -skip-tools -gopath`.

Once dep has compiled its set of inferences, it proceeds to solving.

### The Solving Phase

Once the inference phase is completed, the set of rules and hints dep has assembled will be passed to its [solver](the-solver.md) to work out a transitively complete depgraph, which will ultimately be recorded as the `Gopkg.lock`. This is the same solving process used by `dep ensure`, and completing it successfully means that dep has found a combination of dependency versions that respects all inferred rules, and as many inferred hints as possible. If solving succeeds, then the hard work is done; most of what remains is writing out `Gopkg.toml`, `Gopkg.lock`, and `vendor/`.

The solver returns a solution, which itself is just [a representation](https://godoc.org/github.com/golang/dep/gps#Solution) of [the data stored in a `Gopkg.lock`](https://godoc.org/github.com/golang/dep#Lock): a transitively-complete, reproducible snapshot of the entire dependency graph. Writing out the `Gopkg.lock` from a solution is little more than a copy-and-encode operation, and writing `vendor/` is a matter of placing each project listed in the solution into its appropriate place, at the designated revision. This is exactly the same as `dep ensure`'s behavior.

`Gopkg.toml` is a little different. There's no guarantee that rules were inferred for all (or even any) of your project's dependencies, but we still want to populate `Gopkg.toml` with sane values. So, for any dependency for which a rule was not inferred, dep inspects the solution to see what version was ultimately selected, and creates a constraint based on that:

* If a branch, like `master`, was picked in the solution, then `branch: "master"` will appear in `Gopkg.toml`.
* If a semantic version-compliant version was selected, like `v1.2.0`, then that will be specified as a minimum version: `version: "v1.2.0"`.
* If only a raw revision was selected, nothing will be put in `Gopkg.toml`. While dep does allow `revision: "…"` constraints in `Gopkg.toml`, use of them is considered an antipattern, so dep does not create them automatically in order to avoid implicitly encouraging their use.

## Dealing with failures

First and foremost, make sure that you're running `dep init` with the `-v` flag. That will provide a lot more information.

`dep init`, like dep in general, has both hard and soft failure modes. Hard failures result in the process hanging or aborting entirely, without anything being written to disk. Soft failures may or may not include warnings, but do ultimately write out a `Gopkg.toml`, `Gopkg.lock`, and `vendor/` - just, not the ones you wanted. Before we dig into those, though, let's set some context.

While dep contributors have invested enormous effort into creating automated migration paths into dep, these paths will always best-effort and imprecise. It's simply not always possible to convert from other tools or GOPATH with full fidelity. dep is an opinionated tool, with a correspondingly opinionated model, and that model does sometimes fundamentally differ from that of other tools. Sometimes these model mismatches result in hard failures, sometimes soft, and sometimes there's no harm at all.

Because these are deep assumptions, their symptoms can be varied and surprising. Keeping these assumptions in mind could save you some hair-pulling later on.

* dep does not allow nested `vendor/` directories; it flattens all dependencies to the topmost `vendor/` directory, at the root of your project. This is foundational to dep's model, and cannot be disabled.
* dep wholly controls `vendor`, and will blow away any manual changes or additions made to it that deviate from the version of an upstream source dep selects.
* dep requires that all packages from a given project/repository be at the same version.
* dep generally does not care about what's on your GOPATH; it deals exclusively with projects sourced from remote network locations. (Hint inference is the only exception to this; once solving begins, GOPATH - and any custom changes you've made to code therein - is ignored.)
* dep generally prefers semantic versioning-tagged releases to branches (when not given any additional rules). This is a significant shift from the "default branch" model of `go get` and some other tools. It can result in dep making surprising choices for dependencies for which it could not infer a rule.
* dep assumes that all generated code exists, and has been committed to the source.

A small number of projects that have reported being unable, thus far, to find a reasonable way of adapting to these requirements. If you can't figure out how to make your project fit, please file an issue - while dep necessarily cannot accommodate every single existing approach, it is dep's goal is define rules to which all Go projects can reasonably adapt.

### Hard failures

All of the hard failure modes are covered extensively in the reference on [failure modes](failure-modes.md).

Because the solver, and all its possible failures, are the same for `dep init` as for `dep ensure`, there's a separate section for understanding and dealing with them: [dealing with solving failures](failure-modes.md#solving-failures). It can be trickier with `dep init`, however, as many remediations require tweaking `Gopkg.toml`.

Unfortunately, `dep init` does not write out a partial `Gopkg.toml` when it fails. This is a known, critical problem, and [we have an open issue (help wanted!)](https://github.com/golang/dep/issues/909).

In the meantime, if the particular errors you are encountering do entail `Gopkg.toml` tweaks, you unfortunately may have to do without the automation of `dep init`: create an empty [`Gopkg.toml`](Gopkg.toml.md), and populate it with rules by hand. Before resorting to that, make sure you've run `dep init` with various combinations of the inferencing flags (`-skip-tools` and `-gopath`) to see if they can at least give you something to start from.

### Soft failures

Soft failures are cases where `dep init` appears to exit cleanly, but a subsequent `go build` or `go test` fails. Dep's soft failures are usually more drastically than subtly wrong - e.g., an explosion of type errors when you try to build, because a wildly incorrect version for some dependency got selected.

If you do encounter problems like this, `dep status` is your first diagnostic step; it will report what versions were selected for all your dependencies. It may be clear which dependencies are a problem simply from your building or testing error messages. If not, compare the `dep status` list against the versions recorded by your previous tool to find the differences.

Once you've identified the problematic dependenc(ies), the next step is exerting appropriate controls over them via `Gopkg.toml`.

For each of the following items, assume that you should run `dep ensure` after making the suggested change. If that fails, consult [dealing with solving failures]().

* If the wrong `[[constraint]]` was inferred for one of your direct dependencies, change it. Then, file an issue against dep (please!) - while `dep init` may choose to omit a constraint, converting one incorrectly is considered a bug.
* If one of your transitive dependencies is at the wrong version, define an `[[override]]` on it to force it to the version you need.
  * If the version you need is a specific git commit, it's preferable to instead manually change the `revision` to the desired hash in `Gopkg.lock` for that project, then drop the `version` or `branch` fields (if any).
* If one of your direct dependencies is at the wrong version and there's no `[[constraint]]` on it in `Gopkg.toml` already, then define an appropriate one.
  * As with the transitive dependencies, if the version you need is a specific git commit, prefer doing that manually in `Gopkg.lock`.

Hopefully this information is enough to get you through your project's migration to dep. If not, please feel free to file an issue, or join us in [#vendor on the Gopher's slack](https://gophers.slack.com/messages/C0M5YP9LN) for help!
