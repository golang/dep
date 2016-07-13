# gps
![map-marker-icon copy](https://cloud.githubusercontent.com/assets/21599/16779217/4f5cdc6c-483f-11e6-9de3-661f13d9b215.png)
--

`gps` is the Go Packaging Solver. It is an engine for tackling dependency
management problems in Go. You can replicate the fetching bits of `go get`,
modulo arguments, [in about 30 lines of
code](https://github.com/sdboyer/gps/blob/master/example.go) with `gps`.

`gps` is _not_ Yet Another Go Package Management Tool. Rather, it's a library
that package management (and adjacent) tools can use to solve the
[hard](https://en.wikipedia.org/wiki/Boolean_satisfiability_problem) parts of
the problem in a consistent,
[holistic](https://medium.com/@sdboyer/so-you-want-to-write-a-package-manager-4ae9c17d9527)
way. `gps` is [on track](https://github.com/Masterminds/glide/pull/384) to become the engine behind [glide](https://glide.sh).

The wiki has a [general introduction to the `gps`
approach](https://github.com/sdboyer/gps/wiki/Introduction-to-gps), as well
as guides for folks [implementing
tools](https://github.com/sdboyer/gps/wiki/gps-for-Implementors) or [looking
to contribute](https://github.com/sdboyer/gps/wiki/Introduction-to-gps).

## Wait...a package management _library_?!

Yup. Because it's what the Go ecosystem needs right now.

There are [scads of
tools](https://github.com/golang/go/wiki/PackageManagementTools) out there, each
tackling some slice of the Go package management domain. Some handle more than
others, some impose more restrictions than others, and most are mutually
incompatible (or mutually indifferent, which amounts to the same). This
fragments the Go FLOSS ecosystem, harming the community as a whole.

As in all epic software arguments, some of the points of disagreement between
tools/their authors are a bit silly. Many, though, are based on legitimate
differences of opinion about what workflows, controls, and interfaces are
best to give Go developers.

Now, we're certainly no less opinionated than anyone else. But part of the
challenge has been that, with a problem as
[complex](https://medium.com/@sdboyer/so-you-want-to-write-a-package-manager-4ae9c17d9527)
as package management, subtle design decisions made in pursuit of a particular
workflow or interface can have far-reaching effects on architecture, leading to
deep incompatibilities between tools and approaches.

We believe that many of [these
differences](https://docs.google.com/document/d/1xrV9D5u8AKu1ip-A1W9JqhUmmeOhoI6d6zjVwvdn5mc/edit?usp=sharing)
are incidental - and, given the right general solution, reconcilable. `gps` is
our attempt at such a solution.

By separating out the underlying problem into a standalone library, we are
hoping to provide a common foundation for different tools. Such a foundation
could improve interoperability, reduce harm to the ecosystem, and make the
communal process of figuring out what's right for Go more about collaboration,
and less about fiefdoms.

### Assumptions

Ideally, `gps` could provide this shared foundation with no additional
assumptions beyond pure Go source files. Sadly, package management is too
complex to be assumption-less. So, `gps` tries to keep its assumptions to the
minimum, supporting as many situations as possible while still maintaining a
predictable, well-formed system.

* Go 1.6, or 1.5 with `GO15VENDOREXPERIMENT = 1` set. `vendor/`
  directories are a requirement.
* You don't manually change what's under `vendor/`. That’s tooling’s
  job.
* A **project** concept, where projects comprise the set of Go packages in a
  rooted directory tree.  By happy (not) accident, `vendor/` directories also
  just happen to cover a rooted tree.
* A **manifest** and **lock** approach to tracking project manifest data. The
  solver takes manifest (and, optionally, lock)-type data as inputs, and
  produces lock-type data as its output. Tools decide how to actually
  store this data, but these should generally be at the root of the
  project tree.

Manifests? Locks? Eeew. Yes, we also think it'd be swell if we didn't need
metadata files. We love the idea of Go packages as standalone, self-describing
code. Unfortunately, the wheels come off that idea as soon as versioning and
cross-project/repository dependencies happen. But universe alignment is hard;
trying to intermix version information directly with the code would only make
matters worse.

## Contributing

Yay, contributing! Please see
[CONTRIBUTING.md](https://github.com/sdboyer/gps/blob/master/CONTRIBUTING.md).
Note that `gps` also abides by a [Code of
Conduct](https://github.com/sdboyer/gps/blob/master/CODE_OF_CONDUCT.md), and is MIT-licensed.
