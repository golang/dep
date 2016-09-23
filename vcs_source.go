package gps

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Masterminds/semver"
	"github.com/Masterminds/vcs"
	"github.com/termie/go-shutil"
)

// Kept here as a reference in case it does become important to implement a
// vcsSource interface. Remove if/when it becomes clear we're never going to do
// this.
//type vcsSource interface {
//syncLocal() error
//ensureLocal() error
//listLocalVersionPairs() ([]PairedVersion, sourceExistence, error)
//listUpstreamVersionPairs() ([]PairedVersion, sourceExistence, error)
//hasRevision(Revision) (bool, error)
//checkout(Version) error
//exportVersionTo(Version, string) error
//}

// gitSource is a generic git repository implementation that should work with
// all standard git remotes.
type gitSource struct {
	baseVCSSource
}

func (s *gitSource) exportVersionTo(v Version, to string) error {
	s.crepo.mut.Lock()
	defer s.crepo.mut.Unlock()

	r := s.crepo.r
	if !r.CheckLocal() {
		err := r.Get()
		if err != nil {
			return fmt.Errorf("failed to clone repo from %s", r.Remote())
		}
	}
	// Back up original index
	idx, bak := filepath.Join(r.LocalPath(), ".git", "index"), filepath.Join(r.LocalPath(), ".git", "origindex")
	err := os.Rename(idx, bak)
	if err != nil {
		return err
	}

	// TODO(sdboyer) could have an err here
	defer os.Rename(bak, idx)

	vstr := v.String()
	if rv, ok := v.(PairedVersion); ok {
		vstr = rv.Underlying().String()
	}

	out, err := r.RunFromDir("git", "read-tree", vstr)
	if err != nil {
		return fmt.Errorf("%s: %s", out, err)
	}

	// Ensure we have exactly one trailing slash
	to = strings.TrimSuffix(to, string(os.PathSeparator)) + string(os.PathSeparator)
	// Checkout from our temporary index to the desired target location on disk;
	// now it's git's job to make it fast. Sadly, this approach *does* also
	// write out vendor dirs. There doesn't appear to be a way to make
	// checkout-index respect sparse checkout rules (-a supercedes it);
	// the alternative is using plain checkout, though we have a bunch of
	// housekeeping to do to set up, then tear down, the sparse checkout
	// controls, as well as restore the original index and HEAD.
	out, err = r.RunFromDir("git", "checkout-index", "-a", "--prefix="+to)
	if err != nil {
		return fmt.Errorf("%s: %s", out, err)
	}
	return nil
}

func (s *gitSource) listVersions() (vlist []Version, err error) {
	if s.cvsync {
		vlist = make([]Version, len(s.dc.vMap))
		k := 0
		for v, r := range s.dc.vMap {
			vlist[k] = v.Is(r)
			k++
		}

		return
	}

	vlist, err = s.doListVersions()
	if err != nil {
		return nil, err
	}

	// Process the version data into the cache
	//
	// reset the rmap and vmap, as they'll be fully repopulated by this
	s.dc.vMap = make(map[UnpairedVersion]Revision)
	s.dc.rMap = make(map[Revision][]UnpairedVersion)

	for _, v := range vlist {
		pv := v.(PairedVersion)
		u, r := pv.Unpair(), pv.Underlying()
		s.dc.vMap[u] = r
		s.dc.rMap[r] = append(s.dc.rMap[r], u)
	}
	// Mark the cache as being in sync with upstream's version list
	s.cvsync = true
	return
}

func (s *gitSource) doListVersions() (vlist []Version, err error) {
	r := s.crepo.r
	var out []byte
	c := exec.Command("git", "ls-remote", r.Remote())
	// Ensure no terminal prompting for PWs
	c.Env = mergeEnvLists([]string{"GIT_TERMINAL_PROMPT=0"}, os.Environ())
	out, err = c.CombinedOutput()

	all := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	if err != nil || len(all) == 0 {
		// TODO(sdboyer) remove this path? it really just complicates things, for
		// probably not much benefit

		// ls-remote failed, probably due to bad communication or a faulty
		// upstream implementation. So fetch updates, then build the list
		// locally
		s.crepo.mut.Lock()
		err = r.Update()
		s.crepo.mut.Unlock()
		if err != nil {
			// Definitely have a problem, now - bail out
			return
		}

		// Upstream and cache must exist for this to have worked, so add that to
		// searched and found
		s.ex.s |= existsUpstream | existsInCache
		s.ex.f |= existsUpstream | existsInCache
		// Also, local is definitely now synced
		s.crepo.synced = true

		s.crepo.mut.RLock()
		out, err = r.RunFromDir("git", "show-ref", "--dereference")
		s.crepo.mut.RUnlock()
		if err != nil {
			// TODO(sdboyer) More-er proper-er error
			return
		}

		all = bytes.Split(bytes.TrimSpace(out), []byte("\n"))
		if len(all) == 0 {
			return nil, fmt.Errorf("no versions available for %s (this is weird)", r.Remote())
		}
	}

	// Local cache may not actually exist here, but upstream definitely does
	s.ex.s |= existsUpstream
	s.ex.f |= existsUpstream

	// Pull out the HEAD rev (it's always first) so we know what branches to
	// mark as default. This is, perhaps, not the best way to glean this, but it
	// was good enough for git itself until 1.8.5. Also, the alternative is
	// sniffing data out of the pack protocol, which is a separate request, and
	// also waaaay more than we want to do right now.
	//
	// The cost is that we could potentially have multiple branches marked as
	// the default. If that does occur, a later check (again, emulating git
	// <1.8.5 behavior) further narrows the failure mode by choosing master as
	// the sole default branch if a) master exists and b) master is one of the
	// branches marked as a default.
	//
	// This all reduces the failure mode to a very narrow range of
	// circumstances. Nevertheless, if we do end up emitting multiple
	// default branches, it is possible that a user could end up following a
	// non-default branch, IF:
	//
	// * Multiple branches match the HEAD rev
	// * None of them are master
	// * The solver makes it into the branch list in the version queue
	// * The user/tool has provided no constraint (so, anyConstraint)
	// * A branch that is not actually the default, but happens to share the
	//   rev, is lexicographically less than the true default branch
	//
	// If all of those conditions are met, then the user would end up with an
	// erroneous non-default branch in their lock file.
	headrev := Revision(all[0][:40])
	var onedef, multidef, defmaster bool

	smap := make(map[string]bool)
	uniq := 0
	vlist = make([]Version, len(all)-1) // less 1, because always ignore HEAD
	for _, pair := range all {
		var v PairedVersion
		if string(pair[46:51]) == "heads" {
			rev := Revision(pair[:40])

			isdef := rev == headrev
			n := string(pair[52:])
			if isdef {
				if onedef {
					multidef = true
				}
				onedef = true
				if n == "master" {
					defmaster = true
				}
			}
			v = branchVersion{
				name:      n,
				isDefault: isdef,
			}.Is(rev).(PairedVersion)

			vlist[uniq] = v
			uniq++
		} else if string(pair[46:50]) == "tags" {
			vstr := string(pair[51:])
			if strings.HasSuffix(vstr, "^{}") {
				// If the suffix is there, then we *know* this is the rev of
				// the underlying commit object that we actually want
				vstr = strings.TrimSuffix(vstr, "^{}")
			} else if smap[vstr] {
				// Already saw the deref'd version of this tag, if one
				// exists, so skip this.
				continue
				// Can only hit this branch if we somehow got the deref'd
				// version first. Which should be impossible, but this
				// covers us in case of weirdness, anyway.
			}
			v = NewVersion(vstr).Is(Revision(pair[:40])).(PairedVersion)
			smap[vstr] = true
			vlist[uniq] = v
			uniq++
		}
	}

	// Trim off excess from the slice
	vlist = vlist[:uniq]

	// There were multiple default branches, but one was master. So, go through
	// and strip the default flag from all the non-master branches.
	if multidef && defmaster {
		for k, v := range vlist {
			pv := v.(PairedVersion)
			if bv, ok := pv.Unpair().(branchVersion); ok {
				if bv.name != "master" && bv.isDefault == true {
					bv.isDefault = false
					vlist[k] = bv.Is(pv.Underlying())
				}
			}
		}
	}

	return
}

// gopkginSource is a specialized git source that performs additional filtering
// according to the input URL.
type gopkginSource struct {
	gitSource
	major int64
}

func (s *gopkginSource) listVersions() (vlist []Version, err error) {
	if s.cvsync {
		vlist = make([]Version, len(s.dc.vMap))
		k := 0
		for v, r := range s.dc.vMap {
			vlist[k] = v.Is(r)
			k++
		}

		return
	}

	ovlist, err := s.doListVersions()
	if err != nil {
		return nil, err
	}

	// Apply gopkg.in's filtering rules
	vlist := make([]Version, len(ovlist))
	k := 0
	var dbranch int // index of branch to be marked default
	var bsv *semver.Version
	for _, v := range ovlist {
		// all git versions will always be paired
		pv := v.(versionPair)
		switch tv := pv.v.(type) {
		case semVersion:
			if tv.sv.Major() == s.major {
				vlist[k] = v
				k++
			}
		case branchVersion:
			// The semver lib isn't exactly the same as gopkg.in's logic, but
			// it's close enough that it's probably fine to use. We can be more
			// exact if real problems crop up.
			sv, err := semver.NewVersion(tv.name)
			if err != nil || sv.Major() != s.major {
				// not a semver-shaped branch name at all, or not the same major
				// version as specified in the import path constraint
				continue
			}

			// Turn off the default branch marker unconditionally; we can't know
			// which one to mark as default until we've seen them all
			tv.isDefault = false
			// Figure out if this is the current leader for default branch
			if bsv == nil || bsv.LessThan(sv) {
				bsv = sv
				dbranch = k
			}
			pv.v = tv
			vlist[k] = pv
			k++
		}
		// The switch skips plainVersions because they cannot possibly meet
		// gopkg.in's requirements
	}

	vlist = vlist[:k]
	if bsv != nil {
		vlist[dbranch].(versionPair).v.(branchVersion).isDefault = true
	}

	// Process the filtered version data into the cache
	//
	// reset the rmap and vmap, as they'll be fully repopulated by this
	s.dc.vMap = make(map[UnpairedVersion]Revision)
	s.dc.rMap = make(map[Revision][]UnpairedVersion)

	for _, v := range vlist {
		pv := v.(PairedVersion)
		u, r := pv.Unpair(), pv.Underlying()
		s.dc.vMap[u] = r
		s.dc.rMap[r] = append(s.dc.rMap[r], u)
	}
	// Mark the cache as being in sync with upstream's version list
	s.cvsync = true
	return
}

// bzrSource is a generic bzr repository implementation that should work with
// all standard bazaar remotes.
type bzrSource struct {
	baseVCSSource
}

func (s *bzrSource) listVersions() (vlist []Version, err error) {
	if s.cvsync {
		vlist = make([]Version, len(s.dc.vMap))
		k := 0
		for v, r := range s.dc.vMap {
			vlist[k] = v.Is(r)
			k++
		}

		return
	}

	// Must first ensure cache checkout's existence
	err = s.ensureCacheExistence()
	if err != nil {
		return
	}
	r := s.crepo.r

	// Local repo won't have all the latest refs if ensureCacheExistence()
	// didn't create it
	if !s.crepo.synced {
		s.crepo.mut.Lock()
		err = r.Update()
		s.crepo.mut.Unlock()
		if err != nil {
			return
		}

		s.crepo.synced = true
	}

	var out []byte
	// Now, list all the tags
	out, err = r.RunFromDir("bzr", "tags", "--show-ids", "-v")
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	all := bytes.Split(bytes.TrimSpace(out), []byte("\n"))

	var branchrev []byte
	branchrev, err = r.RunFromDir("bzr", "version-info", "--custom", "--template={revision_id}", "--revision=branch:.")
	br := string(branchrev)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, br)
	}

	// Both commands completed successfully, so there's no further possibility
	// of errors. That means it's now safe to reset the rmap and vmap, as
	// they're about to be fully repopulated.
	s.dc.vMap = make(map[UnpairedVersion]Revision)
	s.dc.rMap = make(map[Revision][]UnpairedVersion)
	vlist = make([]Version, len(all)+1)

	// Now, all the tags.
	for k, line := range all {
		idx := bytes.IndexByte(line, 32) // space
		v := NewVersion(string(line[:idx]))
		r := Revision(bytes.TrimSpace(line[idx:]))

		s.dc.vMap[v] = r
		s.dc.rMap[r] = append(s.dc.rMap[r], v)
		vlist[k] = v.Is(r)
	}

	// Last, add the default branch, hardcoding the visual representation of it
	// that bzr uses when operating in the workflow mode we're using.
	v := newDefaultBranch("(default)")
	rev := Revision(string(branchrev))
	s.dc.vMap[v] = rev
	s.dc.rMap[rev] = append(s.dc.rMap[rev], v)
	vlist[len(vlist)-1] = v.Is(rev)

	// Cache is now in sync with upstream's version list
	s.cvsync = true
	return
}

// hgSource is a generic hg repository implementation that should work with
// all standard mercurial servers.
type hgSource struct {
	baseVCSSource
}

func (s *hgSource) listVersions() (vlist []Version, err error) {
	if s.cvsync {
		vlist = make([]Version, len(s.dc.vMap))
		k := 0
		for v, r := range s.dc.vMap {
			vlist[k] = v.Is(r)
			k++
		}

		return
	}

	// Must first ensure cache checkout's existence
	err = s.ensureCacheExistence()
	if err != nil {
		return
	}
	r := s.crepo.r

	// Local repo won't have all the latest refs if ensureCacheExistence()
	// didn't create it
	if !s.crepo.synced {
		s.crepo.mut.Lock()
		err = r.Update()
		s.crepo.mut.Unlock()
		if err != nil {
			return
		}

		s.crepo.synced = true
	}

	var out []byte

	// Now, list all the tags
	out, err = r.RunFromDir("hg", "tags", "--debug", "--verbose")
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	all := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	lbyt := []byte("local")
	nulrev := []byte("0000000000000000000000000000000000000000")
	for _, line := range all {
		if bytes.Equal(lbyt, line[len(line)-len(lbyt):]) {
			// Skip local tags
			continue
		}

		// tip is magic, don't include it
		if bytes.HasPrefix(line, []byte("tip")) {
			continue
		}

		// Split on colon; this gets us the rev and the tag plus local revno
		pair := bytes.Split(line, []byte(":"))
		if bytes.Equal(nulrev, pair[1]) {
			// null rev indicates this tag is marked for deletion
			continue
		}

		idx := bytes.IndexByte(pair[0], 32) // space
		v := NewVersion(string(pair[0][:idx])).Is(Revision(pair[1])).(PairedVersion)
		vlist = append(vlist, v)
	}

	// bookmarks next, because the presence of the magic @ bookmark has to
	// determine how we handle the branches
	var magicAt bool
	out, err = r.RunFromDir("hg", "bookmarks", "--debug")
	if err != nil {
		// better nothing than partial and misleading
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	out = bytes.TrimSpace(out)
	if !bytes.Equal(out, []byte("no bookmarks set")) {
		all = bytes.Split(out, []byte("\n"))
		for _, line := range all {
			// Trim leading spaces, and * marker if present
			line = bytes.TrimLeft(line, " *")
			pair := bytes.Split(line, []byte(":"))
			// if this doesn't split exactly once, we have something weird
			if len(pair) != 2 {
				continue
			}

			// Split on colon; this gets us the rev and the branch plus local revno
			idx := bytes.IndexByte(pair[0], 32) // space
			// if it's the magic @ marker, make that the default branch
			str := string(pair[0][:idx])
			var v Version
			if str == "@" {
				magicAt = true
				v = newDefaultBranch(str).Is(Revision(pair[1])).(PairedVersion)
			} else {
				v = NewBranch(str).Is(Revision(pair[1])).(PairedVersion)
			}
			vlist = append(vlist, v)
		}
	}

	out, err = r.RunFromDir("hg", "branches", "-c", "--debug")
	if err != nil {
		// better nothing than partial and misleading
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	all = bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	for _, line := range all {
		// Trim inactive and closed suffixes, if present; we represent these
		// anyway
		line = bytes.TrimSuffix(line, []byte(" (inactive)"))
		line = bytes.TrimSuffix(line, []byte(" (closed)"))

		// Split on colon; this gets us the rev and the branch plus local revno
		pair := bytes.Split(line, []byte(":"))
		idx := bytes.IndexByte(pair[0], 32) // space
		str := string(pair[0][:idx])
		// if there was no magic @ bookmark, and this is mercurial's magic
		// "default" branch, then mark it as default branch
		var v Version
		if !magicAt && str == "default" {
			v = newDefaultBranch(str).Is(Revision(pair[1])).(PairedVersion)
		} else {
			v = NewBranch(str).Is(Revision(pair[1])).(PairedVersion)
		}
		vlist = append(vlist, v)
	}

	// reset the rmap and vmap, as they'll be fully repopulated by this
	s.dc.vMap = make(map[UnpairedVersion]Revision)
	s.dc.rMap = make(map[Revision][]UnpairedVersion)

	for _, v := range vlist {
		pv := v.(PairedVersion)
		u, r := pv.Unpair(), pv.Underlying()
		s.dc.vMap[u] = r
		s.dc.rMap[r] = append(s.dc.rMap[r], u)
	}

	// Cache is now in sync with upstream's version list
	s.cvsync = true
	return
}

type repo struct {
	// Path to the root of the default working copy (NOT the repo itself)
	rpath string

	// Mutex controlling general access to the repo
	mut sync.RWMutex

	// Object for direct repo interaction
	r vcs.Repo

	// Whether or not the cache repo is in sync (think dvcs) with upstream
	synced bool
}

func (r *repo) exportVersionTo(v Version, to string) error {
	r.mut.Lock()
	defer r.mut.Unlock()

	// TODO(sdboyer) This is a dumb, slow approach, but we're punting on making
	// these fast for now because git is the OVERWHELMING case (it's handled in
	// its own method)
	r.r.UpdateVersion(v.String())

	cfg := &shutil.CopyTreeOptions{
		Symlinks:     true,
		CopyFunction: shutil.Copy,
		Ignore: func(src string, contents []os.FileInfo) (ignore []string) {
			for _, fi := range contents {
				if !fi.IsDir() {
					continue
				}
				n := fi.Name()
				switch n {
				case "vendor", ".bzr", ".svn", ".hg":
					ignore = append(ignore, n)
				}
			}

			return
		},
	}

	return shutil.CopyTree(r.rpath, to, cfg)
}

// This func copied from Masterminds/vcs so we can exec our own commands
func mergeEnvLists(in, out []string) []string {
NextVar:
	for _, inkv := range in {
		k := strings.SplitAfterN(inkv, "=", 2)[0]
		for i, outkv := range out {
			if strings.HasPrefix(outkv, k) {
				out[i] = inkv
				continue NextVar
			}
		}
		out = append(out, inkv)
	}
	return out
}

func stripVendor(path string, info os.FileInfo, err error) error {
	if info.Name() == "vendor" {
		if _, err := os.Lstat(path); err == nil {
			if info.IsDir() {
				return removeAll(path)
			}
		}
	}

	return nil
}
