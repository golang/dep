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
observed to run between five and ten times the speed on darwin, at
speeds comparable to the that of the unix `find` utility; about twice
the speed on linux; and about four times the speed on Windows.

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
library does not throw the type information away, but rather provides
that information to the callback function in the form of a
`os.FileMode` value. Note that the provided `os.FileMode` value that
this library provides only has the node type information, and does not
have the permission bits, sticky bits, or other information from the
file's mode. If the callback does care about a particular node's
entire `os.FileInfo` data structure, the callback can easiy invoke
`os.Stat` when needed, and only when needed.

### It's more correct on Windows than `filepath.Walk`

I did not previously care about this either, but humor me. We all love
how we can write once and run everywhere. It is essential for the
language's adoption, growth, and success, that the software we create
can run unmodified on all architectures and operating systems
supported by Go.

When the traversed file system has a logical loop caused by symbolic
links to directories, on unix `filepath.Walk` ignores symbolic links
and traverses the entire directory tree without error. On Windows
however, `filepath.Walk` will continue following directory symbolic
links, even though it is not supposed to, eventually causing
`filepath.Walk` to terminate early and return an error when the
pathname gets too long from concatenating endless loops of symbolic
links onto the pathname. This error comes from Windows, passes through
`filepath.Walk`, and to the upstream client running `filepath.Walk`.

The takeaway is that behavior is different based on which platform
`filepath.Walk` is running. While this is clearly not intentional,
until it is fixed in the standard library, it presents a compatibility
problem.

This library correctly identifies symbolic links that point to
directories and will only follow them when `FollowSymbolicLinks` is
set to true. Behavior on Windows and other operating systems is
identical.

### It's more easy to use than `filepath.Walk`

Since this library does not invoke `os.Stat` on every file system node
it encounters, there is no possible error event for the callback
function to filter on. The third argument in the `filepath.WalkFunc`
function signature to pass the error from `os.Stat` to the callback
function is no longer necessary, and thus eliminated from signature of
the callback function from this library.

Also, `filepath.Walk` invokes the callback function with a solidus
delimited pathname regardless of the os-specific path separator. This
library invokes the callback function with the os-specific pathname
separator, obviating a call to `filepath.Clean` in the callback
function for each node prior to actually using the provided pathname.

In other words, even on Windows, `filepath.Walk` will invoke the
callback with `some/path/to/foo.txt`, requiring well written clients
to perform pathname normalization for every file prior to working with
the specified file. In truth, many clients developed on unix and not
tested on Windows neglect this subtlety, and will result in software
bugs when running on Windows. This library would invoke the callback
function with `some\path\to\foo.txt` for the same file when running on
Windows, eliminating the need to normalize the pathname by the client,
and lessen the likelyhood that a client will work on unix but not on
Windows.

### It's more flexible than `filepath.Walk`

The default behavior of this library is to ignore symbolic links to
directories when walking a directory tree, just like `filepath.Walk`
does. However, it does invoke the callback function with each node it
finds, including symbolic links. If a particular use case exists to
follow symbolic links when traversing a directory tree, this library
can be invoked in manner to do so, by setting the
`FollowSymbolicLinks` parameter to true.

The default behavior of this library is to always sort the immediate
descendants of a directory prior to visiting each node, just like
`filepath.Walk` does. This is usually the desired behavior. However,
this does come at a performance penalty to sort the names when a
directory node has many entries. If a particular use case exists that
does not require sorting the directory's immediate descendants prior
to visiting its nodes, this library will skip the sorting step when
the `Unsorted` parameter is set to true.
