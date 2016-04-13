package vsolver

import (
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Masterminds/semver"
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
	GetInfo(build.Context, ProjectName) (ProjectInfo, error)
}

type projectManager struct {
	n ProjectName
	// build.Context to  use in any analysis, and to pass to the analyzer
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
	// The list of versions. Kept separate from the data cache because this is
	// accessed in the hot loop; we don't want to rebuild and realloc for it.
	vlist []Version
	// Direction to sort the version list in (true is for upgrade, false for
	// downgrade)
	sortup bool
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

	if pi, exists := pm.dc.Infos[v.Underlying]; exists {
		return pi, nil
	}

	pm.crepo.mut.Lock()
	err := pm.crepo.r.UpdateVersion(v.Info)
	pm.crepo.mut.Unlock()
	if err != nil {
		// TODO More-er proper-er error
		fmt.Println(err)
		panic("canary - why is checkout/whatever failing")
	}

	pm.crepo.mut.RLock()
	i, err := pm.an.GetInfo(pm.ctx, pm.n)
	pm.crepo.mut.RUnlock()

	return i, err
}

func (pm *projectManager) ListVersions() (vlist []Version, err error) {
	if !pm.cvsync {
		pm.ex.s |= ExistsInCache | ExistsUpstream
		pm.vlist, err = pm.crepo.getCurrentVersionPairs()
		if err != nil {
			// TODO More-er proper-er error
			fmt.Println(err)
			return nil, err
		}

		pm.ex.f |= ExistsInCache | ExistsUpstream
		pm.cvsync = true

		// Process the version data into the cache
		// TODO detect out-of-sync data as we do this?
		for _, v := range pm.vlist {
			pm.dc.VMap[v] = v.Underlying
			pm.dc.RMap[v.Underlying] = append(pm.dc.RMap[v.Underlying], v)
		}

		// Sort the versions
		// TODO do this as a heap in the original call
		if pm.sortup {
			sort.Sort(upgradeVersionSorter(pm.vlist))
		} else {
			sort.Sort(downgradeVersionSorter(pm.vlist))
		}
	}

	return pm.vlist, nil
}

// CheckExistence provides a direct method for querying existence levels of the
// project. It will only perform actual searching (local fs or over the network)
// if no previous attempt at that search has been made.
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
			pm.ex.s |= ExistsInCache
			if pm.crepo.r.CheckLocal() {
				pm.ex.f |= ExistsInCache
			}
		}
		if ex&ExistsUpstream != 0 && pm.ex.s&ExistsUpstream == 0 {
			//pm.ex.s |= ExistsUpstream
			// TODO maybe need a method to do this as cheaply as possible,
			// per-repo type
		}
	}

	return ex&pm.ex.f == ex
}

func (pm *projectManager) ExportVersionTo(v Version, to string) error {
	return pm.crepo.exportVersionTo(v, to)
}

func (r *repo) getCurrentVersionPairs() (vlist []Version, err error) {
	r.mut.Lock()
	defer r.mut.Unlock()

	vis, s, err := r.r.CurrentVersionsWithRevs()
	// Even if an error occurs, it could have synced
	if s {
		r.synced = true
	}

	if err != nil {
		return nil, err
	}

	for _, vi := range vis {
		v := Version{
			Type:       V_Version,
			Info:       vi.Name,
			Underlying: Revision(vi.Revision),
		}

		if vi.IsBranch {
			v.Type = V_Branch
		} else {
			sv, err := semver.NewVersion(vi.Name)
			if err == nil {
				v.SemVer = sv
				v.Type = V_Semver
			}
		}

		vlist = append(vlist, v)
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

		_, err = r.runFromDir("git", "read-tree", v.Info)
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
		_, err = r.runFromDir("git", "checkout-index", "-a", "--prefix="+to)
		if err != nil {
			return err
		}

		return filepath.Walk(to, stripVendor)
	default:
		// TODO This is a dumb, slow approach, but we're punting on making these
		// fast for now because git is the OVERWHELMING case
		r.r.UpdateVersion(v.Info)

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

// These three funcs copied from Masterminds/vcs so we can exec our own commands
func (r *repo) runFromDir(cmd string, args ...string) ([]byte, error) {
	c := exec.Command(cmd, args...)
	c.Dir, c.Env = r.rpath, envForDir(r.rpath)

	return c.CombinedOutput()
}

func envForDir(dir string) []string {
	return mergeEnvLists([]string{"PWD=" + dir}, os.Environ())
}

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
