// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sdboyer/gps"
)

var removeCmd = &command{
	fn:   runRemove,
	name: "rm",
	short: `[flags] [packages]
	Remove a package or a set of packages.
	`,
	long: `
Run it when:
To stop using dependencies
To clean out unused dependencies

What it does
Removes the given dependency from the Manifest, Lock, and vendor/.
If the current project includes that dependency in its import graph, rm will fail unless -force is specified.
If -unused is provided, specs matches all dependencies in the Manifest that are not reachable by the import graph.
The -force and -unused flags cannot be combined (an error occurs).
During removal, dependencies that were only present because of the dependencies being removed are also removed.

Note: this is a separate command to 'ensure' because we want the user to be explicit when making destructive changes.

Flags:
-n		Dry run, don’t actually remove anything
-unused	Remove dependencies that are not used by this project
-force		Remove dependency even if it is used by the project
-keep-source	Do not remove source code
	`,
}

func runRemove(args []string) error {
	p, err := depContext.loadProject("")
	if err != nil {
		return err
	}

	sm, err := depContext.sourceManager()
	if err != nil {
		return err
	}
	defer sm.Release()

	cpr, err := depContext.splitAbsoluteProjectRoot(p.absroot)
	if err != nil {
		return errors.Wrap(err, "determineProjectRoot")
	}
	fmt.Printf("cpr: %#v\n", cpr)

	pkgT, err := gps.ListPackages(p.absroot, cpr)
	if err != nil {
		return errors.Wrap(err, "gps.ListPackages")
	}
	fmt.Printf("package tree: %#v\n", pkgT)

	// get the list of packages
	packages, _, _, _, err := getProjectDependencies(pkgT, cpr, sm)
	if err != nil {
		return err
	}

	for _, arg := range args {
		/*
		 * - Remove package from manifest
		 *	- if the package IS NOT being used, solving should do what we want
		 *	- if the package IS being used:
		 *		- Desired behavior: stop and tell the user, unless --force
		 *		- Actual solver behavior: ?
		 */

		// check if we are using the package
		_, using := packages[gps.ProjectRoot(arg)]
		fmt.Printf("using package %s: %t\n", arg, using)
	}
	fmt.Printf("packages: %#v", packages)

	return nil
}
