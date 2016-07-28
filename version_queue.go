package gps

import (
	"fmt"
	"strings"
)

type failedVersion struct {
	v Version
	f error
}

type versionQueue struct {
	id           ProjectIdentifier
	pi           []Version
	lockv, prefv Version
	fails        []failedVersion
	b            sourceBridge
	failed       bool
	allLoaded    bool
}

func newVersionQueue(id ProjectIdentifier, lockv, prefv Version, b sourceBridge) (*versionQueue, error) {
	vq := &versionQueue{
		id: id,
		b:  b,
	}

	// Lock goes in first, if present
	if lockv != nil {
		vq.lockv = lockv
		vq.pi = append(vq.pi, lockv)
	}

	// Preferred version next
	if prefv != nil {
		vq.prefv = prefv
		vq.pi = append(vq.pi, prefv)
	}

	if len(vq.pi) == 0 {
		var err error
		vq.pi, err = vq.b.ListVersions(vq.id)
		if err != nil {
			// TODO(sdboyer) pushing this error this early entails that we
			// unconditionally deep scan (e.g. vendor), as well as hitting the
			// network.
			return nil, err
		}
		vq.allLoaded = true
	}

	return vq, nil
}

func (vq *versionQueue) current() Version {
	if len(vq.pi) > 0 {
		return vq.pi[0]
	}

	return nil
}

// advance moves the versionQueue forward to the next available version,
// recording the failure that eliminated the current version.
func (vq *versionQueue) advance(fail error) (err error) {
	// Nothing in the queue means...nothing in the queue, nicely enough
	if len(vq.pi) == 0 {
		return
	}

	// Record the fail reason and pop the queue
	vq.fails = append(vq.fails, failedVersion{
		v: vq.pi[0],
		f: fail,
	})
	vq.pi = vq.pi[1:]

	// *now*, if the queue is empty, ensure all versions have been loaded
	if len(vq.pi) == 0 {
		if vq.allLoaded {
			// This branch gets hit when the queue is first fully exhausted,
			// after having been populated by ListVersions() on a previous
			// advance()
			return
		}

		vq.allLoaded = true
		vq.pi, err = vq.b.ListVersions(vq.id)
		if err != nil {
			return err
		}

		// search for and remove locked and pref versions
		//
		// could use the version comparator for binary search here to avoid
		// O(n) each time...if it matters
		for k, pi := range vq.pi {
			if pi == vq.lockv || pi == vq.prefv {
				// GC-safe deletion for slice w/pointer elements
				vq.pi, vq.pi[len(vq.pi)-1] = append(vq.pi[:k], vq.pi[k+1:]...), nil
				//vq.pi = append(vq.pi[:k], vq.pi[k+1:]...)
			}
		}

		if len(vq.pi) == 0 {
			// If listing versions added nothing (new), then return now
			return
		}
	}

	// We're finally sure that there's something in the queue. Remove the
	// failure marker, as the current version may have failed, but the next one
	// hasn't yet
	vq.failed = false

	// If all have been loaded and the queue is empty, we're definitely out
	// of things to try. Return empty, though, because vq semantics dictate
	// that we don't explicitly indicate the end of the queue here.
	return
}

// isExhausted indicates whether or not the queue has definitely been exhausted,
// in which case it will return true.
//
// It may return false negatives - suggesting that there is more in the queue
// when a subsequent call to current() will be empty. Plan accordingly.
func (vq *versionQueue) isExhausted() bool {
	if !vq.allLoaded {
		return false
	}
	return len(vq.pi) == 0
}

func (vq *versionQueue) String() string {
	var vs []string

	for _, v := range vq.pi {
		vs = append(vs, v.String())
	}
	return fmt.Sprintf("[%s]", strings.Join(vs, ", "))
}
