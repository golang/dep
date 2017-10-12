# Gopkg.toml

## `required`
`required` lists a set of packages (not projects) that must be included in
Gopkg.lock. This list is merged with the set of packages imported by the current
project.
```toml
required = ["github.com/user/thing/cmd/thing"]
```

**Use this for:** linters, generators, and other development tools that

* Are needed by your project
* Aren't `import`ed by your project, [directly or transitively](FAQ.md#what-is-a-direct-or-transitive-dependency)
* You don't want put in your `GOPATH`, and/or you want to lock the version

Please note that this only pulls in the sources of these dependencies. It does not install or compile them. So, if you need the tool to be installed you should still run the following (manually or from a `Makefile`)  after each `dep ensure`:

```bash
cd vendor/pkg/to/install
go install .
```

This only works reliably if this is the only project to install these executables. This is not enough if you want to be able to run a different version of the same executable depending on the project you're working. In that case you have to use a different `GOBIN` for each project, by doing something like this before running the above commands:

```bash
export GOBIN=$PWD/bin
export PATH=$GOBIN:$PATH
```

You might also try [virtualgo](https://github.com/GetStream/vg), which installs dependencies in the `required` list automatically in a project specific `GOBIN`.

## `ignored`
`ignored` lists a set of packages (not projects) that are ignored when dep statically analyzes source code. Ignored packages can be in this project, or in a dependency.
```toml
ignored = ["github.com/user/project/badpkg"]
```

Use wildcard character (*) to define a package prefix to be ignored. Use this
to ignore any package and their subpackages.
```toml
ignored = ["github.com/user/project/badpkg*"]
```

**Use this for:** preventing a package and any of that package's unique
dependencies from being installed.

## `metadata`
`metadata` can exist at the root as well as under `constraint` and `override` declarations.

`metadata` declarations are ignored by dep and are meant for usage by other independent systems.

A `metadata` declaration at the root defines metadata about the project itself. While a `metadata` declaration under a `constraint` or an `override` defines metadata about that `constraint` or `override`.
```toml
[metadata]
key1 = "value that convey data to other systems"
system1-data = "value that is used by a system"
system2-data = "value that is used by another system"
```

## `constraint`
A `constraint` provides rules for how a [direct dependency](FAQ.md#what-is-a-direct-or-transitive-dependency) may be incorporated into the
dependency graph.
They are respected by dep whether coming from the Gopkg.toml of the current project or a dependency.
```toml
[[constraint]]
  # Required: the root import path of the project being constrained.
  name = "github.com/user/project"
  # Recommended: the version constraint to enforce for the project.
  # Only one of "branch", "version" or "revision" can be specified.
  version = "1.0.0"
  branch = "master"
  revision = "abc123"

  # Optional: an alternate location (URL or import path) for the project's source.
  source = "https://github.com/myfork/package.git"

  # Optional: metadata about the constraint or override that could be used by other independent systems
  [metadata]
  key1 = "value that convey data to other systems"
  system1-data = "value that is used by a system"
  system2-data = "value that is used by another system"
```

**Use this for:** having a [direct dependency](FAQ.md#what-is-a-direct-or-transitive-dependency)
use a specific branch, version range, revision, or alternate source (such as a
fork).

## `override`
An `override` has the same structure as a `constraint` declaration, but supersede all `constraint` declarations from all projects. Only `override` declarations from the current project's `Gopkg.toml` are applied.

```toml
[[override]]
  # Required: the root import path of the project being constrained.
  name = "github.com/user/project"
  # Optional: specifying a version constraint override will cause all other constraints on this project to be ignored; only the overridden constraint needs to be satisfied. Again, only one of "branch", "version" or "revision" can be specified.
  version = "1.0.0"
  branch = "master"
  revision = "abc123"
  # Optional: specifying an alternate source location as an override will enforce that the alternate location is used for that project, regardless of what source location any dependent projects specify.
  source = "https://github.com/myfork/package.git"

  # Optional: metadata about the constraint or override that could be used by other independent systems
  [metadata]
  key1 = "value that convey data to other systems"
  system1-data = "value that is used by a system"
  system2-data = "value that is used by another system"
```

**Use this for:** all the same things as a [`constraint`](#constraint), but for
[transitive dependencies](FAQ.md#what-is-a-direct-or-transitive-dependency).
See [How do I constrain a transitive dependency's version?](FAQ.md#how-do-i-constrain-a-transitive-dependencys-version)
for more details on how overrides differ from `constraint`s. _Overrides should
be used cautiously, sparingly, and temporarily._

## `version`

`version` is a property of `constraint`s and `override`s. It is used to specify
version constraint of a specific dependency.

Internally, dep uses [Masterminds/semver](https://github.com/Masterminds/semver)
to work with semver versioning.

`~` and `=` operators can be used with the versions. When a version is specified
without any operator, `dep` automatically adds a caret operator, `^`. The caret
operator pins the left-most non-zero digit in the version. For example:
```
^1.2.3 means 1.2.3 <= X < 2.0.0
^0.2.3 means 0.2.3 <= X < 0.3.0
^0.0.3 means 0.0.3 <= X < 0.1.0
```

To pin a version of direct dependency in manifest, prefix the version with `=`.
For example:
```toml
[[constraint]]
  name = "github.com/pkg/errors"
  version = "=0.8.0"
```

[Why is dep ignoring a version constraint in the manifest?](FAQ.md#why-is-dep-ignoring-a-version-constraint-in-the-manifest)

# Example

Here's an example of a sample Gopkg.toml with most of the elements

```toml
required = ["github.com/user/thing/cmd/thing"]

ignored = ["github.com/user/project/pkgX", "bitbucket.org/user/project/pkgA/pkgY"]

[metadata]
codename = "foo"

[[constraint]]
  name = "github.com/user/project"
  version = "1.0.0"

  [metadata]
  property1 = "value1"
  property2 = 10

[[constraint]]
  name = "github.com/user/project2"
  branch = "dev"
  source = "github.com/myfork/project2"

[[override]]
  name = "github.com/x/y"
  version = "2.4.0"

  [metadata]
  propertyX = "valueX"
```
