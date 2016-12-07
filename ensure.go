// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

var ensureCmd = &command{
	fn:   runEnsure,
	name: "ensure",
	short: `[flags] <path>[:alt location][@<version specifier>]
	To ensure a dependency is in your project at a specific version (if specified).
	`,
	long: `
	Run it when
To ensure a new dependency is in your project.
To ensure a dependency is updated.
To the latest version that satisfies constraints.
To a specific version or different constraint.
(With no arguments) To ensure that you have all the dependencies specified by your Manifest + lockfile.


What it does
Download code, placing in the vendor/ directory of the project. Only the packages that are actually used by the current project and its dependencies are included.
Any authentication, proxy settings, or other parameters regarding communicating with external repositories is the responsibility of the underlying VCS tools.
Resolve version constraints
If the set of constraints are not solvable, print an error
Collapse any vendor folders in the downloaded code and its transient deps to the root.
Includes dependencies required by the current project’s tests. There are arguments both for and against including the deps of tests for any transitive deps. Defer on deciding this for now.
Copy the relevant versions of the code to the current project’s vendor directory.
The source of that code is implementation dependant. Probably some kind of local cache of the VCS data (not a GOPATH/workspace).
Write Manifest (if changed) and Lockfile
Print what changed


Flags:
	-update		update all packages
	-n			dry run
	-override <specs>	specify an override constraints for package(s)


Package specs:
	<path>[:alt location][@<version specifier>]


Examples:
Fetch/update github.com/heroku/rollrus to latest version, including transitive dependencies (ensuring it matches the constraints of rollrus, or—if not contrained—their latest versions):
	$ dep ensure github.com/heroku/rollrus
Same dep, but choose any minor patch release in the 0.9.X series, setting the constraint. If another constraint exists that constraint is changed to ~0.9.0:
	$ dep ensure github.com/heroku/rollrus@~0.9.0
Same dep, but choose any release >= 0.9.1 and < 1.0.0, setting/changing constraints:
	$ dep ensure github.com/heroku/rollrus@^0.9.1
Same dep, but updating to 1.0.X:
	$ dep ensure github.com/heroku/rollrus@~1.0.0
Same dep, but fetching from a different location:
	$ dep ensure github.com/heroku/rollrus:git.example.com/foo/bar
Same dep, but check out a specific version or range without updating the Manifest and update the Lockfile. This will fail if the specified version does not satisfy any existing constraints:
	$ dep ensure github.com/heroku/rollrus==1.2.3	# 1.2.3 specifically
	$ dep ensure github.com/heroku/rollrus=^1.2.0	# >= 1.2.0  < 2.0.0
Override any declared dependency range of 'github.com/foo/bar' to have the range of '^0.9.1'. This applies transitively:
	$ dep ensure -override github.com/foo/bar@^0.9.1


Transitive deps are ensured based on constraints in the local Manifest if they exist, then constraints in the dependency’s Manifest file. A lack of constraints defaults to the latest version, eg "^2".


For a description of the version specifier string, see this handy guide from crates.io. We are going to defer on making a final decision about this syntax until we have more experience with it in practice.
	`,
}

func runEnsure(args []string) error {
	return nil
}
