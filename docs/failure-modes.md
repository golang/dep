---
title: Failure Modes
---

Like all complex, network-oriented software, dep has known failure modes. These generally fall into two categories: I/O and logical. I/O errors arise from unexpected responses to system calls that interact with the network or local disk. Logical failures occur when dep encounters issues within the package management problem domain.

## I/O errors

dep reads from the network, and reads and writes to disk, and is thus subject to all the typical errors that are possible with such activities: full disks, failed disks, lack of permissions, network partitions, firewalls, etc. However, there are three classes of I/O errors that are worth addressing specifically:

* Network failures
* Bad local cache state
* `vendor` write errors

In general, these problems aren't things we can reasonably program around in dep. Therefore, they can't be considered bugs for us to fix. Fortunately, most of these problems have straightforward remediations.

### Network failures

> **Remediation tl;dr:** most network issues are ephemeral, even if they may last for a few minutes, and can be addressed simply by re-running the same command. Always try this before attempting more invasive solutions.

dep talks to the network at several different points. These vary somewhat depending on source (VCS) type and local disk state, but this list of operations is generally instructive:

* When dep cannot [statically deduce](deduction.md#static-deduction) the source root of an import path, it issues a `go-get` HTTP metadata request to a URL constructed from the import path.
* Retrieving the list of available versions for a source (think `git ls-remote`) necessarily requires network activity.
* Initially downloading (in git terms, `git clone`) an upstream source into the local cache also necessarily requires network activity.
* Updating a local cache (in git terms, `git fetch`) with the latest changes from an upstream source.
* Writing out code trees under `vendor` is typically done from the local cache, but under some circumstances a tarball may be fetched on-the-fly from a remote source.

Network failures that you actually may observe are biased towards the earlier items in the list, simply because those operations tend to happen first: you generally don't see update failures as much as version-listing failures, because they usually have the same underlying cause (source host is down, network partition, etc.), but the version-list request happens first on most paths.

#### Persistent network failures

Although most network failures are ephemeral, there are three well-defined cases where they're more permanent:

* **The network on which the source resides is permanently unreachable from the user's location:** in practice, this generally means one of two things: you've forgotten to log into your company VPN, or you're behind [the GFW](https://en.wikipedia.org/wiki/Great_Firewall). In the latter case, setting the _de facto_ standard HTTP proxy environment variables that [`http.ProxyFromEnvironment()`](https://golang.org/pkg/net/http/#ProxyFromEnvironment) respects will cause dep's `go-get` HTTP metadata requests, as well as git, bzr, and hg subcommands, to utilize the proxy.

  * Remediation is also exactly the same when the custom `go-get` HTTP metadata service for a source is similarly unreachable. The failure messages, however, will look like [deduction failures](#deduction-failures).

* **The source has been permanently deleted or moved:** these are [left-pad](https://www.theregister.co.uk/2016/03/23/npm_left_pad_chaos/) events, though note that [GitHub automatically redirects traffic after renames](https://help.github.com/articles/renaming-a-repository/), mitigating the rename problem. But, if an upstream source is removed, dep will be unable to proceed until a new upstream source is established for the import path. To that end:

  * If you still have a copy of the source repository in your local cache or GOPATH, consider uploading it to a new location (e.g. forking it) and using a [`source`](Gopkg.toml.md#source) rule to point to the fork.
  * If you don't have a whole repository locally, then extracting the code currently in your `vendor` directory into a new repository and pushing it to a . (Note: this may have licensing implications.)
  * If you have no instances of the code locally, then there's little that can be done - that code is simply gone, and you'll need to refactor your project.

  Future versions of dep will be able to better handle an interim period before a new upstream/forked source is created, or simply living in a world where a given code tree exists solely in your project's `vendor` directory.

* **The user lacks the necessary credentials to interact with a source:** see the [FAQ on configuring credentials](FAQ.md#how-do-i-get-dep-to-authenticate-to-a-git-repo).

The exact error text will vary depending on which of the operations is running, what type of source dep is trying to communicate with, and what actual network problem has occurred. The error text may not always make it immediately clear which combination of these you're dealing with, but for persistent problems, it should at least reduce the search space.

#### Hangs

> **Remediation tl;dr:** hangs are almost always network congestion, or sheer amount of network data to fetch. Wait, or cancel and try again with `-v` to try to get more context.

Almost any case where a dep command, run with `-v`, hangs for more than ten minutes will ultimately be a bug. However, the most common explanation for an apparent dep hangs is actually normal behavior: because dep's operation requires that it keep its own copies of upstream sources hidden away in the [local cache](glossary.md#local-cache), the first run of dep against a project, especially large projects, can take a long time while it populates the cache.

The only known case where dep may hang indefinitely is if one of the underlying VCS binaries it calls is prompting for some kind of input. Typically this means credentials (though not always - make sure to accept remote hosts' SSH keys into your known hosts!), and dep's normal assumption is that necessary credentials have been provided via environmental mechanisms - [configuration files or daemons](FAQ.md#how-do-i-get-dep-to-authenticate-to-a-git-repo), SSH agents, etc. This assumption is necessary for dep's concurrent network activity to work. If your use case absolutely cannot support the use of any such environmental caching mechanism, [please weigh in on this issue](https://github.com/golang/dep/issues/1476).

Unfortunately, until dep [improves the observability of its ongoing I/O operations](), it cannot accurately report to the user which operations are actually underway at any given moment. This can make it difficult to differentiate from other hangs - credentials prompts, long network timeouts induced by firewalls, sluggish TCP when faced with packet loss, etc.

### Bad local cache state

> **Remediation tl;dr:** Remove the local cache dir: `rm -rf $GOPATH/pkg/dep/sources`.

It is possible for parts of the [local cache](glossary.md#local-cache) maintained by dep to get into a bad state. This primarily happens when dep processes are forcibly terminated (e.g. Ctrl-C). This can, for example, terminate a `git` command partway through, leaving bad state on disk. By dep's definition, a [dirty git working copy]() is bad state.

The error messages arising from bad local cache state often do not include full paths, so it may not be immediately obvious that problems are originating in the local cache. If full paths aren't included, then the best hint tends to be that the errors look like local VCS errors, but they're not on files from your own project.

However, for the most part, **dep automatically discovers and recovers from bad local cache state problems**, rebounding back into a good state as it bootstraps each command execution. If you do encounter what appears to be a local cache problem from which dep does not automatically recover, then the fix is typically to just throw out the cache, `rm -rf $GOPATH/pkg/dep/sources`; dep will repopulate it automatically on the next run. However, if you have time, please preserve the local cache dir and report it as a bug!

There are no known cases where, in the course of normal operations, dep can irreparably corrupt its own local cache. Any such case would be considered a critical bug in dep, and you should report it! If you think you've encountered such a case, it should have the following characteristics:

* The error message you're seeing is consistent with some sort of disk state error in a downloaded source within `$GOPATH/pkg/dep/sources`
* You can identify a bad state (generally: a vcs "status"-type command will either fail outright, or report a modified working tree) in a subdirectory of `$GOPATH/pkg/dep/sources` suggested by the above error
* The exact same error recurs after removing the local cache dir and running the same command, **without** prematurely terminating the project (e.g. via Ctrl-C)

### `vendor` write errors

Dep may encounter errors while attempting to write out the `vendor` directory itself (any such errors will result in a full rollback; causing no changes to be made to disk). To help pinpoint where the problem may be, know that this is the flow for populating `vendor`:

1.  Allocate a new temporary directory within the system temporary directory.
2.  Rename the existing `vendor` directory to `vendor.orig`. Do this within the current project's root directory if possible; if not, rename and move it to the tempdir.
3.  Create a new `vendor` directory within the tempdir and concurrently populate it with all the projects named in `Gopkg.lock`.
4.  Move the new `vendor` directory into place in the current project's root directory.
5.  Delete the old `vendor` directory.

Note: this flow will become more targeted after [vendor verification]() allows dep to identify and target the subset of projects currently in `vendor` that need to be changed.

Known problems in this category include:

* Insufficient space in the temporary directory will cause an error, triggering a rollback. However, because the rollback process cleans up files written so-far, the temporary partition won't actually be full after dep exits, which can be misleading.
* Attempting to [re]move the original `vendor` directory can fail with permissions errors if any of the files therein are "open", in some editors/on some OSes (particularly Windows). [There's an issue for this]().

## Logical failures

Logical failures encompass everything that can happen within dep's logical problem-solving domain - after

Some of these failures can be as straightforward as typos, and are just as easily resolved. Others, unfortunately, may necessitate forking and modifying an upstream project - although such cases are very rare.

### Deduction failures

Import path deduction, as detailed in the [deduction reference](deduction.md), has both static and dynamic phases. When neither of these phases is able to determine the source root for a given import path, it is considered to be a deduction failure. Deduction failures all contain this key error text:

```bash
...unable to deduce repository and source type for "<bad path>"...
```

_Note: there are [more varied error messages for the small subset of cases](#malformed-import-paths) where an import path appears to be deducible, but is somehow malformed._

When a deduction failure occurs on a given import path, the proximal cause will have been one of following five scenarios (arranged from most to least likely):

* The import path was never deducible.
* **Dynamic deduction failures:**
  * The import path was, at one time, dynamically deducible, and the metadata service for it is up, but it is unreachable by dep.
  * The import path was, at one time, dynamically deducible, but the metadata service for it is down.
* **Static rule changes:**
  * The import path cannot be statically deduced by the running version of dep, but a newer version of dep has added rules that can statically deduce it.
  * The import path was once statically deducible, but the running version of dep has discontinued support for it.

In all of these cases, your last recourse will be to add a [`source`](Gopkg.toml.md#source) directive to fix the problem. However, these directives are brittle, and should only be used when other options have been exhausted; also, until [this problem is solved](https://github.com/golang/dep/issues/860), even `source` may not be able to help.

#### Undeducible paths

> **Remediation tl;dr:** You made a typo; fix it. If not, you may need a `source`, but be sparing with those.

The most likely cause of deduction failure is minor user error. Specifically, the user is the _current_ user (you), and the error is there is a mistyped import path somewhere in the current (your) project. The problem may be in your `Gopkg.toml`, or one of your imports, but the error message should point you directly at the problem, and the solution is usually obvious - e.g., "gihtub".

Validation of the inputs from the current project are made fast and up front in dep, so these errors will tend to present themselves immediately. Between this fast validation, and the fact that projects are typically uncompilable, or at least not `go get`-able, with these kinds of errors, they tend to be caught early. This is why truly undeducible paths pop up primarily as temporary accidents while hacking on your own projects - you have to fix them to move on.

That undeducibility is an immediate and hard blocker, however, has led to this being a sticking point for migration to dep. In particular, there are two issues:

* Several other Go dependency management tools do allow specifying arbitrary VCS/source URLs, and [but support for that via `source` in dep is still pending](https://github.com/golang/dep/issues/860).
* GitHub Enterprise only implements `go-get` HTTP metadata correctly for the root package of a repository. In practice, this makes all import paths pointing to GHE undeducible, and `source` can't help either without the aforementioned improvement.

If the problem import path is in your current project, but the problem isn't an obvious typo, then you're likely experiencing a dynamic failure, or may need to check the [deduction reference](deduction.md) to understand what what a deducible import path looks like.

#### Dynamic deduction failures

Most dynamic deduction failures are either ephemeral network or service availability issues, and will go away by re-running the previous command. Always try that first.

If the issue persists, and you're certain the import path should be deducible, network issues are the first culprit to check. The typical causes (VPN, firewalls) and remediation for when a metadata service is unreachable are the same as [when a source itself is unreachable](#persistent-network-failures).

The next possibility is a metadata service that's permanently gone away. Whereas network errors are still reasonably common, it is rare to encounter an import path pointing to a defunct public metadata service. Consider: that one import path can render the entire project unfetchable and/or uncompilable, and neither of those are states that popular projects can afford to be in for long. So, being that most (public Go ecosystem) dependencies are on the more popular projects, as long as you're also depending on the more popular projects, you're unlikely to encounter this.

Of course, defunct _private_ metadata services may be much more common, as they are subject to entirely different incentives.

If you think you've encountered a defunct metadata service, try probing the domain portion of the import path directly to see if there is an HTTP(S) server there at all. If not, you can only force with `source` - assuming you know what source URL you should use. If not, you may need to refactor your code (if the problem is in your project), pick a different version of the problem dependency, or drop the problem dependency entirely; sometimes, you just have to get rid of dead code.

#### Static rule changes

> **Remediation tl;dr:** make sure you have the latest released version of dep.

Static rule changes are very unlikely to be the cause of your deduction failures.

It is plausible that dep will add new static deduction rules in the future. And it is possible that, if you have an older version of dep, and you collaborate with or pull in code from someone using a newer version of dep, then their code may take advantage of new import path patterns that your dep doesn't know about yet. But very, very few static rules additions are likely to ever be made to dep over its lifetime - and getting access to them is just a question of updating once.

The final scenario - dep discontinuing support for a static deduction pattern - is included for clarity and completeness, but simply should never happen. Even if a hosting service covered by static rules today were to shut down, dep would retain the existing static rules; if hosted code had been migrated elsewhere, then dep would attempt to perform a remapping automatically. If no such remapping were possible, then dep would still recognize the basic host pattern, but may fall back on using malformed import path errors - the next topic - to informatively reject new imports from the host.

#### Malformed import paths

For the most part, static ("is it one of the handful of hosts we know?") and dynamic ("just do whatever the metadata service tells us to do") deduction are single-pass checks. However, both cases can perform some minor additional validation:

* In static deduction, the rules are necessarily specific to each host, but most enforce allowable characters and schemes in URLs that are known to be required by the underlying host.
* In dynamic deduction, responses from the metadata service are minimally validated to ensure that the source type and scheme are all supported, and that the URL contains valid characters.

### Solving failures

When `dep ensure` or `dep init` exit with an error message looking something like this:

```bash
$ dep init
init failed: unable to solve the dependency graph: Solving failure: No versions of github.com/foo/bar met constraints:
	v1.0.1: Could not introduce github.com/foo/bar@v1.13.1, as its subpackage github.com/foo/bar/foo is missing. (Package is required by (root).)
	v1.0.0: Could not introduce github.com/foo/bar@v1.13.0, as...
	v0.1.0: (another error)
	master: (another error)
```

_Note: all three of the other hard failure types can sometimes be reported as the errors for individual versions in a list like this. This primarily happens because dep is in need of a [thorough refactor of its error handling](https://github.com/golang/dep/issues/288)._

It means that the solver was unable to find a combination of versions for all dependencies that satisfy all the rules enforced by the solver. It is crucial to note that, just because dep provides a big list of reasons why each version failed _doesn't mean_ you have to address each one! That's just dep telling you why it ultimately couldn't use each of those versions in a solution.

These rules, and specific remediations for failing to meet them, are described in detail in the section on [solver invariants](the-solver.md#solving-invariants). This section is about the steps to take when solving failures occur in general. But, to set context, here's a summary:

* **`[[constraint]]` conflicts:** when projects in the dependency graph disagree on what [versions](Gopkg.toml.md#version-rules) are acceptable for a project, or where to [source](Gopkg.toml.md#source) it from.
  * Remediation will usually be either changing a `[[constraint]]` or adding an `[[override]]`, but genuine conflicts may require forking and hacking code.
* **Package validity failure:** when an imported package is quite obviously not capable of being built.
  * There usually isn't much remediation here beyond "stop importing that," as it indicates something broken at a particular version.
* **Import comment failure:** when the import path used to address a package differs from the [import comment](https://golang.org/cmd/go/#hdr-Import_path_checking) the package uses to specify how it should be imported.
  * Remediation is to use the specified import path, instead of whichever one you used.
* **Case-only import variation failure:** when two equal-except-for-case imports exist in the same build.
  * Remediation is to pick one case variation to use throughout your project, then manually update all projects in your depgraph to use the new casing.

Let's break down the process of addressing a solving failure into a series of steps:

1.  First, look through the failed versions list for a version of the dependency that works for you (or a failure that seems fixable), then try to work that one out. Often enough, you'll see a single failure repeated across the entire version list, which makes it pretty clear what problem you need to solve.
2.  Take the remediation steps specific to that failure.
3.  Re-run the same command you ran that produced the failure. There are three possible outcomes:
    1.  Success!
    2.  Your fix was ineffective - the same failure re-occurs. Either re-examine your fix (step 2), or look for a new failure to fix (step 1).
    3.  Your fix was effective, but some new failure arose. Return to step 1 with the new failure list.
