package gps

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Masterminds/vcs"
	"github.com/termie/go-shutil"
)

type vcsSource interface {
	syncLocal() error
	listLocalVersionPairs() ([]PairedVersion, sourceExistence, error)
	listUpstreamVersionPairs() ([]PairedVersion, sourceExistence, error)
	revisionPresentIn(Revision) (bool, error)
	checkout(Version) error
	ping() bool
	ensureCacheExistence() error
}

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
	_, err = r.RunFromDir("git", "read-tree", vstr)
	if err != nil {
		return err
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
	_, err = r.RunFromDir("git", "checkout-index", "-a", "--prefix="+to)
	return err
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

	smap := make(map[string]bool)
	uniq := 0
	vlist = make([]Version, len(all)-1) // less 1, because always ignore HEAD
	for _, pair := range all {
		var v PairedVersion
		if string(pair[46:51]) == "heads" {
			v = NewBranch(string(pair[52:])).Is(Revision(pair[:40])).(PairedVersion)
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

	// Process the version data into the cache
	//
	// reset the rmap and vmap, as they'll be fully repopulated by this
	// TODO(sdboyer) detect out-of-sync pairings as we do this?
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
		return
	}

	all := bytes.Split(bytes.TrimSpace(out), []byte("\n"))

	// reset the rmap and vmap, as they'll be fully repopulated by this
	// TODO(sdboyer) detect out-of-sync pairings as we do this?
	s.dc.vMap = make(map[UnpairedVersion]Revision)
	s.dc.rMap = make(map[Revision][]UnpairedVersion)

	vlist = make([]Version, len(all))
	k := 0
	for _, line := range all {
		idx := bytes.IndexByte(line, 32) // space
		v := NewVersion(string(line[:idx]))
		r := Revision(bytes.TrimSpace(line[idx:]))

		s.dc.vMap[v] = r
		s.dc.rMap[r] = append(s.dc.rMap[r], v)
		vlist[k] = v.Is(r)
		k++
	}

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
		return
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

	out, err = r.RunFromDir("hg", "branches", "--debug", "--verbose")
	if err != nil {
		// better nothing than partial and misleading
		vlist = nil
		return
	}

	all = bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	lbyt = []byte("(inactive)")
	for _, line := range all {
		if bytes.Equal(lbyt, line[len(line)-len(lbyt):]) {
			// Skip inactive branches
			continue
		}

		// Split on colon; this gets us the rev and the branch plus local revno
		pair := bytes.Split(line, []byte(":"))
		idx := bytes.IndexByte(pair[0], 32) // space
		v := NewBranch(string(pair[0][:idx])).Is(Revision(pair[1])).(PairedVersion)
		vlist = append(vlist, v)
	}

	// reset the rmap and vmap, as they'll be fully repopulated by this
	// TODO(sdboyer) detect out-of-sync pairings as we do this?
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

func (r *repo) getCurrentVersionPairs() (vlist []PairedVersion, exbits sourceExistence, err error) {
	r.mut.Lock()
	defer r.mut.Unlock()

	switch r.r.(type) {
	case *vcs.GitRepo:
		var out []byte
		c := exec.Command("git", "ls-remote", r.r.Remote())
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
			err = r.r.Update()
			if err != nil {
				// Definitely have a problem, now - bail out
				return
			}

			// Upstream and cache must exist, so add that to exbits
			exbits |= existsUpstream | existsInCache
			// Also, local is definitely now synced
			r.synced = true

			out, err = r.r.RunFromDir("git", "show-ref", "--dereference")
			if err != nil {
				return
			}

			all = bytes.Split(bytes.TrimSpace(out), []byte("\n"))
		}
		// Local cache may not actually exist here, but upstream definitely does
		exbits |= existsUpstream

		tmap := make(map[string]PairedVersion)
		for _, pair := range all {
			var v PairedVersion
			if string(pair[46:51]) == "heads" {
				v = NewBranch(string(pair[52:])).Is(Revision(pair[:40])).(PairedVersion)
				vlist = append(vlist, v)
			} else if string(pair[46:50]) == "tags" {
				vstr := string(pair[51:])
				if strings.HasSuffix(vstr, "^{}") {
					// If the suffix is there, then we *know* this is the rev of
					// the underlying commit object that we actually want
					vstr = strings.TrimSuffix(vstr, "^{}")
				} else if _, exists := tmap[vstr]; exists {
					// Already saw the deref'd version of this tag, if one
					// exists, so skip this.
					continue
					// Can only hit this branch if we somehow got the deref'd
					// version first. Which should be impossible, but this
					// covers us in case of weirdness, anyway.
				}
				v = NewVersion(vstr).Is(Revision(pair[:40])).(PairedVersion)
				tmap[vstr] = v
			}
		}

		// Append all the deref'd (if applicable) tags into the list
		for _, v := range tmap {
			vlist = append(vlist, v)
		}
	case *vcs.BzrRepo:
		var out []byte
		// Update the local first
		err = r.r.Update()
		if err != nil {
			return
		}
		// Upstream and cache must exist, so add that to exbits
		exbits |= existsUpstream | existsInCache
		// Also, local is definitely now synced
		r.synced = true

		// Now, list all the tags
		out, err = r.r.RunFromDir("bzr", "tags", "--show-ids", "-v")
		if err != nil {
			return
		}

		all := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
		for _, line := range all {
			idx := bytes.IndexByte(line, 32) // space
			v := NewVersion(string(line[:idx])).Is(Revision(bytes.TrimSpace(line[idx:]))).(PairedVersion)
			vlist = append(vlist, v)
		}

	case *vcs.HgRepo:
		var out []byte
		err = r.r.Update()
		if err != nil {
			return
		}

		// Upstream and cache must exist, so add that to exbits
		exbits |= existsUpstream | existsInCache
		// Also, local is definitely now synced
		r.synced = true

		out, err = r.r.RunFromDir("hg", "tags", "--debug", "--verbose")
		if err != nil {
			return
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

		out, err = r.r.RunFromDir("hg", "branches", "--debug", "--verbose")
		if err != nil {
			// better nothing than incomplete
			vlist = nil
			return
		}

		all = bytes.Split(bytes.TrimSpace(out), []byte("\n"))
		lbyt = []byte("(inactive)")
		for _, line := range all {
			if bytes.Equal(lbyt, line[len(line)-len(lbyt):]) {
				// Skip inactive branches
				continue
			}

			// Split on colon; this gets us the rev and the branch plus local revno
			pair := bytes.Split(line, []byte(":"))
			idx := bytes.IndexByte(pair[0], 32) // space
			v := NewBranch(string(pair[0][:idx])).Is(Revision(pair[1])).(PairedVersion)
			vlist = append(vlist, v)
		}
	case *vcs.SvnRepo:
		// TODO(sdboyer) is it ok to return empty vlist and no error?
		// TODO(sdboyer) ...gotta do something for svn, right?
	default:
		panic("unknown repo type")
	}

	return
}

func (r *repo) exportVersionTo(v Version, to string) error {
	r.mut.Lock()
	defer r.mut.Unlock()

	switch r.r.(type) {
	case *vcs.GitRepo:
		// Back up original index
		idx, bak := filepath.Join(r.rpath, ".git", "index"), filepath.Join(r.rpath, ".git", "origindex")
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
		_, err = r.r.RunFromDir("git", "read-tree", vstr)
		if err != nil {
			return err
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
		_, err = r.r.RunFromDir("git", "checkout-index", "-a", "--prefix="+to)
		return err
	default:
		// TODO(sdboyer) This is a dumb, slow approach, but we're punting on making these
		// fast for now because git is the OVERWHELMING case
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
