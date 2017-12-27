# Init - Transitive Constraint

[C](github.com/carolynvs/deptest-transcons-c)
has a bug in the latest release. I am an end-user of [A](github.com/carolynvs/deptest-transcons-a)
which transitively depends on C. A has a constraint on C which avoids a bad release of C.
End-users like me should be able to use A, and have `dep init` pick a version of C
that doesn't have the bug, without manually adding overrides.
