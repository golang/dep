---
title: Creating a New Project
---

Once you have [dep installed](installation.md), we need to pick a root directory for our project. This is primarily about picking the right root import path, and corresponding directory beneath `$GOPATH/src`, at which to situate your project. There are four basic possibilities:

1.  A project that is now or eventually may be shared with or imported by other projects/people. In this case, pick the import path corresponding to the VCS root of its intended network location, e.g., `$GOPATH/src/github.com/golang/dep`.
2.  An entirely local project - one that you have no intention of pushing to a central server (like GitHub). In this case, any subdirectory beneath `$GOPATH/src` will do.
3.  A project that needs to live within a large repository, such as a company monorepo. This may be possible, but gets more complicated. (Unfortunately, no docs on this yet - coming soon!)
4.  Treat the entire GOPATH as a single project, where `$GOPATH/src` is the root. Dep [does not currently support this](https://github.com/golang/dep/issues/417) - it needs a non-empty import path to treat as the root of your project's import namespace.

We'll assume the first case, as it's the most common. Create and move into the directory:

```bash
$ mkdir -p $GOPATH/src/github.com/me/example
$ cd $GOPATH/src/github.com/me/example
```

Now, we'll initialize the project:

```bash
$ dep init
$ ls
Gopkg.toml Gopkg.lock vendor/
```

In a new project like this one, both files and the `vendor` directory will be effectively empty.

This would also be a good time to set up a version control, such as [git](https://git-scm.com/). While dep in no way requires version control for your project, it can make inspecting the changes made by normal dep operations easier. Plus, it's basically best practice #1 of modern software development!

At this point, our project is initialized, and we're ready to start writing code. You can open up a `.go` file in an editor and start hacking away. Or, **after creating your first `.go` file**, you can go ahead and pre-populate your `vendor` directory with some projects that you already know that you'll need:

```bash
$ dep ensure -add github.com/foo/bar github.com/baz/quux
```

Now you're ready to move on to [Daily Dep](daily-dep.md)!
