---
title: Day-to-day dep
---

In keeping with Go's general design philosophy of minimizing knobs, dep has a sparse interface - there are only two commands you're likely to run regularly. For existing projects,  `dep ensure` is the primary workhorse command, and the only thing you'll run that actually changes disk state. `dep status` is a read-only command that can help you understand the current state of your project.

Sparse interface notwithstanding, acclimating to `dep ensure` can take some practice. The verb here is "ensure", as we're asking dep to "make sure that my `Gopkg.lock` satisfies all the imports from my project and the rules in `Gopkg.toml`, and that `vendor/` contains exactly what `Gopkg.lock` says it should." In other words, dep is designed around the idea that it's fine to run `dep ensure` at most any time, and it will bring your project and its dependencies into a good state, while doing as little work as possible to achieve that guarantee (though [we're still optimizing "as little as possible"]()).

That's pretty vague, though. Let's make it clearer by exploring some concrete examples. We'll start by moving to the root of a hypothetical project, then running `dep ensure`:

```
$ cd $GOPATH/src/github.com/me/example
$ dep ensure
```

If `dep ensure` exits 0, then we're guaranteed (with [one fixable caveat]()), that our project is "in sync" - `vendor/` is populated with a depgraph that satisfies all imports and rules. So, let's assume `dep ensure` exited 0, and our project is now in sync. That means we're ready to develop normally: edit `.go` files, run `go test` and `go build`, etc. We don't need to think about dep again until one of the following happens:

- We want to add a new dependency
- We want to upgdate an existing dependency
- We've imported a package for the first time, or removed the last import of a package
- We've made a change to a rule in `Gopkg.toml`

There's a bit of [TIMTOWTDI](https://en.wiktionary.org/wiki/TMTOWTDI) here, as it's possible, and sometimes preferable, to handle each of these cases with a plain `dep ensure`, but dep also allows some additional parameters to make some of these cases a bit more ergonomic. We'll explore each in turn.

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

This reflects the nature of `dep ensure` - `github.com/pkg/errors` isn't in our project's imports, and as such, a subsequent `dep ensure` will classify it as unnecessary, and remove it. If we're ready to use the package, though, then this shouldn't be a problem - add an `import "github.com/pkg/errors"`  to a `.go` file, and the project's now back in sync.

It's also possible to introduce the new dependency without relying on `-add`. If, as a first step, you add `import "github.com/pkg/errors"` to a `.go` file, then run a plain `dep ensure`, your project will be in almost the same state as with `-add`: `Gopkg.lock` and `vendor/` will include `github.com/pkg/errors`. The only difference is that  `Gopkg.toml` will not have been updated with a guess at a version constraint.

In a sense, the plain `dep ensure` approach is more natural here, as using `-add` effectively dupes dep into violating its guarantee - `dep ensure` exits 0, but the extra dependency means our project is out of sync. However, the plain approach has a chicken-or-egg problem. Many Go developers use something like [`goimports`]() to read the contents of a `.go` file and infer what import statements to add. `goimports`, in turn, works by searching vendor and GOPATH for packages that match the identifiers in a file - which requires that those packages already be present locally.

**Both approaches can be useful in different situations. If you're genuinely experimenting with permanently adding a new dependency to your project, then `dep ensure -add` is probably best: you'll probably want that constraint **

### Updating dependencies

First, a quick definition: in dep's world, updating a dependency is the act of changing the value of the `revision` field in `Gopkg.lock` for that project (and, by extension, the version in the `vendor` directory). There are two basic ways of achieving this:

* Run `dep ensure -update <dependency/root/import/path>`; this will update the project to the latest version allowed by the version constraints in `Gopkg.toml`.
* Edit `Gopkg.toml`, changing the version constraint to one that does not allow the version currently in `Gopkg.lock`, then run `dep ensure`.

The CLI-driven approach is strongly preferred to hand-editing `Gopkg.toml`. Let's look at that first.

#### CLI-driven updates

`dep ensure -update` can be used to update multiple dependencies at once:

```
$ dep ensure -update github.com/foo/bar github.com/baz/quux
```

Or all dependencies (not recommended):

```
dep ensure -update
```

The CLI-driven approach is preferred because it is much more convenient, less expressive (read: safer), and better for the Go packaging ecosystem. However, it doesn't always work, as the semantics of `-update` are dependent on the constraint declared in `Gopkg.toml`:

* If `version` is specified with a semantic version range, then `dep ensure -update` will try to get the latest version in that range.
* If `branch` is specified, `dep ensure -update` will try to move to the latest tip of the named branch.
* If `version` is specified with a non-semantic version, or with a non-range (e.g., `version = "=v1.0.0"`), `dep ensure -update` will only make changes if the release moved (e.g., someone did a `git push --force`).
* If a `revision` is specified, `dep ensure -update` cannot make any changes.
* If no constraint is specified (typically not recommended), `dep ensure -update` will try versions in [the upgrade sort order](https://godoc.org/github.com/golang/dep/gps#SortForUpgrade).

There's a fair bit of nuance to the options here, which is explored in greater detail in [Zen of Constraints and Locks](). But only the first and second kinds of constraints are especially useful with `dep ensure -update`. And that's only partially under your control - if the dependency you're working with hasn't made any semver releases, then you can't use semver ranges. 

#### Manual updates

If using semver ranges or branches isn't an option for a particular dependency, you'll have to rely instead on hand-editing `Gopkg.toml` with the exact version you want, then running `dep ensure`.

Given that the version constraints given in `Gopkg.toml` determine the behavior of `dep ensure -update`, hand-editing the constraints necessarily allows a superset of the changes to `Gopkg.lock` allowed by `dep ensure -update`. Really, these sorts of manual edits can be considered "updates" only in the loosest sense; they could just as easily be downgrades, or a change of the constraint type (e.g. from `version` to `branch`) that is not neatly classifiable as "up" or "down."

Note that these approaches are not mutually exclusive. If, for example, you initially constrain a project to `version = "^1.1.0"`, then later run `dep ensure -update` bringing it to `v1.2.0` in `Gopkg.lock` and start using new features introduced in `v1.2.0` in your code, then it's important to update the bottom of the constraint range: `version = "^1.2.0"`.



 hand-edit `Gopkg.toml`  update the range are also times when you'll need to 





but it may not be possible to use. Whether it's possible is a combination of what releases the dependency has published, and what kind of constraint you've chosen to 



The first approach is simultaneously more convenient and less expressive than the second,  


