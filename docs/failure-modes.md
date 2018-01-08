---
title: Failure Modes
---

Like all complex, network-oriented software, dep has known failure modes. These can generally be divided into two groups: hard failures, where the dep process exits non-zero, and soft failures, where it exits zero, but maybe shouldn't have.



## Solving failures

When `dep ensure` or `dep init` exit with an error message looking something like this:

```
$ dep init
init failed: unable to solve the dependency graph: Solving failure: No versions of github.com/foo/bar met constraints:
	v1.0.1: Could not introduce github.com/foo/bar@v1.13.1, as its subpackage github.com/foo/bar/foo is missing. (Package is required by (root).)
	v1.0.0: Could not introduce github.com/foo/bar@v1.13.0, as... 
	v0.1.0: (another error)
	master: (another error)
```

It means that the solver was unable to find a combination of versions for all dependencies that satisfy all the rules enforced by the solver. These rules are described in detail in the section on [solver invariants](the-solver.md#solving-invariants), but here's a summary:

* **`[[constraint]]` conflicts:** when projects in the dependency graph disagree on what [versions](gopkg.toml.md#version-rules) are acceptable for a project, or where to [source](gopkg.toml.md#source) it from.
* **Package validity failure:** when an imported package is quite obviously not capable of being built.
* **Import comment failure:** when the import path used to address a package differs from the [import comment](https://golang.org/cmd/go/#hdr-Import_path_checking) the package uses to specify how it should be imported.
* **Case-only import variation failure:** when two equal-except-for-case imports exist in the same build.

