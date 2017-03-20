package gps

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	radix "github.com/armon/go-radix"
	"github.com/sdboyer/constext"
)

type timeCount struct {
	count int
	start time.Time
}

type durCount struct {
	count int
	dur   time.Duration
}

type callManager struct {
	ctx     context.Context
	mu      sync.Mutex // Guards all maps.
	running map[callInfo]timeCount
	//running map[callInfo]time.Time
	ran map[callType]durCount
	//ran map[callType]time.Duration
}

func newCallManager(ctx context.Context) *callManager {
	return &callManager{
		ctx:     ctx,
		running: make(map[callInfo]timeCount),
		ran:     make(map[callType]durCount),
	}
}

// Helper function to register a call with a callManager, combine contexts, and
// create a to-be-deferred func to clean it all up.
func (cm *callManager) setUpCall(inctx context.Context, name string, typ callType) (cctx context.Context, doneFunc func(), err error) {
	ci := callInfo{
		name: name,
		typ:  typ,
	}

	octx, err := cm.run(ci)
	if err != nil {
		return nil, nil, err
	}

	cctx, cancelFunc := constext.Cons(inctx, octx)
	return cctx, func() {
		cm.done(ci)
		cancelFunc() // ensure constext cancel goroutine is cleaned up
	}, nil
}

func (cm *callManager) run(ci callInfo) (context.Context, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.ctx.Err() != nil {
		// We've already been canceled; error out.
		return nil, cm.ctx.Err()
	}

	if existingInfo, has := cm.running[ci]; has {
		existingInfo.count++
		cm.running[ci] = existingInfo
	} else {
		cm.running[ci] = timeCount{
			count: 1,
			start: time.Now(),
		}
	}

	return cm.ctx, nil
}

func (cm *callManager) done(ci callInfo) {
	cm.mu.Lock()

	existingInfo, has := cm.running[ci]
	if !has {
		panic(fmt.Sprintf("sourceMgr: tried to complete a call that had not registered via run()"))
	}

	if existingInfo.count > 1 {
		// If more than one is pending, don't stop the clock yet.
		existingInfo.count--
		cm.running[ci] = existingInfo
	} else {
		// Last one for this particular key; update metrics with info.
		durCnt := cm.ran[ci.typ]
		durCnt.count++
		durCnt.dur += time.Now().Sub(existingInfo.start)
		cm.ran[ci.typ] = durCnt
		delete(cm.running, ci)
	}

	cm.mu.Unlock()
}

type callType uint

const (
	ctHTTPMetadata callType = iota
	ctListVersions
	ctGetManifestAndLock
)

// callInfo provides metadata about an ongoing call.
type callInfo struct {
	name string
	typ  callType
}

type srcReturnChans struct {
	ret chan *sourceActor
	err chan error
}

func (retchans srcReturnChans) awaitReturn() (*sourceActor, error) {
	select {
	case sa := <-retchans.ret:
		return sa, nil
	case err := <-retchans.err:
		return nil, err
	}
}

type sourcesCompany struct {
	callMgr   *callManager
	srcmut    sync.RWMutex // guards srcs and nameToURL maps
	srcs      map[string]*sourceActor
	nameToURL map[string]string
	psrcmut   sync.Mutex // guards protoSrcs map
	protoSrcs map[string][]srcReturnChans
	deducer   *deductionCoordinator
	cachedir  string
}

func (sc *sourcesCompany) getSourceActorFor(ctx context.Context, id ProjectIdentifier) (*sourceActor, error) {
	normalizedName := id.normalizedSource()

	sc.srcmut.RLock()
	if url, has := sc.nameToURL[normalizedName]; has {
		if srcActor, has := sc.srcs[url]; has {
			sc.srcmut.RUnlock()
			return srcActor, nil
		}
	}
	sc.srcmut.RUnlock()

	// No actor exists for this path yet; set up a proto, being careful to fold
	// together simultaneous attempts on the same path.
	rc := srcReturnChans{
		ret: make(chan *sourceActor),
		err: make(chan error),
	}

	// The rest of the work needs its own goroutine, the results of which will
	// be re-joined to this call via the return chans.
	go func() {
		sc.psrcmut.Lock()
		if chans, has := sc.protoSrcs[normalizedName]; has {
			// Another goroutine is already working on this normalizedName. Fold
			// in with that work by attaching our return channels to the list.
			sc.protoSrcs[normalizedName] = append(chans, rc)
			sc.psrcmut.Unlock()
			return
		}

		sc.protoSrcs[normalizedName] = []srcReturnChans{rc}
		sc.psrcmut.Unlock()

		doReturn := func(sa *sourceActor, err error) {
			sc.psrcmut.Lock()
			if sa != nil {
				for _, rc := range sc.protoSrcs[normalizedName] {
					rc.ret <- sa
				}
			} else if err != nil {
				for _, rc := range sc.protoSrcs[normalizedName] {
					rc.err <- err
				}
			} else {
				panic("sa and err both nil")
			}

			delete(sc.protoSrcs, normalizedName)
			sc.psrcmut.Unlock()
		}

		pd, err := sc.deducer.deduceRootPath(normalizedName)
		if err != nil {
			// As in the deducer, don't cache errors so that externally-driven retry
			// strategies can be constructed.
			doReturn(nil, err)
			return
		}

		// It'd be quite the feat - but not impossible - for an actor
		// corresponding to this normalizedName to have slid into the main
		// sources map after the initial unlock, but before this goroutine got
		// scheduled. Guard against that by checking the main sources map again
		// and bailing out if we find an entry.
		sc.srcmut.RLock()
		srcActor, has := sc.srcs[normalizedName]
		sc.srcmut.RUnlock()
		if has {
			doReturn(srcActor, nil)
			return
		}

		srcActor = &sourceActor{
			maybe:    pd.mb,
			action:   make(chan func()),
			callMgr:  sc.callMgr,
			cachedir: sc.cachedir,
		}

		// The normalized name is usually different from the source URL- e.g.
		// github.com/sdboyer/gps vs. https://github.com/sdboyer/gps. But it's
		// possible to arrive here with a full URL as the normalized name - and
		// both paths *must* lead to the same sourceActor instance in order to
		// ensure disk access is correctly managed.
		//
		// Therefore, we now must query the sourceActor to get the actual
		// sourceURL it's operating on, and ensure it's *also* registered at
		// that path in the map. This will cause it to actually initiate the
		// maybeSource.try() behavior in order to settle on a URL.
		url, err := srcActor.sourceURL(ctx)
		if err != nil {
			doReturn(nil, err)
			return
		}

		// We know we have a working srcActor at this point, and need to
		// integrate it back into the main map.
		sc.srcmut.Lock()
		defer sc.srcmut.Unlock()
		// Record the name -> URL mapping, even if it's a self-mapping.
		sc.nameToURL[normalizedName] = url

		if sa, has := sc.srcs[url]; has {
			// URL already had an entry in the main map; use that as the result.
			doReturn(sa, nil)
			return
		}

		sc.srcs[url] = srcActor
		doReturn(srcActor, nil)
	}()

	return rc.awaitReturn()
}

// sourceActors act as a gateway to all calls for data from sources.
type sourceActor struct {
	maybe    maybeSource
	cachedir string
	mu       sync.Mutex // global lock, serializes all behaviors
	action   chan (func())
	callMgr  *callManager
}

func (sa *sourceActor) sourceURL(ctx context.Context) (string, error) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	return "", nil
}

type deductionCoordinator struct {
	ctx        context.Context
	callMgr    *callManager
	rootxt     *radix.Tree
	deducext   *deducerTrie
	actionChan chan func()
}

func newDeductionCoordinator(ctx context.Context, cm *callManager) *deductionCoordinator {
	dc := &deductionCoordinator{
		ctx:      ctx,
		callMgr:  cm,
		rootxt:   radix.New(),
		deducext: pathDeducerTrie(),
	}

	// Start listener loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(dc.actionChan)
			case action := <-dc.actionChan:
				action()
			}
		}
	}()

	return dc
}

func (dc *deductionCoordinator) deduceRootPath(path string) (pathDeduction, error) {
	if dc.ctx.Err() != nil {
		return pathDeduction{}, errors.New("deductionCoordinator has been terminated")
	}

	retchan, errchan := make(chan pathDeduction), make(chan error)
	dc.actionChan <- func() {
		hmdDeduce := func(hmd *httpMetadataDeducer) {
			pd, err := hmd.deduce(context.TODO(), path)
			if err != nil {
				errchan <- err
			} else {
				retchan <- pd
			}
		}

		// First, check the rootxt to see if there's a prefix match - if so, we
		// can return that and move on.
		if prefix, data, has := dc.rootxt.LongestPrefix(path); has && isPathPrefixOrEqual(prefix, path) {
			switch d := data.(type) {
			case maybeSource:
				retchan <- pathDeduction{root: prefix, mb: d}
			case *httpMetadataDeducer:
				// Multiple calls have come in for a similar path shape during
				// the window in which the HTTP request to retrieve go get
				// metadata is in flight. Fold this request in with the existing
				// one(s) by giving it its own goroutine that awaits a response
				// from the running httpMetadataDeducer.
				go hmdDeduce(d)
			default:
				panic(fmt.Sprintf("unexpected %T in deductionCoordinator.rootxt: %v", d, d))
			}

			// Finding either a finished maybeSource or an in-flight vanity
			// deduction means there's nothing more to do on this action.
			return
		}

		// No match. Try known path deduction first.
		pd, err := dc.deduceKnownPaths(path)
		if err == nil {
			// Deduction worked; store it in the rootxt, send on retchan and
			// terminate.
			// FIXME(sdboyer) deal with changing path vs. root. Probably needs
			// to be predeclared and reused in the hmd returnFunc
			dc.rootxt.Insert(pd.root, pd.mb)
			retchan <- pd
			return
		}

		if err != errNoKnownPathMatch {
			errchan <- err
			return
		}

		// The err indicates no known path matched. It's still possible that
		// retrieving go get metadata might do the trick.
		hmd := &httpMetadataDeducer{
			basePath: path,
			callMgr:  dc.callMgr,
			// The vanity deducer will call this func with a completed
			// pathDeduction if it succeeds in finding one. We process it
			// back through the action channel to ensure serialized
			// access to the rootxt map.
			returnFunc: func(pd pathDeduction) {
				dc.actionChan <- func() {
					if pd.root != path {
						// Replace the vanity deducer with a real result set, so
						// that subsequent deductions don't hit the network
						// again.
						dc.rootxt.Insert(path, pd.mb)
					}
					dc.rootxt.Insert(pd.root, pd.mb)
				}
			},
		}

		// Save the hmd in the rootxt so that calls checking on similar
		// paths made while the request is in flight can be folded together.
		dc.rootxt.Insert(path, hmd)
		// Spawn a new goroutine for the HTTP-backed deduction process.
		go hmdDeduce(hmd)

	}

	select {
	case pd := <-retchan:
		return pd, nil
	case err := <-errchan:
		return pathDeduction{}, err
	}
}

// pathDeduction represents the results of a successful import path deduction -
// a root path, plus a maybeSource that can be used to attempt to connect to
// the source.
type pathDeduction struct {
	root string
	mb   maybeSource
}

var errNoKnownPathMatch = errors.New("no known path match")

func (dc *deductionCoordinator) deduceKnownPaths(path string) (pathDeduction, error) {
	u, path, err := normalizeURI(path)
	if err != nil {
		return pathDeduction{}, err
	}

	// First, try the root path-based matches
	if _, mtch, has := dc.deducext.LongestPrefix(path); has {
		root, err := mtch.deduceRoot(path)
		if err != nil {
			return pathDeduction{}, err
		}
		mb, err := mtch.deduceSource(path, u)
		if err != nil {
			return pathDeduction{}, err
		}

		return pathDeduction{
			root: root,
			mb:   mb,
		}, nil
	}

	// Next, try the vcs extension-based (infix) matcher
	exm := vcsExtensionDeducer{regexp: vcsExtensionRegex}
	if root, err := exm.deduceRoot(path); err == nil {
		mb, err := exm.deduceSource(path, u)
		if err != nil {
			return pathDeduction{}, err
		}

		return pathDeduction{
			root: root,
			mb:   mb,
		}, nil
	}

	return pathDeduction{}, errNoKnownPathMatch
}

type httpMetadataDeducer struct {
	once       sync.Once
	deduced    pathDeduction
	deduceErr  error
	basePath   string
	returnFunc func(pathDeduction)
	callMgr    *callManager
}

func (hmd *httpMetadataDeducer) deduce(ctx context.Context, path string) (pathDeduction, error) {
	hmd.once.Do(func() {
		ctx, doneFunc, err := hmd.callMgr.setUpCall(ctx, path, ctHTTPMetadata)
		if err != nil {
			hmd.deduceErr = err
			return
		}
		defer doneFunc()

		opath := path
		// FIXME should we need this first return val?
		_, path, err := normalizeURI(path)
		if err != nil {
			hmd.deduceErr = err
			return
		}

		pd := pathDeduction{}

		// Make the HTTP call to attempt to retrieve go-get metadata
		root, vcs, reporoot, err := parseMetadata(ctx, path)
		if err != nil {
			hmd.deduceErr = fmt.Errorf("unable to deduce repository and source type for: %q", opath)
			return
		}
		pd.root = root

		// If we got something back at all, then it supercedes the actual input for
		// the real URL to hit
		repoURL, err := url.Parse(reporoot)
		if err != nil {
			hmd.deduceErr = fmt.Errorf("server returned bad URL when searching for vanity import: %q", reporoot)
			return
		}

		switch vcs {
		case "git":
			pd.mb = maybeGitSource{url: repoURL}
		case "bzr":
			pd.mb = maybeBzrSource{url: repoURL}
		case "hg":
			pd.mb = maybeHgSource{url: repoURL}
		default:
			hmd.deduceErr = fmt.Errorf("unsupported vcs type %s in go-get metadata from %s", vcs, path)
			return
		}

		hmd.deduced = pd
		// All data is assigned for other goroutines that may be waiting. Now,
		// send the pathDeduction back to the deductionCoordinator by calling
		// the returnFunc. This will also remove the reference to this hmd in
		// the coordinator's trie.
		//
		// When this call finishes, it is guaranteed the coordinator will have
		// at least begun running the action to insert the path deduction, which
		// means no other deduction request will be able to interleave and
		// request the same path before the pathDeduction can be processed, but
		// after this hmd has been dereferenced from the trie.
		hmd.returnFunc(pd)
	})

	return hmd.deduced, hmd.deduceErr
}
