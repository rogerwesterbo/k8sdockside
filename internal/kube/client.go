package kube

import (
	"fmt"
	"net"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	// Registers the exec credential plugin, which is how Talos, EKS, GKE and
	// anything else using `exec:` credentials authenticate. Importing it for
	// the side effect is the documented way to enable it.
	_ "k8s.io/client-go/plugin/pkg/client/auth/exec"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

const userAgent = "k8sdockside"

// dialTimeout bounds how long we wait to reach an API server. It is a dial
// timeout rather than a request timeout on purpose: a request timeout would
// also cut every watch short, and watches are meant to stay open for hours.
const dialTimeout = 10 * time.Second

// callTimeout bounds a single one-shot request (discovery, a get behind the
// describe panel). A context pointing at an unreachable cluster has to surface
// as an error in the tab, never as a tab that spins forever.
const callTimeout = 20 * time.Second

// clusterClient is everything needed to talk to one cluster: a dynamic client,
// because every kind the UI shows -- built-in, Gateway API or a CRD nobody has
// heard of -- travels the same unstructured path, and a REST mapper to turn the
// kinds the UI names into the resources this particular server actually serves.
type clusterClient struct {
	dynamic dynamic.Interface
	// typed serves the one thing the dynamic client cannot reach: the Eviction
	// API a drain goes through, which is a subresource create with a body of
	// its own rather than a write to a resource.
	typed  kubernetes.Interface
	mapper meta.ResettableRESTMapper
	disco  discovery.CachedDiscoveryInterface
	host   string
	// cfg is the connection itself rather than a client built from it, kept
	// because two things here do not go through a client at all: an exec and a
	// port-forward both upgrade a single HTTP request into a stream, and the
	// dialer that does it is built from the config -- credentials, TLS and
	// proxy included -- not from the typed or dynamic client above.
	cfg *rest.Config
}

// newClusterClient builds the live clients for one kubeconfig context.
//
// Note that this reads the kubeconfig a second time, through client-go rather
// than through the parser in config.go. That is deliberate: config.go reads the
// subset needed to *list* contexts and is forgiving of files it cannot fully
// understand, while clientcmd reads the credentials needed to *connect* --
// client certificates, tokens and exec plugins -- which config.go deliberately
// never touches.
func newClusterClient(kc Context) (*clusterClient, error) {
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kc.File}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: kc.Name}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building a client for context %q: %w", kc.Name, err)
	}
	cfg.UserAgent = userAgent
	cfg.Dial = (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext
	// A desktop app opens several tabs at once, each starting an informer. The
	// client-go defaults (5 QPS, burst 10) would queue those behind each other
	// before the API server has had a chance to object.
	cfg.QPS, cfg.Burst = 50, 100

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client for context %q: %w", kc.Name, err)
	}

	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("client for context %q: %w", kc.Name, err)
	}

	disco, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("discovery client for context %q: %w", kc.Name, err)
	}
	cached := memory.NewMemCacheClient(disco)

	return &clusterClient{
		dynamic: dyn,
		typed:   typed,
		mapper:  restmapper.NewDeferredDiscoveryRESTMapper(cached),
		disco:   cached,
		host:    cfg.Host,
		cfg:     cfg,
	}, nil
}

// mappingFor resolves the REST mapping for a kind the UI asked for: which
// resource to watch, at which version, and whether it is namespaced. The mapper
// is consulted rather than a hard-coded table so that a kind served at more than
// one version resolves to whichever version *this* cluster prefers -- which is
// what makes one code path serve v1 and v1beta1 Gateway API alike.
func (c *clusterClient) mappingFor(gk schema.GroupKind) (*meta.RESTMapping, error) {
	m, err := c.mapper.RESTMapping(gk)
	if err == nil {
		return m, nil
	}
	// A kind the cluster does not serve is the ordinary case for optional APIs,
	// but so is one installed since we last looked. Reset the discovery cache
	// and ask once more before reporting it missing.
	c.mapper.Reset()
	m, err = c.mapper.RESTMapping(gk)
	if err != nil {
		return nil, err
	}
	return m, nil
}
