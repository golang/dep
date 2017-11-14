### Test Data

This directory contains artifacts that represent malformed repo archives. Its purpose is to ensure `dep` can recover from such corrupted repositories in specific test scenarios.

- `corrupt_dot_git_directory.tar`: is a repo with a corrupt `.git` directory. Dep can put a directory in such malformed state when a user hits `Ctrl+C` in the middle of a `dep init` process or others. `TestNewCtxRepoRecovery` uses this file to ensure recovery.
