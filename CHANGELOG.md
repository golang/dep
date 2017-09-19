# v0.3.2 (Unreleased)

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
