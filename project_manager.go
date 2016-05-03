package vsolver

import (
	"bytes"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Masterminds/vcs"
	"github.com/termie/go-shutil"
)

type ProjectManager interface {
	GetInfoAt(Version) (ProjectInfo, error)
	ListVersions() ([]Version, error)
	CheckExistence(ProjectExistence) bool
	ExportVersionTo(Version, string) error
}

type ProjectAnalyzer interface {
	GetInfo(build.Context, ProjectName) (Manifest, Lock, error)
}

type projectManager struct {
	n ProjectName
	// build.Context to use in any analysis, and to pass to the analyzer
	ctx build.Context
	// Top-level project vendor dir
	vendordir string
	// Object for the cache repository
	crepo *repo
	// Indicates the extent to which we have searched for, and verified, the
	// existence of the project/repo.
	ex existence
	// Analyzer, injected by way of the SourceManager and originally from the
	// sm's creator
	an ProjectAnalyzer
	// Whether the cache has the latest info on versions
	cvsync bool
	// The project metadata cache. This is persisted to disk, for reuse across
	// solver runs.
	dc *projectDataCache
}

type existence struct {
	// The existence levels for which a search/check has been performed
	s ProjectExistence
	// The existence levels verified to be present through searching
	f ProjectExistence
}

// TODO figure out shape of versions, then implement marshaling/unmarshaling
type projectDataCache struct {
	Version string                   `json:"version"` // TODO use this
	Infos   map[Revision]ProjectInfo `json:"infos"`
	VMap    map[Version]Revision     `json:"vmap"`
	RMap    map[Revision][]Version   `json:"rmap"`
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

func (pm *projectManager) GetInfoAt(v Version) (ProjectInfo, error) {
	// Technically, we could attempt to return straight from the metadata cache
	// even if the repo cache doesn't exist on disk. But that would allow weird
	// state inconsistencies (cache exists, but no repo...how does that even
	// happen?) that it'd be better to just not allow so that we don't have to
	// think about it elsewhere
	if !pm.CheckExistence(ExistsInCache) {
		return ProjectInfo{}, fmt.Errorf("Project repository cache for %s does not exist", pm.n)
	}

	if r, exists := pm.dc.VMap[v]; exists {
		if pi, exists := pm.dc.Infos[r]; exists {
			return pi, nil
		}
	}

	var err error
	if !pm.cvsync {
		err = pm.crepo.r.Update()
		if err != nil {
			return ProjectInfo{}, fmt.Errorf("Could not fetch latest updates into repository")
		}
		pm.cvsync = true
	}

	pm.crepo.mut.Lock()
	// Always prefer a rev, if it's available
	if pv, ok := v.(PairedVersion); ok {
		err = pm.crepo.r.UpdateVersion(pv.Underlying().String())
	} else {
		err = pm.crepo.r.UpdateVersion(v.String())
	}
	pm.crepo.mut.Unlock()
	if err != nil {
		// TODO More-er proper-er error
		panic(fmt.Sprintf("canary - why is checkout/whatever failing: %s", err))
	}

	pm.crepo.mut.RLock()
	m, l, err := pm.an.GetInfo(pm.ctx, pm.n)
	// TODO cache results
	pm.crepo.mut.RUnlock()

	if err == nil {
		return ProjectInfo{
			pa: ProjectAtom{
				// TODO probably don't want atoms in here anymore
				Ident: ProjectIdentifier{
					LocalName: pm.n,
				},
				Version: v,
			},
			Manifest: m,
			Lock:     l,
		}, nil
	}

	return ProjectInfo{}, err
}

func (pm *projectManager) ListVersions() (vlist []Version, err error) {
	if !pm.cvsync {
		pm.ex.s |= ExistsInCache | ExistsUpstream

		vpairs, exbits, err := pm.crepo.getCurrentVersionPairs()
		pm.ex.f |= exbits

		if err != nil {
			// TODO More-er proper-er error
			fmt.Println(err)
			return nil, err
		}

		vlist = make([]Version, len(vpairs))
		pm.cvsync = true
		// Process the version data into the cache
		// TODO detect out-of-sync data as we do this?
		for k, v := range vpairs {
			pm.dc.VMap[v] = v.Underlying()
			pm.dc.RMap[v.Underlying()] = append(pm.dc.RMap[v.Underlying()], v)
			vlist[k] = v
		}
	} else {
		vlist = make([]Version, len(pm.dc.VMap))
		k := 0
		// TODO key type of VMap should be string; recombine here
		//for v, r := range pm.dc.VMap {
		for v, _ := range pm.dc.VMap {
			vlist[k] = v
			k++
		}
	}

	return
}

// CheckExistence provides a direct method for querying existence levels of the
// project. It will only perform actual searching (local fs or over the network)
// if no previous attempt at that search has been made.
//
// Note that this may perform read-ish operations on the cache repo, and it
// takes a lock accordingly. Deadlock may result from calling it during a
// segment where the cache repo mutex is already write-locked.
func (pm *projectManager) CheckExistence(ex ProjectExistence) bool {
	if pm.ex.s&ex != ex {
		if ex&ExistsInVendorRoot != 0 && pm.ex.s&ExistsInVendorRoot == 0 {
			pm.ex.s |= ExistsInVendorRoot

			fi, err := os.Stat(path.Join(pm.vendordir, string(pm.n)))
			if err == nil && fi.IsDir() {
				pm.ex.f |= ExistsInVendorRoot
			}
		}
		if ex&ExistsInCache != 0 && pm.ex.s&ExistsInCache == 0 {
			pm.crepo.mut.RLock()
			pm.ex.s |= ExistsInCache
			if pm.crepo.r.CheckLocal() {
				pm.ex.f |= ExistsInCache
			}
			pm.crepo.mut.RUnlock()
		}
		if ex&ExistsUpstream != 0 && pm.ex.s&ExistsUpstream == 0 {
			pm.crepo.mut.RLock()
			pm.ex.s |= ExistsUpstream
			if pm.crepo.r.Ping() {
				pm.ex.f |= ExistsUpstream
			}
			pm.crepo.mut.RUnlock()
		}
	}

	return ex&pm.ex.f == ex
}

func (pm *projectManager) ExportVersionTo(v Version, to string) error {
	return pm.crepo.exportVersionTo(v, to)
}

func (r *repo) getCurrentVersionPairs() (vlist []PairedVersion, exbits ProjectExistence, err error) {
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
			// TODO remove this path? it really just complicates things, for
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
			exbits |= ExistsUpstream | ExistsInCache
			// Also, local is definitely now synced
			r.synced = true

			out, err = r.r.RunFromDir("git", "show-ref", "--dereference")
			if err != nil {
				return
			}

			all = bytes.Split(bytes.TrimSpace(out), []byte("\n"))
		}
		// Local cache may not actually exist here, but upstream definitely does
		exbits |= ExistsUpstream

		for _, pair := range all {
			var v PairedVersion
			if string(pair[46:51]) == "heads" {
				v = NewBranch(string(pair[52:])).Is(Revision(pair[:40])).(PairedVersion)
			} else if string(pair[46:50]) == "tags" {
				// TODO deal with dereferenced tags
				v = NewVersion(string(pair[51:])).Is(Revision(pair[:40])).(PairedVersion)
			} else {
				continue
			}
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
		exbits |= ExistsUpstream | ExistsInCache
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
		exbits |= ExistsUpstream | ExistsInCache
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
		// TODO is it ok to return empty vlist and no error?
		// TODO ...gotta do something for svn, right?
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
		idx, bak := path.Join(r.rpath, ".git", "index"), path.Join(r.rpath, ".git", "origindex")
		err := os.Rename(idx, bak)
		if err != nil {
			return err
		}

		// TODO could have an err here
		defer os.Rename(bak, idx)

		_, err = r.r.RunFromDir("git", "read-tree", v.String())
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
		if err != nil {
			return err
		}

		return filepath.Walk(to, stripVendor)
	default:
		// TODO This is a dumb, slow approach, but we're punting on making these
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
				return os.RemoveAll(path)
			}
		}
	}

	return nil
}
