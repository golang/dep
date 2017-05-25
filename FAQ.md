# FAQ

_The first rule of FAQ is don't bikeshed the FAQ, leave that for
[Create structure for managing docs](https://github.com/golang/dep/issues/331)._

Please contribute to the FAQ! Found an explanation in an issue or pull request helpful?
Summarize the question and quote the reply, linking back to the original comment.

* [What is the difference between Gopkg.toml (the "manifest") and Gopkg.lock (the "lock")?](#what-is-the-difference-between-gopkgtoml-the-manifest-and-gopkglock-the-lock)
* [When should I use `constraint`, `override` `required`, or `ignored` in the Gopkg.toml?](#when-should-i-use-constraint-override-required-or-ignored-in-gopkgtoml)
* [What is a direct or transitive dependency?](#what-is-a-direct-or-transitive-dependency)
* [Should I commit my vendor directory?](#should-i-commit-my-vendor-directory)
* [Why is it `dep ensure` instead of `dep install`?](#why-is-it-dep-ensure-instead-of-dep-install)
* [Does `dep` replace `go get`?](#does-dep-replace-go-get)
* [Why is `dep` ignoring a version constraint in the manifest?](#why-is-dep-ignoring-a-version-constraint-in-the-manifest)
* [How do I constrain a transitive dependency's version?](#how-do-i-constrain-a-transitive-dependencys-version)
* [`dep` deleted my files in the vendor directory!](#dep-deleted-my-files-in-the-vendor-directory)
* [Can I put the manifest and lock in the vendor directory?](#can-i-put-the-manifest-and-lock-in-the-vendor-directory)
* [Why did dep use a different revision for package X instead of the revision in the lock file?](#why-did-dep-use-a-different-revision-for-package-x-instead-of-the-revision-in-the-lock-file)
* [Why is `dep` slow?](#why-is-dep-slow)
* [How does `dep` handle symbolic links?](#how-does-dep-handle-symbolic-links)

## What is the difference between Gopkg.toml (the "manifest") and Gopkg.lock (the "lock")?

> The manifest describes user intent, and the lock describes computed outputs. There's flexibility in manifests that isn't present in locks..., as the "branch": "master" constraint will match whatever revision master HAPPENS to be at right now, whereas the lock is nailed down to a specific revision.
>
> This flexibility is important because it allows us to provide easy commands (e.g. `dep ensure -update`) that can manage an update process for you, within the constraints you specify, AND because it allows your project, when imported by someone else, to collaboratively specify the constraints for your own dependencies.
-[@sdboyer in #281](https://github.com/golang/dep/issues/281#issuecomment-284118314)

## When should I use `constraint`, `override`, `required`, or `ignored` in `Gopkg.toml`?

* Use `constraint` to constrain a [direct dependency](#what-is-a-direct-or-transitive-dependency) to a specific branch, version range, revision, or specify an alternate source such as a fork.
* Use `override` to constrain a [transitive dependency](#what-is-a-direct-or-transitive-dependency). See [How do I constrain a transitive dependency's version?](#how-do-i-constrain-a-transitive-dependencys-version) for more details on how overrides differ from dependencies. Overrides should be used cautiously, sparingly, and temporarily.
* Use `required` to explicitly add a dependency that is not imported directly or transitively, for example a development package used for code generation.
* Use `ignored` to ignore a package and any of that package's unique dependencies.

## What is a direct or transitive dependency?
* Direct dependencies are dependencies that are imported directly by your project: they appear in at least one import statement from your project.
* Transitive dependencies are the dependencies of your dependencies. Necessary to compile but are not directly used by your code.

## Should I commit my vendor directory?

It's up to you:

**Pros**

- it's the only way to get truly reproducible builds, as it guards against upstream renames and deletes
- you don't need an extra `dep ensure` step (to fetch dependencies) on fresh clones to build your repo

**Cons**

- your repo will be bigger, potentially a lot bigger
- PR diffs are more annoying

## Why is it `dep ensure` instead of `dep install`?

> Yeah, we went round and round on names. [A lot](https://gist.github.com/jessfraz/315db91b272441f510e81e449f675a8b).
>
> The idea of "ensure" is roughly, "ensure that all my local states - code tree, manifest, lock, and vendor - are in sync with each other." When arguments are passed, it becomes "ensure this argument is satisfied, along with synchronization between all my local states."
>
> We opted for this approach because we came to the conclusion that allowing the tool to perform partial work/exit in intermediate states ended up creating a tool that had more commands, had far more possible valid exit and input states, and was generally full of footguns. In this approach, the user has most of the same ultimate control, but exercises it differently (by modifying the code/manifest and re-running dep ensure).
-[@sdboyer in #371](https://github.com/golang/dep/issues/371#issuecomment-293246832)

## Does `dep` replace `go get`?

No, `dep` is an experiment and is still in its infancy. Depending on how this
experiment goes, it may be considered for inclusion in the go project in some form
or another in the future but that is not guaranteed.

Here are some suggestions for when you could use `dep` or `go get`:
> I would say that dep doesn't replace go get, but they both can do similar things. Here's how I use them:
>
> `go get`: I want to download the source code for a go project so that I can work on it myself, or to install a tool. This clones the repo under GOPATH for all to use.
>
> `dep ensure`: I have imported a new dependency in my code and want to download the dependency so I can start using it. My workflow is "add the import to the code, and then run dep ensure so that the manifest/lock/vendor are updated". This clones the repo under my project's vendor directory, and remembers the revision used so that everyone who works on my project is guaranteed to be using the same version of dependencies.
-[@carolynvs in #376](https://github.com/golang/dep/issues/376#issuecomment-293964655)

> The long term vision is a sane, overall-consistent go tool. My general take is that `go get`
> is for people consuming Go code, and dep-family commands are for people developing it.
-[@sdboyer in #376](https://github.com/golang/dep/issues/376#issuecomment-294045873)

## Why is `dep` ignoring a version constraint in the manifest?
Only your project's directly imported dependencies are affected by a `dependencies` entry
in the manifest. Transitive dependencies are unaffected.

Use an `overrides` entry for transitive dependencies.

## How do I constrain a transitive dependency's version?
First, if you're wondering about this because you're trying to keep the version
of the transitive dependency from changing, then you're working against `dep`'s
design. The lock file, `Gopkg.lock`, will keep the selected version of the
transitive dependency stable, unless you explicitly request an upgrade or it's
impossible to find a solution without changing that version.

If that isn't your use case and you still need to constrain a transitive
dependency, you have a couple of options:

1. Make the transitive dependency a direct one, either with a dummy import or an entry in the `required` list in `Gopkg.toml`.
2. Use an override.

Overrides are a sledgehammer, and should only be used as a last resort. While
dependencies and overrides are declared in the same way in `Gopkg.toml`, they
behave differently:

* Dependencies:
   1. Can be declared by any project's manifest, yours or a dependency
   2. Apply only to direct dependencies of the project declaring the constraint
   3. Must not conflict with the `dependencies` declared in any other project's manifest
* Overrides:
   1. Are only utilized from the current/your project's manifest
   2. Apply globally, to direct and transitive dependencies
   3. Supersede constraints declared in all manifests, yours or a dependency's

Overrides are also discussed with some visuals in [the gps docs](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#overrides).

## `dep` deleted my files in the vendor directory!
First, sorry! 😞 We hope you were able to recover your files...

> dep assumes complete control of vendor/, and may indeed blow things away if it feels like it.
-[@peterbourgon in #206](https://github.com/golang/dep/issues/206#issuecomment-277139419)

## Can I put the manifest and lock in the vendor directory?
No.

> Placing these files inside `vendor/` would concretely bind us to `vendor/` in the long term.
> We prefer to treat the `vendor/` as an implementation detail.
-[@sdboyer on go package management list](https://groups.google.com/d/msg/go-package-management/et1qFUjrkP4/LQFCHP4WBQAJ)

## Why did dep use a different revision for package X instead of the revision in the lock file?
Sometimes the revision specified in the lock file is no longer valid. There are a few
ways this can occur:

* When you generated the lock file, you had an unpushed commit in your local copy of package X's repository in your GOPATH. (This case will be going away soon)
* After generating the lock file, new commits were force pushed to package X's repository, causing the commit revision in your lock file to no longer exist.

To troubleshoot, you can revert dep's changes to your lock, and then run `dep ensure -v -n`.
This retries the command in dry-run mode with verbose logs enabled. Check the output
for a warning like the one below, indicating that a commit in the lock is no longer valid.

```
Unable to update checked out version: fatal: reference is not a tree: 4dfc6a8a7e15229398c0a018b6d7a078cccae9c8
```

> The lock file represents a set of precise, typically immutable versions for the entire transitive closure of dependencies for a project. But "the project" can be, and is, decomposed into just a bunch of arguments to an algorithm. When those inputs change, the lock may need to change as well.
>
> Under most circumstances, if those arguments don't change, then the lock remains fine and correct. You've hit one one of the few cases where that guarantee doesn't apply. The fact that you ran dep ensure and it DID a solve is a product of some arguments changing; that solving failed because this particular commit had become stale is a separate problem.
-[@sdboyer in #405](https://github.com/golang/dep/issues/405#issuecomment-295998489)

## Why is `dep` slow?

There are two things that really slow `dep` down. One is unavoidable; for the other, we have a plan.

The unavoidable part is the initial clone. `dep` relies on a cache of local
repositories (stored under `$GOPATH/pkg/dep`), which is populated on demand.
Unfortunately, the first `dep` run, especially for a large project, may take a
while, as all dependencies are cloned into the cache.

Fortunately, this is just an _initial_ clone - pay it once, and you're done.
The problem repeats itself a bit when you're running `dep` for the first time
in a while and there's new changesets to fetch, but even then, these costs are
only paid once per changeset.

The other part is the work of retrieving information about dependencies. There are three parts to this:

1. Getting an up-to-date list of versions from the upstream source
2. Reading the `Gopkg.toml` for a particular version out of the local cache
3. Parsing the tree of packages for import statements at a particular version

The first requires one or more network calls; the second two usually mean
something like a `git checkout`, and the third is a filesystem walk, plus
loading and parsing `.go` files. All of these are expensive operations.

Fortunately, we can cache the second and third. And that cache can be permanent
when keyed on an immutable identifier for the version - like a git commit SHA1
hash. The first is a bit trickier, but there are reasonable staleness tradeoffs
we can consider to avoid the network entirely. There's an issue to [implement
persistent caching](https://github.com/golang/dep/issues/431) that's the
gateway to all of these improvements.

There's another major performance issue that's much harder - the process of picking versions itself is an NP-complete problem in `dep`'s current design. This is a much trickier problem 😜

## How does `dep` handle symbolic links?

> because we're not crazy people who delight in inviting chaos into our lives, we need to work within one GOPATH at a time.
-[@sdboyer in #247](https://github.com/golang/dep/pull/247#issuecomment-284181879)

Out of convenience, one might create a symlink to a directory within their `GOPATH`, e.g. `ln -s ~/go/src/github.com/golang/dep dep`. When `dep` is invoked it will resolve the current working directory accordingly:

- If the cwd is a symlink outside a `GOPATH` and links to directory within a `GOPATH`, or vice versa, `dep` chooses whichever path is within the `GOPATH`.  If neither path is within a `GOPATH`, `dep` produces an error.
- If both the cwd and resolved path are in the same `GOPATH`, an error is thrown since the users intentions and expectations can't be accurately deduced.
- If the symlink is within a `GOPATH` and the real path is within a *different* `GOPATH` - an error is thrown.

This is the only symbolic link support that `dep` really intends to provide. In keeping with the general practices of the `go` tool, `dep` tends to either ignore symlinks (when walking) or copy the symlink itself, depending on the filesystem operation being performed.

