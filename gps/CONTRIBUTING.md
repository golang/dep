# Contributing to `gps`

:+1::tada: First, we're thrilled you're thinking about contributing! :tada::+1:

As a library trying to cover all the bases in Go package management, it's
crucial that we incorporate a broad range of experiences and use cases. There is
a strong, motivating design behind `gps`, but we are always open to discussion
on ways we can improve the library, particularly if it allows `gps` to cover
more of the Go package management possibility space.

`gps` has no CLA, but we do have a [Code of Conduct](https://github.com/sdboyer/gps/blob/master/CODE_OF_CONDUCT.md). By
participating, you are expected to uphold this code.

## How can I contribute?

It may be best to start by getting a handle on what `gps` actually is. Our
wiki has a [general introduction](https://github.com/sdboyer/gps/wiki/Introduction-to-gps), a
[guide for tool implementors](https://github.com/sdboyer/gps/wiki/gps-for-Implementors), and
a [guide for contributors](https://github.com/sdboyer/gps/wiki/gps-for-contributors).
There's also a [discursive essay](https://medium.com/@sdboyer/so-you-want-to-write-a-package-manager-4ae9c17d9527)
that lays out the big-picture goals and considerations driving the `gps` design.

There are a number of ways to contribute, all highly valuable and deeply
appreciated:

* **Helping "translate" existing issues:** as `gps` exits its larval stage, it still
  has a number of issues that may be incomprehensible to everyone except
  @sdboyer. Simply asking clarifying questions on these issues is helpful!
* **Identifying missed use cases:** the loose `gps` rule of thumb is, "if you can do
  it in Go, we support it in `gps`." Posting issues about cases we've missed
  helps us reach that goal.
* **Writing tests:** in the same vein, `gps` has a [large suite](https://github.com/sdboyer/gps/blob/master/CODE_OF_CONDUCT.md) of solving tests, but
  they still only scratch the surface. Writing tests is not only helpful, but is
  also a great way to get a feel for how `gps` works.
* **Suggesting enhancements:** `gps` has plenty of missing chunks. Help fill them in!
* **Reporting bugs**: `gps` being a library means this isn't always the easiest.
  However, you could always compile the [example](https://github.com/sdboyer/gps/blob/master/example.go), run that against some of
  your projects, and report problems you encounter.
* **Building experimental tools with `gps`:** probably the best and fastest ways to
  kick the tires!

`gps` is still beta-ish software. There are plenty of bugs to squash! APIs are
stabilizing, but are still subject to change.

## Issues and Pull Requests

Pull requests are the preferred way to submit changes to 'gps'. Unless the
changes are quite small, pull requests should generally reference an
already-opened issue. Make sure to explain clearly in the body of the PR what
the reasoning behind the change is.

The changes themselves should generally conform to the following guidelines:

* Git commit messages should be [well-written](http://chris.beams.io/posts/git-commit/#seven-rules).
* Code should be `gofmt`-ed.
* New or changed logic should be accompanied by tests.
* Maintainable, table-based tests are strongly preferred, even if it means
  writing a new testing harness to execute them.

## Setting up your development environment

In order to run `gps`'s tests, you'll need to inflate `gps`'s dependencies using
`glide`. Install `[glide](https://github.com/Masterminds/glide)`, and then download
and install `gps`'s dependencies by running `glide install` from the repo base.

Also, you'll need to have working copies of `git`, `hg`, and `bzr` to run all of
`gps`'s tests.
