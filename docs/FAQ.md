# FAQ

_The first rule of FAQ is don't bikeshed the FAQ, leave that for
[Create structure for managing docs](https://github.com/golang/dep/issues/331)._

Please contribute to the FAQ! Found an explanation in an issue or pull request helpful?
Summarize the question and quote the reply, linking back to the original comment.

## Concepts
* [Does `dep` replace `go get`?](#does-dep-replace-go-get)
* [Why is it `dep ensure` instead of `dep install`?](#why-is-it-dep-ensure-instead-of-dep-install)
* [What is a direct or transitive dependency?](#what-is-a-direct-or-transitive-dependency)

## Configuration
* [What is the difference between Gopkg.toml (the "manifest") and Gopkg.lock (the "lock")?](#what-is-the-difference-between-gopkgtoml-the-manifest-and-gopkglock-the-lock)
* [How do I constrain a transitive dependency's version?](#how-do-i-constrain-a-transitive-dependencys-version)
* [Can I put the manifest and lock in the vendor directory?](#can-i-put-the-manifest-and-lock-in-the-vendor-directory)
* [How do I get `dep` to authenticate to a `git` repo?](#how-do-i-get-dep-to-authenticate-to-a-git-repo)
* [How do I get `dep` to consume private `git` repos using a Github Token?](#how-do-i-get-dep-to-consume-private-git-repos-using-a-github-token)

## Behavior
* [How does `dep` decide what version of a dependency to use?](#how-does-dep-decide-what-version-of-a-dependency-to-use)
* [What external tools are supported?](#what-external-tools-are-supported)
* [Why is `dep` ignoring a version constraint in the manifest?](#why-is-dep-ignoring-a-version-constraint-in-the-manifest)
* [Why did `dep` use a different revision for package X instead of the revision in the lock file?](#why-did-dep-use-a-different-revision-for-package-x-instead-of-the-revision-in-the-lock-file)
* [Why is `dep` slow?](#why-is-dep-slow)
* [How does `dep` handle symbolic links?](#how-does-dep-handle-symbolic-links)
* [Does `dep` support relative imports?](#does-dep-support-relative-imports)
* [How do I make `dep` resolve dependencies from my `GOPATH`?](#how-do-i-make-dep-resolve-dependencies-from-my-gopath)
* [Will `dep` let me use git submodules to store dependencies in `vendor`?](#will-dep-let-me-use-git-submodules-to-store-dependencies-in-vendor)

## Best Practices
* [Should I commit my vendor directory?](#should-i-commit-my-vendor-directory)
* [How do I roll releases that `dep` will be able to use?](#how-do-i-roll-releases-that-dep-will-be-able-to-use)
* [What semver version should I use?](#what-semver-version-should-i-use)
* [Is it OK to make backwards-incompatible changes now?](#is-it-ok-to-make-backwards-incompatible-changes-now)
* [My dependers don't use `dep` yet. What should I do?](#my-dependers-dont-use-dep-yet-what-should-i-do)
* [How do I configure a dependency that doesn't tag its releases?](#how-do-i-configure-a-dependency-that-doesnt-tag-its-releases)
* [How do I use `dep` with Docker?](#how-do-i-use-dep-with-docker)
* [How do I use `dep` in CI?](#how-do-i-use-dep-in-ci)

## Concepts
### Does `dep` replace `go get`?

No. `dep` and `go get` serve mostly different purposes.

Here are some suggestions for when you could use `dep` or `go get`:
> I would say that dep doesn't replace go get, but they both can do similar things. Here's how I use them:
>
> `go get`: I want to download the source code for a go project so that I can work on it myself, or to install a tool. This clones the repo under GOPATH for all to use.
>
> `dep ensure`: I have imported a new dependency in my code and want to download the dependency so I can start using it. My workflow is "add the import to the code, and then run dep ensure so that the manifest/lock/vendor are updated". This clones the repo under my project's vendor directory, and remembers the revision used so that everyone who works on my project is guaranteed to be using the same version of dependencies.
>
> [@carolynvs in #376](https://github.com/golang/dep/issues/376#issuecomment-293964655)

> The long term vision is a sane, overall-consistent go tool. My general take is that `go get`
> is for people consuming Go code, and dep-family commands are for people developing it.
>
> [@sdboyer in #376](https://github.com/golang/dep/issues/376#issuecomment-294045873)

### Why is it `dep ensure` instead of `dep install`?

> Yeah, we went round and round on names. [A lot](https://gist.github.com/jessfraz/315db91b272441f510e81e449f675a8b).
>
> The idea of "ensure" is roughly, "ensure that all my local states - code tree, manifest, lock, and vendor - are in sync with each other." When arguments are passed, it becomes "ensure this argument is satisfied, along with synchronization between all my local states."
>
> We opted for this approach because we came to the conclusion that allowing the tool to perform partial work/exit in intermediate states ended up creating a tool that had more commands, had far more possible valid exit and input states, and was generally full of footguns. In this approach, the user has most of the same ultimate control, but exercises it differently (by modifying the code/manifest and re-running dep ensure).
>
> [@sdboyer in #371](https://github.com/golang/dep/issues/371#issuecomment-293246832)

### What is a direct or transitive dependency?
* Direct dependencies are dependencies that are imported directly by your project: they appear in at least one import statement from your project.
* Transitive dependencies are the dependencies of your dependencies. Necessary to compile but are not directly used by your code.

## Configuration
### What is the difference between `Gopkg.toml` (the "manifest") and `Gopkg.lock` (the "lock")?

> The manifest describes user intent, and the lock describes computed outputs. There's flexibility in manifests that isn't present in locks..., as the "branch": "master" constraint will match whatever revision master HAPPENS to be at right now, whereas the lock is nailed down to a specific revision.
>
> This flexibility is important because it allows us to provide easy commands (e.g. `dep ensure -update`) that can manage an update process for you, within the constraints you specify, AND because it allows your project, when imported by someone else, to collaboratively specify the constraints for your own dependencies.
>
> [@sdboyer in #281](https://github.com/golang/dep/issues/281#issuecomment-284118314)

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
constraints and overrides are declared in the same way in `Gopkg.toml`, they
behave differently:

* Constraints:
   1. Can be declared by any project's manifest, yours or a dependency
   2. Apply only to direct dependencies of the project declaring the constraint
   3. Must not conflict with the `constraint` entries declared in any other project's manifest
* Overrides:
   1. Are only utilized from the current/your project's manifest
   2. Apply globally, to direct and transitive dependencies
   3. Supersede constraints declared in all manifests, yours or a dependency's

Overrides are also discussed with some visuals in [the gps docs](https://github.com/sdboyer/gps/wiki/gps-for-Implementors#overrides).

## Can I put the manifest and lock in the vendor directory?
No.

> Placing these files inside `vendor/` would concretely bind us to `vendor/` in the long term.
> We prefer to treat the `vendor/` as an implementation detail.
>
> [@sdboyer on go package management list](https://groups.google.com/d/msg/go-package-management/et1qFUjrkP4/LQFCHP4WBQAJ)

## How do I get dep to authenticate to a git repo?

`dep` currently uses the `git` command under the hood, so configuring the credentials
for each repository you wish to authenticate to will allow `dep` to use an
authenticated repository.

First, configure `git` to use the credentials option for the specific repository.

For example, if you use gitlab, and you wish to access `https://gitlab.example.com/example/package.git`,
then you would want to use the following configuration:

```
$ git config --global credential.https://gitlab.example.com.example yourusername
```

In the example the hostname `gitlab.example.com.username` string seems incorrect, but
it's actually the hostname plus the name of the repo you are accessing which is `username`.
The trailing 'yourusername' is the username you would use for the actual authentication.

You also need to configure `git` with the authentication provider you wish to use. You can get
a list of providers, with the command:

```
$ git help -a | grep credential-
  credential-cache          remote-fd
  credential-cache--daemon  remote-ftp
  credential-osxkeychain    remote-ftps
  credential-store          remote-http
```

You would then choose an appropriate provider. For example, to use the osxkeychain, you
would use the following:

```
git config --global credential.helper osxkeychain
```

If you need to do this for a CI system, then you may want to use the "store" provider.
Please see the documentation on how to configure that: https://git-scm.com/docs/git-credential-store

After configuring `git`, you may need to use `git` manually once to have it store the
credentials. Once you've checked out the repo manually, it will then use the stored
credentials. This at least appears to be the behavior for the osxkeychain provider.

### How do I get dep to consume private git repos using a Github Token?

Another alternative to make `dep` work with private repos is to use a [Personal Github
Token](https://help.github.com/articles/creating-a-personal-access-token-for-the-command-line/)
and configure it inside the [`.netrc` file](https://www.gnu.org/software/inetutils/manual/html_node/The-_002enetrc-file.html)
as the following example:
```
machine github.com
    login [YOUR_GITHUB_TOKEN]
```

Once you have set that up, dep will automatically use that Token to authenticate to the repositories.

## Behavior
### How does `dep` decide what version of a dependency to use?

The full algorithm is complex, but the most important thing to understand is
that `dep` tries versions in a [certain
order](https://godoc.org/github.com/golang/dep/gps#SortForUpgrade),
checking to see a version is acceptable according to specified constraints.

- All semver versions come first, and sort mostly according to the semver 2.0
  spec, with one exception:
  - Semver versions with a prerelease are sorted after *all* non-prerelease
    semver. Within this subset they are sorted first by their numerical
    component, then lexicographically by their prerelease version.
- The default branch(es) are next; the semantics of what "default branch" means
  are specific to the underlying source type, but this is generally what you'd
  get from a `go get`.
- All other branches come next, sorted lexicographically.
- All non-semver versions (tags) are next, sorted lexicographically.
- Revisions, if any, are last, sorted lexicographically. Revisions do not
  typically appear in version lists, so the only invariant we maintain is
  determinism - deeper semantics, like chronology or topology, do not matter.

So, given a slice of the following versions:

- Branch: `master` `devel`
- Semver tags: `v1.0.0` `v1.1.0` `v1.1.0-alpha1`
- Non-semver tags: `footag`
- Revision: `f6e74e8d`

Sorting for upgrade will result in the following slice.

`[v1.1.0 v1.0.0 v1.1.0-alpha1 footag devel master f6e74e8d]`

There are a number of factors that can eliminate a version from consideration,
the simplest of which is that it doesn't match a constraint. But if you're
trying to figure out why `dep` is doing what it does, understanding that its
basic action is to attempt versions in this order should help you to reason
about what's going on.

## What external tools are supported?
During `dep init` configuration from other dependency managers is detected
and imported, unless `-skip-tools` is specified.

The following tools are supported: `glide`, `godep`, `vndr`, `govend`, `gb` and `gvt`.

See [#186](https://github.com/golang/dep/issues/186#issuecomment-306363441) for
how to add support for another tool.

## Why is `dep` ignoring a version constraint in the manifest?
Only your project's directly imported dependencies are affected by a `constraint` entry
in the manifest. Transitive dependencies are unaffected. See [How do I constrain a transitive dependency's version](#how-do-i-constrain-a-transitive-dependencys-version)?

## Why did `dep` use a different revision for package X instead of the revision in the lock file?
Sometimes the revision specified in the lock file is no longer valid. There are a few
ways this can occur:

* When you generated the lock file, you had an unpushed commit in your local copy of package X's repository in your `GOPATH`. (This case will be going away soon)
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
>
> [@sdboyer in #405](https://github.com/golang/dep/issues/405#issuecomment-295998489)

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

> because we're not crazy people who delight in inviting chaos into our lives, we need to work within one `GOPATH` at a time.
-[@sdboyer in #247](https://github.com/golang/dep/pull/247#issuecomment-284181879)

Out of convenience, one might create a symlink to a directory within their `GOPATH/src`, e.g. `ln -s ~/go/src/github.com/user/awesome-project ~/Code/awesome-project`.

When `dep` is invoked with a project root that is a symlink, it will be resolved according to the following rules:

- If the symlink is outside `GOPATH` and links to a directory within a `GOPATH`, or vice versa, then `dep` will choose whichever path is within `GOPATH`.
- If the symlink is within a `GOPATH` and the resolved path is within a *different* `GOPATH`, then an error is thrown.
- If both the symlink and the resolved path are in the same `GOPATH`, then an error is thrown.
- If neither the symlink nor the resolved path are in a `GOPATH`, then an error is thrown.

This is the only symbolic link support that `dep` really intends to provide. In keeping with the general practices of the `go` tool, `dep` tends to either ignore symlinks (when walking) or copy the symlink itself, depending on the filesystem operation being performed.

## Does `dep` support relative imports?

No.
> dep simply doesn't allow relative imports. this is one of the few places where we restrict a case that the toolchain itself allows. we disallow them only because:
>
> * the toolchain already frowns heavily on them<br>
> * it's worse for our case, as we start venturing into [dot dot hell](http://doc.cat-v.org/plan_9/4th_edition/papers/lexnames) territory when trying to prove that the import does not escape the tree of the project
>
> [@sdboyer in #899](https://github.com/golang/dep/issues/899#issuecomment-317904001)

For a refresher on Go's recommended workspace organization, see the ["How To Write Go Code"](https://golang.org/doc/code.html) article in the Go docs. Organizing your code this way gives you a unique import path for every package.

## How do I make `dep` resolve dependencies from my `GOPATH`?

`dep init` provides an option to scan the `GOPATH` for dependencies by doing
`dep init -gopath`, which falls back to network mode when the packages are not
found in `GOPATH`. `dep ensure` doesn't work with projects in `GOPATH`.

## Will `dep` let me use git submodules to store dependencies in `vendor`?

No, with just one tiny exception: `dep` preserves `/vendor/.git`, if it exists. This was added at [cockroachdb](https://github.com/cockroachdb/cockroach)'s request, who rely on it to keep `vendor` from bloating their primary repository.

The reasons why git submodules will not be a part of dep are best expressed as a pro/con list:

**Pros**

* git submodules provide a well-structured way of nesting repositories within repositories.

**Cons**

* The nesting that git submodules perform is no more powerful or expressive than what dep already does, but dep does it both more generally (for bzr and hg) and more domain-specifically (e.g. elimination of nested vendor directories).
* Incorporating git submodules in any way would new fork new paths in the logic to handle the submodule cases, meaning nontrivial complexity increases.
* dep does not currently know or care if the project it operates on is under version control. Relying on submodules would entail that dep start paying attention to that. That it would only be conditionally does not make it better - again, more forking paths in the logic, more complexity.
* Incorporating submodules in a way that is at all visible to the user (and why else would you do it?) makes dep's workflows both more complicated and less predictable: _sometimes_ submodule-related actions are expected; _sometimes_ submodule-derived workflows are sufficient.
* Nesting one repository within another implies that changes could, potentially, be made directly in that subrepository. This is directly contrary to dep's foundational principle that `vendor` is dead code, and directly modifying anything in there is an error.

## Best Practices
### Should I commit my vendor directory?

It's up to you:

**Pros**

- It's the only way to get truly reproducible builds, as it guards against upstream renames,
  deletes and commit history overwrites.
- You don't need an extra `dep ensure` step to sync `vendor/` with Gopkg.lock after most operations,
  such as `go get`, cloning, getting latest, merging, etc.

**Cons**

- Your repo will be bigger, potentially a lot bigger,
  though `dep prune` can help minimize this problem.
- PR diffs will include changes for files under `vendor/` when Gopkg.lock is modified,
  however files in `vendor/` are [hidden by default](https://github.com/github/linguist/blob/v5.2.0/lib/linguist/generated.rb#L328) on Github.

## How do I roll releases that `dep` will be able to use?

In short: make sure you've committed your `Gopkg.toml` and `Gopkg.lock`, then
just create a tag in your version control system and push it to the canonical
location. `dep` is designed to work automatically with this sort of metadata
from `git`, `bzr`, and `hg`.

It's strongly preferred that you use [semver](http://semver.org)-compliant tag
names. We hope to develop documentation soon that describes this more precisely,
but in the meantime, the [npm](https://docs.npmjs.com/misc/semver) docs match
our patterns pretty well.

## What semver version should I use?

This can be a nuanced question, and the community is going to have to work out
some accepted standards for how semver should be applied to Go projects. At the
highest level, though, these are the rules:

* Below `v1.0.0`, anything goes. Use these releases to figure out what you want
  your API to be.
* Above `v1.0.0`, the general Go best practices continue to apply - don't make
  backwards-incompatible changes - exported identifiers can be added to, but
  not changed or removed.
* If you must make a backwards-incompatible change, then bump the major version.

It's important to note that having a `v1.0.0` does not preclude you from having
alpha/beta/etc releases. The semver spec allows for [prerelease
versions](http://semver.org/#spec-item-9), and `dep` is careful to _not_ allow
such versions unless `Gopkg.toml` contains a range constraint that explicitly
includes prereleases: if there exists a version `v1.0.1-alpha4`, then the
constraint `>=1.0.0` will not match it, but `>=1.0.1-alpha1` will.

Some work has been done towards [a tool
to](https://github.com/bradleyfalzon/apicompat) that will analyze and compare
your code with the last release, and suggest the next version you should use.

## Is it OK to make backwards-incompatible changes now?

Yes. But.

`dep` will make it possible for the Go ecosystem to handle
backwards-incompatible changes more gracefully. However, `dep` is not some
magical panacea. Version and dependency management is hard, and dependency hell
is real. The longstanding community wisdom about avoiding breaking changes
remains important. Any `v1.0.0` release should be accompanied by a plan for how
to avoid future breaking API changes.

One good strategy may be to add to your API instead of changing it, deprecating
old versions as you progress. Then, when the time is right, you can roll a new
major version and clean out a bunch of deprecated symbols all at once.

Note that providing an incremental migration path across breaking changes (i.e.,
shims) is tricky, and something we [don't have a good answer for
yet](https://groups.google.com/forum/#!topic/go-package-management/fp2uBMf6kq4).

## My dependers don't use `dep` yet. What should I do?

For the most part, you needn't do anything differently.

The only possible issue is if your project is ever consumed as a library. If
so, then you may want to be wary about committing your `vendor/` directory, as
it can [cause
problems](https://groups.google.com/d/msg/golang-nuts/AnMr9NL6dtc/UnyUUKcMCAAJ).
If your dependers are using `dep`, this is not a concern, as `dep` takes care of
stripping out nested `vendor` directories.

## How do I configure a dependency that doesn't tag its releases?

Add a constraint to `Gopkg.toml` that specifies `branch: "master"` (or whichever branch you need) in the `[[constraint]]` for that dependency. `dep ensure` will determine the current revision of your dependency's master branch, and place it in `Gopkg.lock` for you. See also: [What is the difference between Gopkg.toml and Gopkg.lock?](#what-is-the-difference-between-gopkgtoml-the-manifest-and-gopkglock-the-lock)

## How do I use `dep` with Docker?

`dep ensure -vendor-only` creates the vendor folder from a valid `Gopkg.toml` and `Gopkg.lock` without checking for Go code.
This is especially useful for builds inside docker utilizing cache layers.

Sample dockerfile:

```Dockerfile
FROM golang:1.9 AS builder

RUN curl -fsSL -o /usr/local/bin/dep https://github.com/golang/dep/releases/download/vX.X.X/dep-linux-amd64 && chmod +x /usr/local/bin/dep

RUN mkdir -p /go/src/github.com/***
WORKDIR /go/src/github.com/***

COPY Gopkg.toml Gopkg.lock ./
# copies the Gopkg.toml and Gopkg.lock to WORKDIR

RUN dep ensure -vendor-only
# install the dependencies without checking for go code

...
```

## How do I use `dep` in CI?

Since `dep` is expected to change until `v1.0.0` is released, it is recommended to rely on a released version.
You can find the latest binary from the [releases](https://github.com/golang/dep/releases) page.

Sample configuration for Travis CI:

```yml
# ...

env:
  - DEP_VERSION="X.X.X"

before_install:
  # Download the binary to bin folder in $GOPATH
  - curl -L -s https://github.com/golang/dep/releases/download/v${DEP_VERSION}/dep-linux-amd64 -o $GOPATH/bin/dep
  # Make the binary executable
  - chmod +x $GOPATH/bin/dep

install:
  - dep ensure
```

Caching can also be enabled but there are a couple of caveats you should be aware of:

> Until recently, we have had intermittent cache corruption that would have been super annoying if it was breaking Travis build too.
>
> Also according to https://docs.travis-ci.com/user/caching/#Things-not-to-cache, they don't recommend it for larger caches.
>
> https://docs.travis-ci.com/user/caching/#How-does-the-caching-work%3F
>
> > Note that this makes our cache not network-local, it's still bound to network bandwidth and DNS resolutions for S3.
> > That impacts what you can and should store in the cache. If you store archives larger than a few hundred megabytes in the cache, it's unlikely that you'll see a big speed improvement.
>
> [@carolynvs in #1293](https://github.com/golang/dep/pull/1293#issuecomment-342969292)

If you are sure you want to enable caching on travis, it can be done by adding `$GOPATH/pkg/dep`, the default location for `dep` cache, to the cached directories:

```yml
# ...

cache:
  directories:
    - $GOPATH/pkg/dep
```
