---
title: Creating a new project
---

First, we need a working Go workspace and GOPATH. If you're unfamiliar with Go workspaces and GOPATH, have a look at [the language documentation](https://golang.org/doc/code.html#Organization) and get your local workspace set up. (dep's model could eventually lead to being able to work without GOPATH, but we're not there yet.)

Next, we need to pick a root directory for our project. This is primarily about picking the right root import path, and corresponding directory beneath `$GOPATH/src`, at which to situate your project. There are four basic possibilities:

1. A project that is now or eventually may be shared with or imported by other projects/people. In this case, pick the import path corresponding to the VCS root of its intended network location, e.g., `$GOPATH/src/github.com/golang/dep`.
2. An entirely local project - one that you have no intention of pushing to a central server (like GitHub). In this case, any subdirectory beneath `$GOPATH/src` will do.
3. A project that needs to live within a large repository, such as a company monorepo. This is possible, but gets more complicated - see [dep in monorepos]().
4. Treat the entire GOPATH as a single project, where `$GOPATH/src` is the root. dep does not currently support this - it needs a non-empty import path to treat as the root of the import namespace.

We'll assume the first case, as it's the most common. Create and move into the directory:

```
$ mkdir -p $GOPATH/src/github.com/me/example
$ cd $GOPATH/src/github.com/me/example
```

Now, we'll initialize the project:

```
$ dep init
$ ls
Gopkg.toml Gopkg.lock vendor/
```

In a new project like this one, both files and the `vendor` directory will be effectively empty. 

At this point, we're initialized and ready to start writing code! You can open up a `.go` file in an editor and start hacking away. Or, if you already know some projects you'll need, you can pre-populate your `vendor` directory with them:

```
$ dep ensure -add github.com/foo/bar github.com/baz/quux
```

Note that, because of how `dep ensure -add` works, you'll have to list all dependencies you want to add in a single `dep ensure -add` execution, rather than running it repeatedly. But the `-add` approach is ultimately equivalent to just jumping directly into editing  `.go` files, so this needn't be a huge problem.

The [day-to-day dep]() guide is a good next stop now that your project is set up, and has more detail on these `-add` patterns.