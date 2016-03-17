package vsolver

type VersionQueue struct {
	ref                ProjectIdentifier
	pi                 []ProjectID
	failed             bool
	hasLock, allLoaded bool
	sm                 SourceManager
}

func NewVersionQueue(ref ProjectIdentifier, lockv *ProjectID, sm SourceManager) (*VersionQueue, error) {
	vq := &VersionQueue{
		ref: ref,
		sm:  sm,
	}

	if lockv != nil {
		vq.hasLock = true
		vq.pi = append(vq.pi, *lockv)
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

func (vq *VersionQueue) current() ProjectID {
	if len(vq.pi) > 0 {
		return vq.pi[0]
	}

	return ProjectID{}
}

func (vq *VersionQueue) advance() (err error) {
	// The current version may have failed, but the next one hasn't
	vq.failed = false

	if !vq.allLoaded {
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
	}

	if len(vq.pi) > 0 {
		vq.pi = vq.pi[1:]
	}

	// normal end of queue. we don't error; it's left to the caller to infer an
	// empty queue w/a subsequent call to current(), which will return nil.
	// TODO this approach kinda...sucks
	return
}
