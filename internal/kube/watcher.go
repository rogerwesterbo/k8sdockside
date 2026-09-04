package kube

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

// Snapshot is one push to the frontend: the whole current contents of a
// subscribed collection.
//
// Deltas would be cheaper on the wire, but a Row is a *projection* of an object
// and the frontend cannot apply a delta without redoing that projection. The
// informer keeps a local cache, so rebuilding the entire table costs no API
// traffic at all -- only the projection itself, which is why sending the whole
// thing is affordable.
type Snapshot struct {
	SubscriptionID string `json:"subscriptionId"`
	Table          Table  `json:"table"`
}

// coalesce is how long a subscription waits after a change before pushing. A
// rollout produces a burst of pod events; without this the UI would repaint for
// every one of them.
const coalesce = 150 * time.Millisecond

// resync is the informer's relist interval. Watches are meant to be reliable,
// so this is a backstop against a missed event rather than the primary path.
const resync = 10 * time.Minute

// Watcher owns every live connection: one client and one set of informers per
// kubeconfig context, shared by all the tabs looking at that context.
//
// Lifetime is reference-counted from both ends. Three tabs on the same kind
// share one watch; the informer stops when the last of them closes, and the
// cluster's client goes with the last informer.
type Watcher struct {
	emit func(Snapshot)

	mu       sync.Mutex
	clusters map[string]*cluster
	subs     map[string]*subscription
	nextID   atomic.Uint64
}

// NewWatcher returns a watcher that pushes snapshots through emit. emit is
// called from background goroutines and must be safe for concurrent use.
func NewWatcher(emit func(Snapshot)) *Watcher {
	return &Watcher{
		emit:     emit,
		clusters: map[string]*cluster{},
		subs:     map[string]*subscription{},
	}
}

// cluster is one context's live connection plus the informers opened against it.
type cluster struct {
	refs      int
	informers map[schema.GroupVersionResource]*liveInformer

	// ready closes once client/err are set. Building a client can run an exec
	// credential plugin, which is slow, so it happens off the watcher's lock
	// with other callers waiting here rather than on the mutex.
	ready  chan struct{}
	client *clusterClient
	err    error
}

// liveInformer is one watch: shared by every subscription to the same resource
// in the same cluster, regardless of the namespace each of them is filtering to.
type liveInformer struct {
	refs     int
	gvr      schema.GroupVersionResource
	informer cache.SharedIndexInformer
	lister   cache.GenericLister
	scope    meta.RESTScopeName
	stop     chan struct{}
}

// subscription is one open tab. It holds the namespace filter, because the
// informer it reads from is cluster-scoped and shared.
type subscription struct {
	id        string
	contextID string
	kind      string
	namespace string
	columns   []column
	live      *liveInformer

	dirty chan struct{} // buffered(1): a pending "something changed"
	done  chan struct{}
}

// Subscribe opens a live view of one kind and returns its subscription ID. It
// returns as soon as the watch is started; the first snapshot arrives through
// emit once the informer's cache has synced, so the caller never blocks on the
// network.
func (w *Watcher) Subscribe(kc Context, kind, namespace string) (string, error) {
	cl, err := w.clusterFor(kc)
	if err != nil {
		return "", err
	}

	mapping, err := cl.client.mappingForKind(kind)
	if err != nil {
		w.releaseCluster(kc.ID)
		return "", err
	}

	namespaced := mapping.Scope.Name() == meta.RESTScopeNameNamespace
	// Columns are resolved per subscription, not per informer: two kinds can
	// map to the same resource (a Gateway is both "gateways" and a custom
	// resource) and they do not have to be rendered the same way.
	cols, err := cl.client.columnsFor(kind, namespaced)
	if err != nil {
		w.releaseCluster(kc.ID)
		return "", err
	}

	live := w.informerFor(cl, mapping)

	sub := &subscription{
		id:        fmt.Sprintf("sub-%d", w.nextID.Add(1)),
		contextID: kc.ID,
		kind:      kind,
		namespace: namespace,
		columns:   cols,
		live:      live,
		dirty:     make(chan struct{}, 1),
		done:      make(chan struct{}),
	}

	w.mu.Lock()
	w.subs[sub.id] = sub
	w.mu.Unlock()

	go w.pump(sub)
	go w.firstSnapshot(sub)

	return sub.id, nil
}

// Unsubscribe closes one tab's view, stopping the underlying watch if no other
// tab is using it.
func (w *Watcher) Unsubscribe(id string) {
	w.mu.Lock()
	sub, ok := w.subs[id]
	if !ok {
		w.mu.Unlock()
		return
	}
	delete(w.subs, id)

	live := sub.live
	live.refs--
	stopInformer := live.refs == 0
	if stopInformer {
		if cl, ok := w.clusters[sub.contextID]; ok {
			delete(cl.informers, live.gvr)
		}
	}
	w.mu.Unlock()

	close(sub.done)
	if stopInformer {
		close(live.stop)
		w.releaseCluster(sub.contextID)
	}
}

// SetNamespace re-points an existing subscription at another namespace. The
// informer is cluster-scoped, so this is a filter change and a repaint -- no
// watch is torn down or reopened.
func (w *Watcher) SetNamespace(id, namespace string) {
	w.mu.Lock()
	sub, ok := w.subs[id]
	if ok {
		sub.namespace = namespace
	}
	w.mu.Unlock()
	if ok {
		sub.markDirty()
	}
}

// Close tears down every subscription and connection. Called when the app quits.
func (w *Watcher) Close() {
	w.mu.Lock()
	ids := make([]string, 0, len(w.subs))
	for id := range w.subs {
		ids = append(ids, id)
	}
	w.mu.Unlock()

	for _, id := range ids {
		w.Unsubscribe(id)
	}
}

// clusterFor returns the live client for a context, building it if this is the
// first subscription against it, and takes a reference on it.
func (w *Watcher) clusterFor(kc Context) (*cluster, error) {
	w.mu.Lock()
	cl, ok := w.clusters[kc.ID]
	if !ok {
		cl = &cluster{informers: map[schema.GroupVersionResource]*liveInformer{}, ready: make(chan struct{})}
		w.clusters[kc.ID] = cl
	}
	cl.refs++
	w.mu.Unlock()

	if !ok {
		// First caller builds the client, off the lock: an exec credential
		// plugin can take seconds and must not stall other contexts.
		cl.client, cl.err = newClusterClient(kc)
		close(cl.ready)
	}
	<-cl.ready

	if cl.err != nil {
		w.releaseCluster(kc.ID)
		return nil, cl.err
	}
	return cl, nil
}

// releaseCluster drops one reference, forgetting the cluster at zero.
func (w *Watcher) releaseCluster(contextID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cl, ok := w.clusters[contextID]
	if !ok {
		return
	}
	cl.refs--
	if cl.refs <= 0 {
		delete(w.clusters, contextID)
	}
}

// informerFor returns the shared informer for a resource, starting it if this
// is the first subscription, and takes a reference on it.
func (w *Watcher) informerFor(cl *cluster, mapping *meta.RESTMapping) *liveInformer {
	w.mu.Lock()
	defer w.mu.Unlock()

	if live, ok := cl.informers[mapping.Resource]; ok {
		live.refs++
		return live
	}

	// Cluster-scoped, with namespace filtering done against the cache. That
	// matches how the tables have always filtered, and it means changing the
	// namespace dropdown is instant instead of reopening a watch.
	gi := dynamicinformer.NewFilteredDynamicInformer(
		cl.client.dynamic, mapping.Resource, "", resync, cache.Indexers{}, nil,
	)
	inf := gi.Informer()

	// An informer caches the whole collection. Without this, opening a Secrets
	// tab would pull every secret value in the cluster into the app's memory;
	// managed fields are simply bulk we never render.
	_ = inf.SetTransform(stripBulk)

	live := &liveInformer{
		refs:     1,
		gvr:      mapping.Resource,
		informer: inf,
		lister:   gi.Lister(),
		scope:    mapping.Scope.Name(),
		stop:     make(chan struct{}),
	}
	cl.informers[mapping.Resource] = live

	handler := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { w.markKind(live) },
		UpdateFunc: func(any, any) { w.markKind(live) },
		DeleteFunc: func(any) { w.markKind(live) },
	}
	_, _ = inf.AddEventHandler(handler)

	// A watch that cannot be established -- unreachable cluster, expired
	// credentials, no RBAC for this kind -- is the failure the user most needs
	// to see, and it arrives here rather than from Subscribe.
	_ = inf.SetWatchErrorHandler(func(_ *cache.Reflector, err error) {
		w.reportError(live, err)
	})

	go inf.Run(live.stop)
	return live
}

// markKind flags every subscription reading from an informer as needing a push.
func (w *Watcher) markKind(live *liveInformer) {
	w.mu.Lock()
	subs := w.subsFor(live)
	w.mu.Unlock()
	for _, s := range subs {
		s.markDirty()
	}
}

// reportError pushes a failure to every tab watching the affected kind.
func (w *Watcher) reportError(live *liveInformer, err error) {
	w.mu.Lock()
	subs := w.subsFor(live)
	w.mu.Unlock()
	for _, s := range subs {
		w.emit(Snapshot{
			SubscriptionID: s.id,
			Table:          Table{Kind: s.kind, Columns: []string{}, Rows: []Row{}, Error: err.Error()},
		})
	}
}

// subsFor lists the subscriptions reading from an informer. The caller holds
// the lock.
func (w *Watcher) subsFor(live *liveInformer) []*subscription {
	var out []*subscription
	for _, s := range w.subs {
		if s.live == live {
			out = append(out, s)
		}
	}
	return out
}

// firstSnapshot pushes once the cache has synced, so that a collection which is
// simply empty still resolves the tab's loading state.
func (w *Watcher) firstSnapshot(sub *subscription) {
	if cache.WaitForCacheSync(sub.done, sub.live.informer.HasSynced) {
		sub.markDirty()
	}
}

// pump is one subscription's push loop: it waits for a change, lets the burst
// settle, then sends the current contents.
func (w *Watcher) pump(sub *subscription) {
	for {
		select {
		case <-sub.done:
			return
		case <-sub.dirty:
		}

		select {
		case <-sub.done:
			return
		case <-time.After(coalesce):
		}

		w.mu.Lock()
		namespace := sub.namespace
		w.mu.Unlock()

		w.emit(Snapshot{SubscriptionID: sub.id, Table: w.project(sub, namespace)})
	}
}

// project turns the informer's cache into the table the UI renders.
func (w *Watcher) project(sub *subscription, namespace string) Table {
	objs, err := sub.live.lister.List(labels.Everything())
	if err != nil {
		return Table{Kind: sub.kind, Columns: []string{}, Rows: []Row{}, Error: err.Error()}
	}

	items := make([]*unstructured.Unstructured, 0, len(objs))
	for _, o := range objs {
		u, ok := o.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		if namespace != AllNamespaces && u.GetNamespace() != namespace {
			continue
		}
		items = append(items, u)
	}

	return buildLiveTable(sub.kind, sub.columns, sub.live.scope == meta.RESTScopeNameNamespace, items)
}

// markDirty records that this subscription has something new to send. The
// channel is buffered to one, so a burst collapses into a single wake-up.
func (s *subscription) markDirty() {
	select {
	case s.dirty <- struct{}{}:
	default:
	}
}

// stripBulk drops the parts of an object we never render before it is cached.
func stripBulk(obj any) (any, error) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return obj, nil
	}
	u.SetManagedFields(nil)
	// Secret values must not sit in a desktop app's memory just because a tab
	// listing their names happens to be open. The count of keys is all the
	// table shows, so keep the keys and drop what they point at.
	if u.GetKind() == "Secret" {
		if data, found, _ := unstructured.NestedMap(u.Object, "data"); found {
			redacted := make(map[string]any, len(data))
			for k := range data {
				redacted[k] = ""
			}
			_ = unstructured.SetNestedMap(u.Object, redacted, "data")
		}
	}
	return u, nil
}

var _ runtime.Object = (*unstructured.Unstructured)(nil)
