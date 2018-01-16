---
title: Creating a New Project
---

First, we need a working Go workspace and GOPATH. If you're unfamiliar with Go workspaces and GOPATH, have a look at [the language documentation](https://golang.org/doc/code.html#Organization) and get your local workspace set up. (dep's model could eventually lead to being able to work without GOPATH, but we're not there yet.)

Next, we need to pick a root directory for our project. This is primarily about picking the right root import path, and corresponding directory beneath `$GOPATH/src`, at which to situate your project. There are four basic possibilities:

1. A project that is now or eventually may be shared with or imported by other projects/people. In this case, pick the import path corresponding to the VCS root of its intended network location, e.g., `$GOPATH/src/github.com/golang/dep`.
2. An entirely local project - one that you have no intention of pushing to a central server (like GitHub). In this case, any subdirectory beneath `$GOPATH/src` will do.
3. A project that needs to live within a large repository, such as a company monorepo. This may be possible, but gets more complicated. (Unfortunately, no docs on this yet - coming soon!)
4. Treat the entire GOPATH as a single project, where `$GOPATH/src` is the root. dep [does not currently support this](https://github.com/golang/dep/issues/417) - it needs a non-empty import path to treat as the root of your project's import namespace.

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

This would also be a good time to set up a version control, such as [git](https://git-scm.com/). While dep in no way requires version control for your project, it can make inspecting the changes made by normal dep operations easier. Plus, it's basically best practice #1 of modern software development!

At this point, we're initialized and ready to start writing code! You can open up a `.go` file in an editor and start hacking away. Or, if you already know some projects you'll need, you can pre-populate your `vendor` directory with them:

```
$ dep ensure -add github.com/foo/bar github.com/baz/quux
```

Great, your project's all set up! You're ready to move on to [Daily Dep](daily-dep.md).