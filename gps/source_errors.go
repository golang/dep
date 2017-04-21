package gps

import (
	"fmt"

	"github.com/Masterminds/vcs"
)

// unwrapVcsErr will extract actual command output from a vcs err, if possible
//
// TODO this is really dumb, lossy, and needs proper handling
func unwrapVcsErr(err error) error {
	switch verr := err.(type) {
	case *vcs.LocalError:
		return fmt.Errorf("%s: %s", verr.Error(), verr.Out())
	case *vcs.RemoteError:
		return fmt.Errorf("%s: %s", verr.Error(), verr.Out())
	default:
		return err
	}
}
