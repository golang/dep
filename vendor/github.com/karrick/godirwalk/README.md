# godirwalk

`godirwalk` is a library for traversing a directory tree on a file
system.

In short, why do I use this library?

1. It's faster than `filepath.Walk`.
1. It's more correct on Windows than `filepath.Walk`.
1. It's more easy to use than `filepath.Walk`.
1. It's more flexible than `filepath.Walk`.

## Usage Example

This library will normalize the provided top level directory name
based on the os-specific path separator by calling `filepath.Clean` on
its first argument. However it always provides the pathname created by
using the correct os-specific path separator when invoking the
provided callback function.

```Go
    dirname := "some/directory/root"
    err := godirwalk.Walk(dirname, &godirwalk.Options{
        Callback: func(osPathname string, de *godirwalk.Dirent) error {
            fmt.Printf("%s %s\n", de.ModeType(), osPathname)
            return nil
        },
    })
```

This library not only provides functions for traversing a file system
directory tree, but also for obtaining a list of immediate descendants
of a particular directory, typically much more quickly than using
`os.ReadDir` or `os.ReadDirnames`.

Documentation is available via
[![GoDoc](https://godoc.org/github.com/karrick/godirwalk?status.svg)](https://godoc.org/github.com/karrick/godirwalk).

## Description

Here's why I use `godirwalk` in preference to `filepath.Walk`,
`os.ReadDir`, and `os.ReadDirnames`.

### It's faster than `filepath.Walk`

When compared against `filepath.Walk` in benchmarks, it has been
observed to run up to ten times the speed on unix, comparable to the
speed of the unix `find` utility, and about four times the speed on
Windows.

How does it obtain this performance boost? Primarily by not invoking
`os.Stat` on every file system node it encounters.

While traversing a file system directory tree, `filepath.Walk` obtains
the list of immediate descendants of a directory, and throws away the
file system node type information provided by the operating system
that comes with the node's name. Then, immediately prior to invoking
the callback function, `filepath.Walk` invokes `os.Stat` for each
node, and passes the returned `os.FileInfo` information to the
callback.

While the `os.FileInfo` information provided by `os.Stat` is extremely
helpful--and even includes the `os.FileMode` data--providing it
requires an additional system call for each node.

Because most callbacks only care about what the node type is, this
library does not throw that information away, but rather provides that
information to the callback function in the form of its `os.FileMode`
value. If the callback does care about a particular node's entire
`os.FileInfo` data structure, the callback can easiy invoke `os.Stat`
when needed, and only when needed.

### It's more correct on Windows than `filepath.Walk`

I did not previously care about this either, but humor me. We all love
how we can write once and run everywhere. It is essential for the
language's adoption, growth, and success, that the software we create
can run unmodified on both on unix like operating systems and on
Windows.

When the traversed file system has a loop caused by symbolic links to
directories, on Windows `filepath.Walk` will continue following
directory symbolic links, even though it is not supposed to,
eventually causing `filepath.Walk` to return an error when the
pathname gets too long from concatenating the directories in the loop
onto the pathname of the file system node. While this is clearly not
intentional, until it is fixed in the standard library, it presents a
compatibility problem.

This library correctly identifies symbolic links that point to
directories and will only follow them when `ResurseSymbolicLinks` is
set to true. Behavior on Windows and unix like operating systems is
identical.

### It's more easy to use than `filepath.Walk`

Since this library does not invoke `os.Stat` on every file system node
it encounters, there is no possible error event for the callback
function to filter on. The third argument in the `filepath.WalkFunc`
function signature to pass the error from `os.Stat` to the callback
function is no longer necessary, and thus eliminated from signature of
the callback function from this library.

Also, `filepath.Walk` invokes the callback function with a slashed
version of the pathname regardless of the os-specific path
separator. This library invokes callback function with the os-specific
pathname separator, obviating a call to `filepath.Clean` for each node
in the callback function, prior to actually using the provided
pathname.

In other words, even on Windows, `filepath.Walk` will invoke the
callback with `some/path/to/foo.txt`, requiring well written clients
to perform pathname normalization for every file prior to working with
the specified file. In truth, many clients developed on unix and not
tested on Windows neglect this difference, and will result in software
bugs when running on Windows. This library however would invoke the
callback function with `some\path\to\foo.txt` for the same file,
eliminating the need to normalize the pathname by the client, and
lessen the likelyhood that a client will work on unix but not on
Windows.

### It's more flexible than `filepath.Walk`

The `filepath.Walk` function attempts to ignore the problem posed by
file system directory loops created by symbolic links. I say "attempts
to" because it does follow symbolic links to directories on Windows,
causing infinite loops, or error messages, and causing behavior to be
different based on which platform is running. Even so, there are times
when following symbolic links while traversing a file system directory
tree is desired, and this library allows that by providing the
`FollowSymbolicLinks` option parameter when the upstream client
requires the functionality.

The `filepath.Walk` function also always sorts the immediate
descendants of a directory prior to traversing them. While this is
usually desired for consistent file system traversal, it is not always
needed, and may impact performance. This library provides the
`Unsorted` option to skip sorting directory descendants when the order
of file system traversal is not important for some applications.
