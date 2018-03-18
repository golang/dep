# Dep with a local file registry

## Use case: vendoring `foo.com/bar/*`

Add the following to `Gopkg.reg`:

```
^foo\.com/bar/([^/]*)(/.*)?$ foo.com/bar/$1 ssh://git@repo.internal:7999/my_mirror/$1
```

Here:

- the first field `^foo\.com/bar/([^/]*)(/.*)?$` matches import paths
- the second field `foo.com/bar/$1` gives the resulting repo base path
- the third field `ssh://git@repo.interal:7999/my_mirror/$1` gives the location to clone.

Lines are matched on a first-match-wins basis.

Subpackages work as expected.

## use case: mirroring everything into a local proxy repo

(Untested: I've only needed the first version for faas at present, but you might find this handy.)

```
^github.com/([^/]*)(/.*)?$ github.com/$1 https://my.proxy/github.com/$1
```

# Docker files

The various docker files should build libs and alpine versions of golang 1.9 and 1.10 with the modified
`dep` command installed in `$GOBIN`.

    docker build -t my-dep-image -f Dockerfile.alpine-1.10 .



## Dep itself

If you're reading this you probably already know what dep is. See https://github.com/golang/dep for details.
