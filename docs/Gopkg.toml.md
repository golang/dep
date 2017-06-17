# Gopkg.toml

## `required`
`required` lists a set of packages (not projects) that must be included in Gopkg.lock. This list is merged with the set of packages imported by the current project. Use it when your project needs a package it doesn't explicitly import - including "main" packages.
 ```toml
required = ["github.com/user/thing/cmd/thing"]
```

## `ignored`
`ignored` lists a set of packages (not projects) that are ignored when dep statically analyzes source code. Ignored packages can be in this project, or in a dependency.
```toml
ignored = ["github.com/user/project/badpkg"]
```

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
A `constraint` provides rules for how a [direct dependency]((https://github.com/golang/dep/blob/master/FAQ.md#what-is-a-direct-or-transitive-dependency)) may be incorporated into the 
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

## `override`
An `override` has the same structure as a `constraint` declaration, but supersede all `constraint` declarations from all projects. Only `override` declarations from the current project's are applied.

 [When should I use constraint, override, required, or ignored in Gopkg.toml?](https://github.com/golang/dep/blob/master/FAQ.md#when-should-i-use-constraint-override-required-or-ignored-in-gopkgtoml)
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