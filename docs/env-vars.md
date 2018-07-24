---
id: env-vars
title: Environment Variables
---

dep's behavior can be modified by some environment variables:

* [`DEPCACHEAGE`](#depcacheage)
* [`DEPCACHEDIR`](#depcachedir)
* [`DEPPROJECTROOT`](#depprojectroot)
* [`DEPNOLOCK`](#depnolock)

Environment variables are passed through to subcommands, and therefore can be used to affect vcs (e.g. `git`) behavior.

---

### `DEPCACHEAGE`

If set to a [duration](https://golang.org/pkg/time/#ParseDuration) (e.g. `24h`), it will enable caching of metadata from source repositories: 

* Lists of published versions
* The contents of a project's `Gopkg.toml` file, at a particular version
* A project's tree of packages and imports, at a particular version

A duration must be set to enable caching. (In future versions of dep, it will be on by default). The duration is used as a TTL, but only for mutable information, like version lists. Information associated with an immutable VCS revision (packages and imports; `Gopkg.toml` declarations) is cached indefinitely.

The cache lives in `$DEPCACHEDIR/bolt-v1.db`, where the version number is an internal number associated with a particular data schema dep uses.

The file can be removed safely; the database will be automatically rebuilt as needed.

### `DEPCACHEDIR`

Allows the user to specify a custom directory for dep's [local cache](glossary.md#local-cache) of pristine VCS source repositories. Defaults to `$GOPATH/pkg/dep`.

### `DEPPROJECTROOT`

If set, the value of this variable will be treated as the [project root](glossary.md#project-root) of the [current project](glossary.md#current-project), superseding GOPATH-based inference.

This is primarily useful if you're not using the standard `go` toolchain as a compiler (for example, with Bazel), as there otherwise isn't much use to operating outside of GOPATH.

### `DEPNOLOCK`

By default, dep creates an `sm.lock` file at `$DEPCACHEDIR/sm.lock` in order to prevent multiple dep processes from interacting with the [local cache](glossary.md#local-cache) simultaneously. Setting this variable will bypass that protection; no file will be created. This can be useful on certain filesystems; VirtualBox shares in particular are known to misbehave.
