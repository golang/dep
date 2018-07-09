---
id: env-vars
title: Environment Variables
---

dep's behavior can be modified by some environment variables:

* [`DEPCACHEDIR`](#depcachedir)
* [`DEPPROJECTROOT`](#depprojectroot)
* [`DEPNOLOCK`](#depnolock)

Environment variables are passed through to subcommands, and therefore can be used to affect vcs (e.g. `git`) behavior.

---

### `DEPCACHEDIR`

Allows the user to specify a custom directory for dep's [local cache](glossary.md#local-cache) of pristine VCS source repositories. Defaults to `$GOPATH/pkg/dep`.

### `DEPPROJECTROOT`

If set, the value of this variable will be treated as the [project root](glossary.md#project-root) of the [current project](glossary.md#current-project), superseding GOPATH-based inference.

This is primarily useful if you're not using the standard `go` toolchain as a compiler (for example, with Bazel), as there otherwise isn't much use to operating outside of GOPATH.

### `DEPNOLOCK`

By default, dep creates an `sm.lock` file at `$DEPCACHEDIR/sm.lock` in order to prevent multiple dep processes from interacting with the [local cache](glossary.md#local-cache) simultaneously. Setting this variable will bypass that protection; no file will be created. This can be useful on certain filesystems; VirtualBox shares in particular are known to misbehave.
