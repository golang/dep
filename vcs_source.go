package gps

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/sdboyer/gps/internal/fs"
	"github.com/sdboyer/gps/pkgtree"
)

type baseVCSSource struct {
	repo ctxRepo
}

func (bs *baseVCSSource) sourceType() string {
	return string(bs.repo.Vcs())
}

func (bs *baseVCSSource) existsLocally(ctx context.Context) bool {
	return bs.repo.CheckLocal()
}

// TODO reimpl for git
func (bs *baseVCSSource) existsUpstream(ctx context.Context) bool {
	return !bs.repo.Ping()
}

func (bs *baseVCSSource) upstreamURL() string {
	return bs.repo.Remote()
}

func (bs *baseVCSSource) getManifestAndLock(ctx context.Context, pr ProjectRoot, r Revision, an ProjectAnalyzer) (Manifest, Lock, error) {
	err := bs.repo.updateVersion(ctx, r.String())
	if err != nil {
		return nil, nil, unwrapVcsErr(err)
	}

	m, l, err := an.DeriveManifestAndLock(bs.repo.LocalPath(), pr)
	if err != nil {
		return nil, nil, err
	}

	if l != nil && l != Lock(nil) {
		l = prepLock(l)
	}

	return prepManifest(m), l, nil
}

func (bs *baseVCSSource) revisionPresentIn(r Revision) (bool, error) {
	return bs.repo.IsReference(string(r)), nil
}

// initLocal clones/checks out the upstream repository to disk for the first
// time.
func (bs *baseVCSSource) initLocal(ctx context.Context) error {
	err := bs.repo.get(ctx)

	if err != nil {
		return unwrapVcsErr(err)
	}
	return nil
}

// updateLocal ensures the local data (versions and code) we have about the
// source is fully up to date with that of the canonical upstream source.
func (bs *baseVCSSource) updateLocal(ctx context.Context) error {
	err := bs.repo.fetch(ctx)

	if err != nil {
		return unwrapVcsErr(err)
	}
	return nil
}

func (bs *baseVCSSource) listPackages(ctx context.Context, pr ProjectRoot, r Revision) (ptree pkgtree.PackageTree, err error) {
	err = bs.repo.updateVersion(ctx, r.String())

	if err != nil {
		err = unwrapVcsErr(err)
	} else {
		ptree, err = pkgtree.ListPackages(bs.repo.LocalPath(), string(pr))
	}

	return
}

func (bs *baseVCSSource) exportRevisionTo(ctx context.Context, r Revision, to string) error {
	// Only make the parent dir, as CopyDir will balk on trying to write to an
	// empty but existing dir.
	if err := os.MkdirAll(filepath.Dir(to), 0777); err != nil {
		return err
	}

	if err := bs.repo.updateVersion(ctx, r.String()); err != nil {
		return unwrapVcsErr(err)
	}

	// TODO(sdboyer) this is a simplistic approach and relying on the tools
	// themselves might make it faster, but git's the overwhelming case (and has
	// its own method) so fine for now
	return fs.CopyDir(bs.repo.LocalPath(), to)
}

// gitSource is a generic git repository implementation that should work with
// all standard git remotes.
type gitSource struct {
	baseVCSSource
}

func (s *gitSource) exportRevisionTo(ctx context.Context, rev Revision, to string) error {
	r := s.repo

	if err := os.MkdirAll(to, 0777); err != nil {
		return err
	}

	// Back up original index
	idx, bak := filepath.Join(r.LocalPath(), ".git", "index"), filepath.Join(r.LocalPath(), ".git", "origindex")
	err := fs.RenameWithFallback(idx, bak)
	if err != nil {
		return err
	}

	// could have an err here...but it's hard to imagine how?
	defer fs.RenameWithFallback(bak, idx)

	out, err := runFromRepoDir(ctx, r, "git", "read-tree", rev.String())
	if err != nil {
		return fmt.Errorf("%s: %s", out, err)
	}

	// Ensure we have exactly one trailing slash
	to = strings.TrimSuffix(to, string(os.PathSeparator)) + string(os.PathSeparator)
	// Checkout from our temporary index to the desired target location on
	// disk; now it's git's job to make it fast.
	//
	// Sadly, this approach *does* also write out vendor dirs. There doesn't
	// appear to be a way to make checkout-index respect sparse checkout
	// rules (-a supercedes it). The alternative is using plain checkout,
	// though we have a bunch of housekeeping to do to set up, then tear
	// down, the sparse checkout controls, as well as restore the original
	// index and HEAD.
	out, err = runFromRepoDir(ctx, r, "git", "checkout-index", "-a", "--prefix="+to)
	if err != nil {
		return fmt.Errorf("%s: %s", out, err)
	}

	return nil
}

func (s *gitSource) listVersions(ctx context.Context) (vlist []PairedVersion, err error) {
	r := s.repo

	var out []byte
	c := newMonitoredCmd(exec.Command("git", "ls-remote", r.Remote()), 30*time.Second)
	// Ensure no prompting for PWs
	c.cmd.Env = mergeEnvLists([]string{"GIT_ASKPASS=", "GIT_TERMINAL_PROMPT=0"}, os.Environ())
	out, err = c.combinedOutput(ctx)

	if err != nil {
		return nil, err
	}

	all := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	if len(all) == 1 && len(all[0]) == 0 {
		return nil, fmt.Errorf("no data returned from ls-remote")
	}

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
	vlist = make([]PairedVersion, len(all)-1) // less 1, because always ignore HEAD
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
	major uint64
}

func (s *gopkginSource) listVersions(ctx context.Context) ([]PairedVersion, error) {
	ovlist, err := s.gitSource.listVersions(ctx)
	if err != nil {
		return nil, err
	}

	// Apply gopkg.in's filtering rules
	vlist := make([]PairedVersion, len(ovlist))
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
			// exact if real problems crop up. The most obvious vector for
			// problems is that we totally ignore the "unstable" designation
			// right now.
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
		dbv := vlist[dbranch].(versionPair)
		vlist[dbranch] = branchVersion{
			name:      dbv.v.(branchVersion).name,
			isDefault: true,
		}.Is(dbv.r)
	}

	return vlist, nil
}

// bzrSource is a generic bzr repository implementation that should work with
// all standard bazaar remotes.
type bzrSource struct {
	baseVCSSource
}

func (s *bzrSource) listVersions(ctx context.Context) ([]PairedVersion, error) {
	r := s.repo

	// Now, list all the tags
	out, err := runFromRepoDir(ctx, r, "bzr", "tags", "--show-ids", "-v")
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, string(out))
	}

	all := bytes.Split(bytes.TrimSpace(out), []byte("\n"))

	var branchrev []byte
	branchrev, err = runFromRepoDir(ctx, r, "bzr", "version-info", "--custom", "--template={revision_id}", "--revision=branch:.")
	br := string(branchrev)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, br)
	}

	vlist := make([]PairedVersion, 0, len(all)+1)

	// Now, all the tags.
	for _, line := range all {
		idx := bytes.IndexByte(line, 32) // space
		v := NewVersion(string(line[:idx]))
		r := Revision(bytes.TrimSpace(line[idx:]))
		vlist = append(vlist, v.Is(r))
	}

	// Last, add the default branch, hardcoding the visual representation of it
	// that bzr uses when operating in the workflow mode we're using.
	v := newDefaultBranch("(default)")
	vlist = append(vlist, v.Is(Revision(string(branchrev))))

	return vlist, nil
}

// hgSource is a generic hg repository implementation that should work with
// all standard mercurial servers.
type hgSource struct {
	baseVCSSource
}

func (s *hgSource) listVersions(ctx context.Context) ([]PairedVersion, error) {
	var vlist []PairedVersion

	r := s.repo
	// Now, list all the tags
	out, err := runFromRepoDir(ctx, r, "hg", "tags", "--debug", "--verbose")
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
	out, err = runFromRepoDir(ctx, r, "hg", "bookmarks", "--debug")
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
			var v PairedVersion
			if str == "@" {
				magicAt = true
				v = newDefaultBranch(str).Is(Revision(pair[1])).(PairedVersion)
			} else {
				v = NewBranch(str).Is(Revision(pair[1])).(PairedVersion)
			}
			vlist = append(vlist, v)
		}
	}

	out, err = runFromRepoDir(ctx, r, "hg", "branches", "-c", "--debug")
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
		var v PairedVersion
		if !magicAt && str == "default" {
			v = newDefaultBranch(str).Is(Revision(pair[1])).(PairedVersion)
		} else {
			v = NewBranch(str).Is(Revision(pair[1])).(PairedVersion)
		}
		vlist = append(vlist, v)
	}

	return vlist, nil
}

type repo struct {
	// Object for direct repo interaction
	r ctxRepo
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
