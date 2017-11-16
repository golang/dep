# v0.3.3 (Unreleased)

BUG FIXES:

* Releases targeting Windows now have a `.exe` suffix (#1291).
* Adaptively recover from dirty and corrupted git repositories in cache (#1279).
* Suppress git password prompts in more places (#1357).
* Validate `git ls-remote` output and ignore all malformed lines (#1379)

IMPROVEMENTS:

* Log as dependencies are pre-fetched during dep init ([#1176](https://github.com/golang/dep/pull/1176)).
* Make the gps package importable ([#1349](https://github.com/golang/dep/pull/1349)).

# v0.3.2

NEW FEATURES:

* Add support for importing from [gvt](https://github.com/FiloSottile/gvt)
and [gb](https://godoc.org/github.com/constabulary/gb/cmd/gb-vendor).
(#1149)
* Wildcard ignore support. (#1156)
* Disable SourceManager lock by setting `DEPNOLOCK` environment variable.
(#1206)
* `dep ensure -no-vendor -dry-run` now exits with an error when changes would
have to be made to `Gopkg.lock`. This is useful for CI. (#1256)

BUG FIXES:

* gps: Fix case mismatch error with multiple dependers. (#1233)
* Skip broken `vendor` symlink rather than returning an error. (#1191)
* Fix `status` shows incorrect reason for lock mismatch when ignoring packages.
(#1216)

IMPROVEMENTS:

* Allow `dep ensure -add` and `-update` when lock is out-of-sync. (#1225)
* gps: vcs: Dedupe git version list (#1212)
* gps: Add prune functions to gps. (#1020)
* gps: Skip broken vendor symlinks. (#1191)
* `dep ensure -add` now concurrently fetches the source and adds the projects.
(#1218)
* File name case check is now performed on `Gopkg.toml` and `Gopkg.lock`.
(#1114)
* gps: gps now supports pruning. (#1020)
* `dep ensure -update` now concurrently validates the passed project arguments.
Improving performance when updating dependencies with `-update`. (#1175)
* `dep status` now concurrently fetches repo info. Improving status performance.
(#1135)
* gps: Add SourceURLsForPath() to SourceManager. (#1166)
* gps: Include output in error. (#1180)

WIP:

* gps: Process canonical import paths. (#1017)
* gps: Persistent cache. (#1127, #1215)


# v0.3.1

* gps: Add satisfiability check for case variants (#1079)
* Validate Project Roots in manifest (#1116)
* gps: Properly separate sources for different gopkg.in versions & github
(#1132)
* gps: Add persistent BoltDB cache (#1098)
* gps: Increase default subcommand timeout to 30s (#1087)
* Fix importer [issue](https://github.com/golang/dep/issues/939) where the
importer would drop the imported version of a project (#1100)
* Import analyzer now always uses the same name, fixing the lock mismatch
immediately after dep init issue (#1099)
* Add support for importing from [govend](https://github.com/govend/govend)
(#1040) and [LK4D4/vndr](https://github.com/LK4D4/vndr) (#978) based projects
* gps: gps no longer assumes that every git repo has a HEAD (#1053)
* `os.Chmod` failures on Windows due to long path length has been fixed (#925)
* Add `version` command (#996)
* Drop support for building with go1.7 (#714)
* gps: Parse abbreviated git revisions (#1027)
* gps: Parallelize writing dep tree (#1021)
* `status` now shows the progress in verbose mode (#1009, #1037)
* Fix empty `Constraint` and `Version` in `status` json output (#976)
* `status` table output now shows override constraints (#918)
* gps: Display warning message every 15 seconds when lockfile is busy (#958)
* gps: Hashing directory tree and tree verification (#959)
* `ensure` now has `-vendor-only` mode to populate vendor/ without updating
Gopkg.lock (#954)
* Use fork of Masterminds/semver until
Masterminds/semver [issue#59](https://github.com/Masterminds/semver/issues/59)
is fixed upstream (#938)
* gps: Ensure packages are deducible before attempting to solve (#697)
