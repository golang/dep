# FAQ

_The first rule of FAQ is don't bikeshed the FAQ, leave that for
[Create structure for managing docs](https://github.com/golang/dep/issues/331)._

Please contribute to the FAQ! Found an explanation in an issue or pull request helpful?
Summarize the question and quote the reply, linking back to the original comment.

* [What is a direct or transitive dependency?](#what-is-a-direct-or-transitive-dependency)
* [Should I commit my vendor directory?](#should-i-commit-my-vendor-directory)
* [Why is it `dep ensure` instead of `dep install`?](#why-is-it-dep-ensure-instead-of-dep-install)
* [Does `dep` replace `go get`?](#does-dep-replace-go-get)
* [Why did `dep ensure -update` not update package X?](#why-did-dep-ensure--update-not-update-package-x)
* [Why is `dep` ignoring the version specified in the manifest?](#why-is-dep-ignoring-the-version-specified-in-the-manifest)
* [`dep` deleted my files in the vendor directory!](#dep-deleted-my-files-in-the-vendor-directory)
* [Can I put the manifest and lock in the vendor directory?](#can-i-put-the-manifest-and-lock-in-the-vendor-directory)
* [Why did dep use a different revision for package X instead of the revision in the lock file?](#why-did-dep-use-a-different-revision-for-package-x-instead-of-the-revision-in-the-lock-file)

## What is a direct or transitive dependency?
* Direct dependencies are dependencies that are imported by your project.
* Transitive dependencies are the dependencies of your dependencies. Necessary
  to compile but are not directly used by your code.

## Should I commit my vendor directory?

Committing the vendor directory is totally up to you. There is no general advice that applies in all cases.

**Pros**: it's the only way to get truly reproducible builds, as it guards against upstream renames and deletes; and you don't need an extra `dep ensure` step on fresh clones to build your repo.

**Cons**: your repo will be bigger, potentially a lot bigger; and PR diffs are more annoying.

## Why is it `dep ensure` instead of `dep install`?

> Yeah, we went round and round on names. [A lot](https://gist.github.com/jessfraz/315db91b272441f510e81e449f675a8b).
>
> The idea of "ensure" is roughly, "ensure that all my local states - code tree, manifest, lock, and vendor - are in sync with each other." When arguments are passed, it becomes "ensure this argument is satisfied, along with synchronization between all my local states."
>
> We opted for this approach because we came to the conclusion that allowing the tool to perform partial work/exit in intermediate states ended up creating a tool that had more commands, had far more possible valid exit and input states, and was generally full of footguns. In this approach, the user has most of the same ultimate control, but exercises it differently (by modifying the code/manifest and re-running dep ensure).
-[@sdboyer in #371](https://github.com/golang/dep/issues/371#issuecomment-293246832)

## Does `dep` replace `go get`?

> I would say that dep doesn't replace go get, but they both can do similar things. Here's how I use them:
>
> `go get`: I want to download the source code for a go project so that I can work on it myself, or to install a tool. This clones the repo under GOPATH for all to use.
>
> `dep ensure`: I have imported a new dependency in my code and want to download the dependency so I can start using it. My workflow is "add the import to the code, and then run dep ensure so that the manifest/lock/vendor are updated". This clones the repo under my project's vendor directory, and remembers the revision used so that everyone who works on my project is guaranteed to be using the same version of dependencies.
-[@carolynvs in #376](https://github.com/golang/dep/issues/376#issuecomment-293964655)

> The long term vision is a sane, overall-consistent go tool. My general take is that `go get`
> is for people consuming Go code, and dep-family commands are for people developing it.
-[@sdboyer in #376](https://github.com/golang/dep/issues/376#issuecomment-294045873)

## Why did `dep ensure -update` not update package X?
Is package X a direct dependency? [#385](https://github.com/golang/dep/issues/385)

Constraints given in a project's manifest are only applied if the
dependent project is actually imported. Transitive dependencies (dependencies
of your imports) are only updated when the revision in the lockfile no
longer meets the constraints of your direct dependencies.

> If you absolutely need to specify the constraint of a transitive dep from your own project, you have two options:
>
> Specify the constraint on github.com/gorilla/context via an override. Overrides apply globally, but are a power only given to the root project, so if anything else imports your project, the override won't be used.
> Mark github.com/gorilla/context as a required package in the manifest. This will cause it to be treated as a direct dependency, and your constraint will come into effect.
>
> However, before taking either of those steps, I'd say it's worth asking if you actually need to use master of github.com/gorilla/context. I imagine it's imported by github.com/gorilla/mux - and if that package is OK with using the tagged release instead of master (which is the preferred mode of operation anyway), then maybe that should be good enough for you? If you really needed something out of github.com/gorilla/context, then you'd probably be importing it directly and doing something with it
-[@sdboyer in #385](https://github.com/golang/dep/issues/385#issuecomment-294361087)

## Why is `dep` ignoring the version specified in the manifest?
Only direct dependencies can be managed with a `dependencies` entry
in the manifest. Use an `overrides` entry for transitive dependencies.

>  Constraints:
>
>  1. Can be declared by any project's manifest, yours or a dependency
>  2. Apply only to direct dependencies of the project declaring the constraint
>  3. Must not conflict with the constraints declared in any other project's manifest
>
>  Overrides:
>
>  1. Are only utilized from the current/your project's manifest
>  2. Apply globally, to direct and transitive dependencies
>  3. Supercede constraints declared in all manifests, yours or a dependency's
>
>  Overrides are also discussed with some visuals in [the gps docs](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#overrides).
-[@sdboyer in #328](https://github.com/golang/dep/issues/328#issuecomment-286631961)

## `dep` deleted my files in the vendor directory!
First, sorry! 😞 We hope you were able to recover your files...

> dep assumes complete control of vendor/, and may indeed blow things away if it feels like it.
-[@peterbourgon in #206](https://github.com/golang/dep/issues/206#issuecomment-277139419)

## Can I put the manifest and lock in the vendor directory?
No.

> Placing these files inside vendor/ would concretely bind us to vendor/ in the long term.
> We prefer to treat the use of vendor/ as an implementation detail.
-[@sdboyer on go package management list](https://groups.google.com/d/msg/go-package-management/et1qFUjrkP4/LQFCHP4WBQAJ)

## Why did dep use a different revision for package X instead of the revision in the lock file?
Sometimes the revision specified in the lock file is no longer valid. There are a few
ways this can occur:

* When you generated the lock file, you had an unpushed commit in your local copy of package X's repository in your GOPATH.
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