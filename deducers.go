package gps

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	ctx        context.Context
	cancelFunc context.CancelFunc
	mu         sync.Mutex // Guards all maps.
	running    map[callInfo]timeCount
	//running map[callInfo]time.Time
	ran map[callType]durCount
	//ran map[callType]time.Duration
}

func newCallManager(ctx context.Context) *callManager {
	ctx, cf := context.WithCancel(ctx)
	return &callManager{
		ctx:        ctx,
		cancelFunc: cf,
		running:    make(map[callInfo]timeCount),
		ran:        make(map[callType]durCount),
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

func (cm *callManager) getLifetimeContext() context.Context {
	return cm.ctx
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
	ret chan *sourceGateway
	err chan error
}

func (rc srcReturnChans) awaitReturn() (sg *sourceGateway, err error) {
	select {
	case sg = <-rc.ret:
	case err = <-rc.err:
	}
	return
}

type sourceCoordinator struct {
	callMgr   *callManager
	srcmut    sync.RWMutex // guards srcs and nameToURL maps
	srcs      map[string]*sourceGateway
	nameToURL map[string]string
	psrcmut   sync.Mutex // guards protoSrcs map
	protoSrcs map[string][]srcReturnChans
	deducer   *deductionCoordinator
	cachedir  string
}

func newSourceCoordinator(cm *callManager, deducer *deductionCoordinator, cachedir string) *sourceCoordinator {
	return &sourceCoordinator{
		callMgr:   cm,
		deducer:   deducer,
		cachedir:  cachedir,
		srcs:      make(map[string]*sourceGateway),
		nameToURL: make(map[string]string),
		protoSrcs: make(map[string][]srcReturnChans),
	}
}

func (sc *sourceCoordinator) getSourceGatewayFor(ctx context.Context, id ProjectIdentifier) (*sourceGateway, error) {
	normalizedName := id.normalizedSource()

	sc.srcmut.RLock()
	if url, has := sc.nameToURL[normalizedName]; has {
		if srcGate, has := sc.srcs[url]; has {
			sc.srcmut.RUnlock()
			return srcGate, nil
		}
	}
	sc.srcmut.RUnlock()

	// No gateway exists for this path yet; set up a proto, being careful to fold
	// together simultaneous attempts on the same path.
	rc := srcReturnChans{
		ret: make(chan *sourceGateway),
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

		doReturn := func(sa *sourceGateway, err error) {
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

		// It'd be quite the feat - but not impossible - for a gateway
		// corresponding to this normalizedName to have slid into the main
		// sources map after the initial unlock, but before this goroutine got
		// scheduled. Guard against that by checking the main sources map again
		// and bailing out if we find an entry.
		var srcGate *sourceGateway
		sc.srcmut.RLock()
		if url, has := sc.nameToURL[normalizedName]; has {
			if srcGate, has := sc.srcs[url]; has {
				sc.srcmut.RUnlock()
				doReturn(srcGate, nil)
				return
			}
			// This should panic, right?
			panic("")
		}
		sc.srcmut.RUnlock()

		srcGate = &sourceGateway{
			maybe:    pd.mb,
			action:   make(chan func()),
			callMgr:  sc.callMgr,
			cachedir: sc.cachedir,
		}

		// The normalized name is usually different from the source URL- e.g.
		// github.com/sdboyer/gps vs. https://github.com/sdboyer/gps. But it's
		// possible to arrive here with a full URL as the normalized name - and
		// both paths *must* lead to the same sourceGateway instance in order to
		// ensure disk access is correctly managed.
		//
		// Therefore, we now must query the sourceGateway to get the actual
		// sourceURL it's operating on, and ensure it's *also* registered at
		// that path in the map. This will cause it to actually initiate the
		// maybeSource.try() behavior in order to settle on a URL.
		url, err := srcGate.sourceURL(ctx)
		if err != nil {
			doReturn(nil, err)
			return
		}

		// We know we have a working srcGateway at this point, and need to
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

		sc.srcs[url] = srcGate
		doReturn(srcGate, nil)
	}()

	return rc.awaitReturn()
}

// sourceGateways manage all incoming calls for data from sources, serializing
// and caching them as needed.
type sourceGateway struct {
	maybe    maybeSource
	cachedir string
	mu       sync.Mutex // global lock, serializes all behaviors
	action   chan (func())
	callMgr  *callManager
}

func (sg *sourceGateway) sourceURL(ctx context.Context) (string, error) {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	return "", nil
}
