# go-flock
[![TravisCI Build Status](https://img.shields.io/travis/theckman/go-flock/master.svg?style=flat)](https://travis-ci.org/theckman/go-flock)
[![GoDoc](https://img.shields.io/badge/godoc-go--flock-blue.svg?style=flat)](https://godoc.org/github.com/theckman/go-flock)
[![License](https://img.shields.io/badge/license-BSD_3--Clause-brightgreen.svg?style=flat)](https://github.com/theckman/go-flock/blob/master/LICENSE)

`flock` implements a thread-safe sync.Locker interface for file locking. It also
includes a non-blocking TryLock() function to allow locking without blocking execution.

## License
`flock` is released under the BSD 3-Clause License. See the `LICENSE` file for more details.

## Intsallation
```
go get -u github.com/theckman/go-flock
```

## Usage
```Go
import "github.com/theckman/go-flock"

fileLock := flock.NewFlock("/var/lock/go-lock.lock")

locked, err := fileLock.TryLock()

if err != nil {
	// handle locking error
}

if locked {
	// do work
	fileLock.Unlock()
}
```

For more detailed usage information take a look at the package API docs on
[GoDoc](https://godoc.org/github.com/theckman/go-flock).
