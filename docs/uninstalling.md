---
title: Uninstalling Dep
---

## Uninstalling

To uninstall `dep` itself, follow these instructions, depending on how you installed `dep` originally.

### If you installed `dep` by executing the `install.sh` script via curl

If you installed `dep` using the `install.sh` script, it is safe to simply delete the installed binary file.

On Linux and MacOS, the `install.sh` script installs a pre-compiled binary to `$GOPATH/bin/dep`. It is safe to simply `rm` the installed `$GOPATH/bin/dep` file:

```sh
$ rm $GOPATH/bin/dep
```

On Windows, the `install.sh` script installs a pre-compiled binary to `$GOPATH/bin/dep.exe`. It is safe to simply delete this file to uninstall `dep`.

### If you installed `dep` using Homebrew on MacOS

If you installed `dep` using Homebrew on MacOS, uninstall `dep` also using Homebrew:

```sh
$ brew uninstall dep
```

### If you installed `dep` using `pacman` on Arch Linux

If you installed `dep` using `pacman` on Arch Linux, uninstall `dep` like so:

```sh
$ pacman -R dep
```
