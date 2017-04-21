package gps

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sdboyer/constext"
	"github.com/sdboyer/gps/pkgtree"
)

// Used to compute a friendly filepath from a URL-shaped input.
var sanitizer = strings.NewReplacer("-", "--", ":", "-", "/", "-", "+", "-")

// A SourceManager is responsible for retrieving, managing, and interrogating
// source repositories. Its primary purpose is to serve the needs of a Solver,
// but it is handy for other purposes, as well.
//
// gps's built-in SourceManager, SourceMgr, is intended to be generic and
// sufficient for any purpose. It provides some additional semantics around the
// methods defined here.
type SourceManager interface {
	// SourceExists checks if a repository exists, either upstream or in the
	// SourceManager's central repository cache.
	SourceExists(ProjectIdentifier) (bool, error)

	// SyncSourceFor will attempt to bring all local information about a source
	// fully up to date.
	SyncSourceFor(ProjectIdentifier) error

	// ListVersions retrieves a list of the available versions for a given
	// repository name.
	// TODO convert to []PairedVersion
	ListVersions(ProjectIdentifier) ([]PairedVersion, error)

	// RevisionPresentIn indicates whether the provided Version is present in
	// the given repository.
	RevisionPresentIn(ProjectIdentifier, Revision) (bool, error)

	// ListPackages parses the tree of the Go packages at or below root of the
	// provided ProjectIdentifier, at the provided version.
	ListPackages(ProjectIdentifier, Version) (pkgtree.PackageTree, error)

	// GetManifestAndLock returns manifest and lock information for the provided
	// root import path.
	//
	// gps currently requires that projects be rooted at their repository root,
	// necessitating that the ProjectIdentifier's ProjectRoot must also be a
	// repository root.
	GetManifestAndLock(ProjectIdentifier, Version, ProjectAnalyzer) (Manifest, Lock, error)

	// ExportProject writes out the tree of the provided import path, at the
	// provided version, to the provided directory.
	ExportProject(ProjectIdentifier, Version, string) error

	// DeduceRootProject takes an import path and deduces the corresponding
	// project/source root.
	DeduceProjectRoot(ip string) (ProjectRoot, error)

	// Release lets go of any locks held by the SourceManager. Once called, it is
	// no longer safe to call methods against it; all method calls will
	// immediately result in errors.
	Release()
}

// A ProjectAnalyzer is responsible for analyzing a given path for Manifest and
// Lock information. Tools relying on gps must implement one.
type ProjectAnalyzer interface {
	// Perform analysis of the filesystem tree rooted at path, with the
	// root import path importRoot, to determine the project's constraints, as
	// indicated by a Manifest and Lock.
	DeriveManifestAndLock(path string, importRoot ProjectRoot) (Manifest, Lock, error)

	// Report the name and version of this ProjectAnalyzer.
	Info() (name string, version int)
}

// SourceMgr is the default SourceManager for gps.
//
// There's no (planned) reason why it would need to be reimplemented by other
// tools; control via dependency injection is intended to be sufficient.
type SourceMgr struct {
	cachedir    string                // path to root of cache dir
	lf          *os.File              // handle for the sm lock file on disk
	suprvsr     *supervisor           // subsystem that supervises running calls/io
	cancelAll   context.CancelFunc    // cancel func to kill all running work
	deduceCoord *deductionCoordinator // subsystem that manages import path deduction
	srcCoord    *sourceCoordinator    // subsystem that manages sources
	sigmut      sync.Mutex            // mutex protecting signal handling setup/teardown
	qch         chan struct{}         // quit chan for signal handler
	relonce     sync.Once             // once-er to ensure we only release once
	releasing   int32                 // flag indicating release of sm has begun
}

type smIsReleased struct{}

func (smIsReleased) Error() string {
	return "this SourceMgr has been released, its methods can no longer be called"
}

var _ SourceManager = &SourceMgr{}

// NewSourceManager produces an instance of gps's built-in SourceManager. It
// takes a cache directory, where local instances of upstream sources are
// stored.
//
// The returned SourceManager aggressively caches information wherever possible.
// If tools need to do preliminary work involving upstream repository analysis
// prior to invoking a solve run, it is recommended that they create this
// SourceManager as early as possible and use it to their ends. That way, the
// solver can benefit from any caches that may have already been warmed.
//
// gps's SourceManager is intended to be threadsafe (if it's not, please file a
// bug!). It should be safe to reuse across concurrent solving runs, even on
// unrelated projects.
func NewSourceManager(cachedir string) (*SourceMgr, error) {
	err := os.MkdirAll(filepath.Join(cachedir, "sources"), 0777)
	if err != nil {
		return nil, err
	}

	glpath := filepath.Join(cachedir, "sm.lock")
	_, err = os.Stat(glpath)
	if err == nil {
		return nil, CouldNotCreateLockError{
			Path: glpath,
			Err:  fmt.Errorf("cache lock file %s exists - another process crashed or is still running?", glpath),
		}
	}

	fi, err := os.OpenFile(glpath, os.O_CREATE|os.O_EXCL, 0600) // is 0600 sane for this purpose?
	if err != nil {
		return nil, CouldNotCreateLockError{
			Path: glpath,
			Err:  fmt.Errorf("err on attempting to create global cache lock: %s", err),
		}
	}

	ctx, cf := context.WithCancel(context.TODO())
	superv := newSupervisor(ctx)
	deducer := newDeductionCoordinator(superv)

	sm := &SourceMgr{
		cachedir:    cachedir,
		lf:          fi,
		suprvsr:     superv,
		cancelAll:   cf,
		deduceCoord: deducer,
		srcCoord:    newSourceCoordinator(superv, deducer, cachedir),
		qch:         make(chan struct{}),
	}

	return sm, nil
}

// UseDefaultSignalHandling sets up typical os.Interrupt signal handling for a
// SourceMgr.
func (sm *SourceMgr) UseDefaultSignalHandling() {
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt)
	sm.HandleSignals(sigch)
}

// HandleSignals sets up logic to handle incoming signals with the goal of
// shutting down the SourceMgr safely.
//
// Calling code must provide the signal channel, and is responsible for calling
// signal.Notify() on that channel.
//
// Successive calls to HandleSignals() will deregister the previous handler and
// set up a new one. It is not recommended that the same channel be passed
// multiple times to this method.
//
// SetUpSigHandling() will set up a handler that is appropriate for most
// use cases.
func (sm *SourceMgr) HandleSignals(sigch chan os.Signal) {
	sm.sigmut.Lock()
	// always start by closing the qch, which will lead to any existing signal
	// handler terminating, and deregistering its sigch.
	if sm.qch != nil {
		close(sm.qch)
	}
	sm.qch = make(chan struct{})

	// Run a new goroutine with the input sigch and the fresh qch
	go func(sch chan os.Signal, qch <-chan struct{}) {
		defer signal.Stop(sch)
		for {
			select {
			case <-sch:
				// Set up a timer to uninstall the signal handler after three
				// seconds, so that the user can easily force termination with a
				// second ctrl-c
				go func(c <-chan time.Time) {
					<-c
					signal.Stop(sch)
				}(time.After(3 * time.Second))

				if !atomic.CompareAndSwapInt32(&sm.releasing, 0, 1) {
					// Something's already called Release() on this sm, so we
					// don't have to do anything, as we'd just be redoing
					// that work. Instead, deregister and return.
					return
				}

				opc := sm.suprvsr.count()
				if opc > 0 {
					fmt.Printf("Signal received: waiting for %v ops to complete...\n", opc)
				}

				// Mutex interaction in a signal handler is, as a general rule,
				// unsafe. I'm not clear on whether the guarantees Go provides
				// around signal handling, or having passed this through a
				// channel in general, obviate those concerns, but it's a lot
				// easier to just rely on the mutex contained in the Once right
				// now, so do that until it proves problematic or someone
				// provides a clear explanation.
				sm.relonce.Do(func() { sm.doRelease() })
				return
			case <-qch:
				// quit channel triggered - deregister our sigch and return
				return
			}
		}
	}(sigch, sm.qch)
	// Try to ensure handler is blocked in for-select before releasing the mutex
	runtime.Gosched()

	sm.sigmut.Unlock()
}

// StopSignalHandling deregisters any signal handler running on this SourceMgr.
//
// It's normally not necessary to call this directly; it will be called as
// needed by Release().
func (sm *SourceMgr) StopSignalHandling() {
	sm.sigmut.Lock()
	if sm.qch != nil {
		close(sm.qch)
		sm.qch = nil
		runtime.Gosched()
	}
	sm.sigmut.Unlock()
}

// CouldNotCreateLockError describe failure modes in which creating a SourceMgr
// did not succeed because there was an error while attempting to create the
// on-disk lock file.
type CouldNotCreateLockError struct {
	Path string
	Err  error
}

func (e CouldNotCreateLockError) Error() string {
	return e.Err.Error()
}

// Release lets go of any locks held by the SourceManager. Once called, it is no
// longer safe to call methods against it; all method calls will immediately
// result in errors.
func (sm *SourceMgr) Release() {
	// Set sm.releasing before entering the Once func to guarantee that no
	// _more_ method calls will stack up if/while waiting.
	atomic.CompareAndSwapInt32(&sm.releasing, 0, 1)

	// Whether 'releasing' is set or not, we don't want this function to return
	// until after the doRelease process is done, as doing so could cause the
	// process to terminate before a signal-driven doRelease() call has a chance
	// to finish its cleanup.
	sm.relonce.Do(func() { sm.doRelease() })
}

// doRelease actually releases physical resources (files on disk, etc.).
//
// This must be called only and exactly once. Calls to it should be wrapped in
// the sm.relonce sync.Once instance.
func (sm *SourceMgr) doRelease() {
	// Send the signal to the supervisor to cancel all running calls
	sm.cancelAll()
	sm.suprvsr.wait()

	// Close the file handle for the lock file and remove it from disk
	sm.lf.Close()
	os.Remove(filepath.Join(sm.cachedir, "sm.lock"))

	// Close the qch, if non-nil, so the signal handlers run out. This will
	// also deregister the sig channel, if any has been set up.
	if sm.qch != nil {
		close(sm.qch)
	}
}

// GetManifestAndLock returns manifest and lock information for the provided
// ProjectIdentifier, at the provided Version. The work of producing the
// manifest and lock is delegated to the provided ProjectAnalyzer's
// DeriveManifestAndLock() method.
func (sm *SourceMgr) GetManifestAndLock(id ProjectIdentifier, v Version, an ProjectAnalyzer) (Manifest, Lock, error) {
	if atomic.CompareAndSwapInt32(&sm.releasing, 1, 1) {
		return nil, nil, smIsReleased{}
	}

	srcg, err := sm.srcCoord.getSourceGatewayFor(context.TODO(), id)
	if err != nil {
		return nil, nil, err
	}

	return srcg.getManifestAndLock(context.TODO(), id.ProjectRoot, v, an)
}

// ListPackages parses the tree of the Go packages at and below the ProjectRoot
// of the given ProjectIdentifier, at the given version.
func (sm *SourceMgr) ListPackages(id ProjectIdentifier, v Version) (pkgtree.PackageTree, error) {
	if atomic.CompareAndSwapInt32(&sm.releasing, 1, 1) {
		return pkgtree.PackageTree{}, smIsReleased{}
	}

	srcg, err := sm.srcCoord.getSourceGatewayFor(context.TODO(), id)
	if err != nil {
		return pkgtree.PackageTree{}, err
	}

	return srcg.listPackages(context.TODO(), id.ProjectRoot, v)
}

// ListVersions retrieves a list of the available versions for a given
// repository name.
//
// The list is not sorted; while it may be returned in the order that the
// underlying VCS reports version information, no guarantee is made. It is
// expected that the caller either not care about order, or sort the result
// themselves.
//
// This list is always retrieved from upstream on the first call. Subsequent
// calls will return a cached version of the first call's results. if upstream
// is not accessible (network outage, access issues, or the resource actually
// went away), an error will be returned.
func (sm *SourceMgr) ListVersions(id ProjectIdentifier) ([]PairedVersion, error) {
	if atomic.CompareAndSwapInt32(&sm.releasing, 1, 1) {
		return nil, smIsReleased{}
	}

	srcg, err := sm.srcCoord.getSourceGatewayFor(context.TODO(), id)
	if err != nil {
		// TODO(sdboyer) More-er proper-er errors
		return nil, err
	}

	return srcg.listVersions(context.TODO())
}

// RevisionPresentIn indicates whether the provided Revision is present in the given
// repository.
func (sm *SourceMgr) RevisionPresentIn(id ProjectIdentifier, r Revision) (bool, error) {
	if atomic.CompareAndSwapInt32(&sm.releasing, 1, 1) {
		return false, smIsReleased{}
	}

	srcg, err := sm.srcCoord.getSourceGatewayFor(context.TODO(), id)
	if err != nil {
		// TODO(sdboyer) More-er proper-er errors
		return false, err
	}

	return srcg.revisionPresentIn(context.TODO(), r)
}

// SourceExists checks if a repository exists, either upstream or in the cache,
// for the provided ProjectIdentifier.
func (sm *SourceMgr) SourceExists(id ProjectIdentifier) (bool, error) {
	if atomic.CompareAndSwapInt32(&sm.releasing, 1, 1) {
		return false, smIsReleased{}
	}

	srcg, err := sm.srcCoord.getSourceGatewayFor(context.TODO(), id)
	if err != nil {
		return false, err
	}

	ctx := context.TODO()
	return srcg.existsInCache(ctx) || srcg.existsUpstream(ctx), nil
}

// SyncSourceFor will ensure that all local caches and information about a
// source are up to date with any network-acccesible information.
//
// The primary use case for this is prefetching.
func (sm *SourceMgr) SyncSourceFor(id ProjectIdentifier) error {
	if atomic.CompareAndSwapInt32(&sm.releasing, 1, 1) {
		return smIsReleased{}
	}

	srcg, err := sm.srcCoord.getSourceGatewayFor(context.TODO(), id)
	if err != nil {
		return err
	}

	return srcg.syncLocal(context.TODO())
}

// ExportProject writes out the tree of the provided ProjectIdentifier's
// ProjectRoot, at the provided version, to the provided directory.
func (sm *SourceMgr) ExportProject(id ProjectIdentifier, v Version, to string) error {
	if atomic.CompareAndSwapInt32(&sm.releasing, 1, 1) {
		return smIsReleased{}
	}

	srcg, err := sm.srcCoord.getSourceGatewayFor(context.TODO(), id)
	if err != nil {
		return err
	}

	return srcg.exportVersionTo(context.TODO(), v, to)
}

// DeduceProjectRoot takes an import path and deduces the corresponding
// project/source root.
//
// Note that some import paths may require network activity to correctly
// determine the root of the path, such as, but not limited to, vanity import
// paths. (A special exception is written for gopkg.in to minimize network
// activity, as its behavior is well-structured)
func (sm *SourceMgr) DeduceProjectRoot(ip string) (ProjectRoot, error) {
	if atomic.CompareAndSwapInt32(&sm.releasing, 1, 1) {
		return "", smIsReleased{}
	}

	pd, err := sm.deduceCoord.deduceRootPath(context.TODO(), ip)
	return ProjectRoot(pd.root), err
}

type timeCount struct {
	count int
	start time.Time
}

type durCount struct {
	count int
	dur   time.Duration
}

type supervisor struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
	mu         sync.Mutex // Guards all maps
	cond       sync.Cond  // Wraps mu so callers can wait until all calls end
	running    map[callInfo]timeCount
	ran        map[callType]durCount
}

func newSupervisor(ctx context.Context) *supervisor {
	ctx, cf := context.WithCancel(ctx)
	supv := &supervisor{
		ctx:        ctx,
		cancelFunc: cf,
		running:    make(map[callInfo]timeCount),
		ran:        make(map[callType]durCount),
	}

	supv.cond = sync.Cond{L: &supv.mu}
	return supv
}

// do executes the incoming closure using a conjoined context, and keeps
// counters to ensure the sourceMgr can't finish Release()ing until after all
// calls have returned.
func (sup *supervisor) do(inctx context.Context, name string, typ callType, f func(context.Context) error) error {
	ci := callInfo{
		name: name,
		typ:  typ,
	}

	octx, err := sup.start(ci)
	if err != nil {
		return err
	}

	cctx, cancelFunc := constext.Cons(inctx, octx)
	err = f(cctx)
	sup.done(ci)
	cancelFunc()
	return err
}

func (sup *supervisor) getLifetimeContext() context.Context {
	return sup.ctx
}

func (sup *supervisor) start(ci callInfo) (context.Context, error) {
	sup.mu.Lock()
	defer sup.mu.Unlock()
	if sup.ctx.Err() != nil {
		// We've already been canceled; error out.
		return nil, sup.ctx.Err()
	}

	if existingInfo, has := sup.running[ci]; has {
		existingInfo.count++
		sup.running[ci] = existingInfo
	} else {
		sup.running[ci] = timeCount{
			count: 1,
			start: time.Now(),
		}
	}

	return sup.ctx, nil
}

func (sup *supervisor) count() int {
	sup.mu.Lock()
	defer sup.mu.Unlock()
	return len(sup.running)
}

func (sup *supervisor) done(ci callInfo) {
	sup.mu.Lock()

	existingInfo, has := sup.running[ci]
	if !has {
		panic(fmt.Sprintf("sourceMgr: tried to complete a call that had not registered via run()"))
	}

	if existingInfo.count > 1 {
		// If more than one is pending, don't stop the clock yet.
		existingInfo.count--
		sup.running[ci] = existingInfo
	} else {
		// Last one for this particular key; update metrics with info.
		durCnt := sup.ran[ci.typ]
		durCnt.count++
		durCnt.dur += time.Now().Sub(existingInfo.start)
		sup.ran[ci.typ] = durCnt
		delete(sup.running, ci)

		if len(sup.running) == 0 {
			// This is the only place where we signal the cond, as it's the only
			// time that the number of running calls could become zero.
			sup.cond.Signal()
		}
	}
	sup.mu.Unlock()
}

// wait until all active calls have terminated.
//
// Assumes something else has already canceled the supervisor via its context.
func (sup *supervisor) wait() {
	sup.cond.L.Lock()
	for len(sup.running) > 0 {
		sup.cond.Wait()
	}
	sup.cond.L.Unlock()
}

type callType uint

const (
	ctHTTPMetadata callType = iota
	ctListVersions
	ctGetManifestAndLock
	ctListPackages
	ctSourcePing
	ctSourceInit
	ctSourceFetch
	ctCheckoutVersion
	ctExportTree
)

// callInfo provides metadata about an ongoing call.
type callInfo struct {
	name string
	typ  callType
}
