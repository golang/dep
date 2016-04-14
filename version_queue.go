package vsolver

import (
	"fmt"
	"strings"
)

type failedVersion struct {
	v V
	f error
}

type versionQueue struct {
	ref                ProjectName
	pi                 []V
	fails              []failedVersion
	sm                 SourceManager
	failed             bool
	hasLock, allLoaded bool
}

func newVersionQueue(ref ProjectName, lockv *ProjectAtom, sm SourceManager) (*versionQueue, error) {
	vq := &versionQueue{
		ref: ref,
		sm:  sm,
	}

	if lockv != nil {
		vq.hasLock = true
		vq.pi = append(vq.pi, lockv.Version)
	} else {
		var err error
		vq.pi, err = vq.sm.ListVersions(vq.ref)
		if err != nil {
			// TODO pushing this error this early entails that we
			// unconditionally deep scan (e.g. vendor), as well as hitting the
			// network.
			return nil, err
		}
		vq.allLoaded = true
	}

	return vq, nil
}

func (vq *versionQueue) current() V {
	if len(vq.pi) > 0 {
		return vq.pi[0]
	}

	return Version{}
}

func (vq *versionQueue) advance(fail error) (err error) {
	// The current version may have failed, but the next one hasn't
	vq.failed = false

	if len(vq.pi) == 0 {
		return
	}

	vq.fails = append(vq.fails, failedVersion{
		v: vq.pi[0],
		f: fail,
	})
	if vq.allLoaded {
		vq.pi = vq.pi[1:]
		return
	}

	vq.allLoaded = true
	// Can only get here if no lock was initially provided, so we know we
	// should have that
	lockv := vq.pi[0]

	vq.pi, err = vq.sm.ListVersions(vq.ref)
	if err != nil {
		return
	}

	// search for and remove locked version
	// TODO should be able to avoid O(n) here each time...if it matters
	for k, pi := range vq.pi {
		if pi == lockv {
			// GC-safe deletion for slice w/pointer elements
			//vq.pi, vq.pi[len(vq.pi)-1] = append(vq.pi[:k], vq.pi[k+1:]...), nil
			vq.pi = append(vq.pi[:k], vq.pi[k+1:]...)
		}
	}

	// normal end of queue. we don't error; it's left to the caller to infer an
	// empty queue w/a subsequent call to current(), which will return an empty
	// item.
	// TODO this approach kinda...sucks
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
