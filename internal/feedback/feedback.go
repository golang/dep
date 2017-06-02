// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package feedback

import (
	"fmt"
	"log"
)

// Constraint types
const ConsTypeConstraint = "constraint"
const ConsTypeHint = "hint"

// DepTypeDirect represents a direct dependency
const DepTypeDirect = "direct dep"

// DepTypeTransitive represents a transitive dependency,
// or a dependency of a dependency
const DepTypeTransitive = "transitive dep"

// DepTypeImported represents a dependency imported by an external tool
const DepTypeImported = "imported dep"

// ConstraintFeedback holds project constraint feedback data
type ConstraintFeedback struct {
	Version, LockedVersion, Revision, ConstraintType, DependencyType, ProjectPath string
}

// LogFeedback logs the feedback
func (cf ConstraintFeedback) LogFeedback(logger *log.Logger) {
	// "Using" feedback for direct dep
	if cf.DependencyType == DepTypeDirect || cf.DependencyType == DepTypeImported {
		ver := cf.Version
		// revision as version for hint
		if cf.ConstraintType == ConsTypeHint {
			ver = cf.Revision
		}
		logger.Printf("  %v", GetUsingFeedback(ver, cf.ConstraintType, cf.DependencyType, cf.ProjectPath))
	}
	// No "Locking" feedback for hints. "Locking" feedback only for constraint
	// and transitive dep
	if cf.ConstraintType != ConsTypeHint {
		logger.Printf("  %v", GetLockingFeedback(cf.LockedVersion, cf.Revision, cf.DependencyType, cf.ProjectPath))
	}
}

// GetUsingFeedback returns dependency using feedback string.
// Example:
// Using ^1.0.0 as constraint for direct dep github.com/foo/bar
// Using 1b8edb3 as hint for direct dep github.com/bar/baz
func GetUsingFeedback(version, consType, depType, projectPath string) string {
	if depType == DepTypeImported {
		return fmt.Sprintf("Using %s as initial %s for %s %s", version, consType, depType, projectPath)
	}
	return fmt.Sprintf("Using %s as %s for %s %s", version, consType, depType, projectPath)
}

// GetLockingFeedback returns dependency locking feedback string.
// Example:
// Locking in v1.1.4 (bc29b4f) for direct dep github.com/foo/bar
// Locking in master (436f39d) for transitive dep github.com/baz/qux
func GetLockingFeedback(version, revision, depType, projectPath string) string {
	if depType == DepTypeImported {
		return fmt.Sprintf("Trying %s (%s) as initial lock for %s %s", version, revision, depType, projectPath)
	}
	return fmt.Sprintf("Locking in %s (%s) for %s %s", version, revision, depType, projectPath)
}
