## Local repositories

This directory is used by the testing system to set up local repository
fixtures that stand in for repositories ordinarily accessible only over teh
network. These local fixtures can be transparently swapped in, allowing tests
to operate locally, drastically increasing both speed and reliability of tests.

Additionally, the these repositories being strictly local allows us to safely
mutate their state in order to simulate upstream repository changes as part of
testing. This is crucial for probing the behavior of the SourceMgr in the
presence of upstream changes.
