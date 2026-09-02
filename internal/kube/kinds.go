package kube

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GatewayGroup is the Gateway API's group. Ingress is deprecated but still in
// wide use, so both are offered; these kinds are not a replacement for the
// Network group, they sit beside it.
const GatewayGroup = "gateway.networking.k8s.io"

// Gateway API and CRD kinds, continuing the contract in resources.go: these
// strings are what the frontend catalogue sends to BuildTable and Subscribe.
const (
	KindGatewayClasses  = "gatewayclasses"
	KindGateways        = "gateways"
	KindHTTPRoutes      = "httproutes"
	KindGRPCRoutes      = "grpcroutes"
	KindReferenceGrants = "referencegrants"
	KindCRDs            = "customresourcedefinitions"
)

// CustomPrefix marks a kind that names a custom resource rather than one of the
// kinds compiled in above: "crd:<plural>.<group>", e.g.
// "crd:certificates.cert-manager.io".
//
// It is a prefixed string rather than a structured field because a tab's kind
// is persisted (appconfig.TabRef.Kind) and travels through the frontend's tab
// machinery untouched; making it a string keeps every one of those layers
// unchanged. Both sides parse it in exactly one place: here, and labelFor() in
// the frontend catalogue.
const CustomPrefix = "crd:"

// CustomKind builds the kind string for a custom resource.
func CustomKind(plural, group string) string {
	return CustomPrefix + plural + "." + group
}

// ParseCustomKind splits a "crd:" kind back into its plural and group. A group
// is required: every CRD has one, since custom resources may not live in the
// core group.
func ParseCustomKind(kind string) (plural, group string, ok bool) {
	rest, found := strings.CutPrefix(kind, CustomPrefix)
	if !found {
		return "", "", false
	}
	plural, group, found = strings.Cut(rest, ".")
	if !found || plural == "" || group == "" {
		return "", "", false
	}
	return plural, group, true
}

// builtinKinds maps the kinds the UI offers by name onto their API group and
// kind. Versions are deliberately absent: the REST mapper picks whichever
// version the cluster in front of us actually serves, which is what lets one
// entry cover both v1 and v1beta1 of the Gateway API.
var builtinKinds = map[string]schema.GroupKind{
	KindPods:        {Kind: "Pod"},
	KindNamespaces:  {Kind: "Namespace"},
	KindNodes:       {Kind: "Node"},
	KindServices:    {Kind: "Service"},
	KindConfigMaps:  {Kind: "ConfigMap"},
	KindSecrets:     {Kind: "Secret"},
	KindPVCs:        {Kind: "PersistentVolumeClaim"},
	KindEvents:      {Kind: "Event"},
	KindDeployments: {Group: "apps", Kind: "Deployment"},
	KindStatefulSet: {Group: "apps", Kind: "StatefulSet"},
	KindDaemonSets:  {Group: "apps", Kind: "DaemonSet"},
	KindJobs:        {Group: "batch", Kind: "Job"},
	KindCronJobs:    {Group: "batch", Kind: "CronJob"},
	KindIngresses:   {Group: "networking.k8s.io", Kind: "Ingress"},

	KindGatewayClasses:  {Group: GatewayGroup, Kind: "GatewayClass"},
	KindGateways:        {Group: GatewayGroup, Kind: "Gateway"},
	KindHTTPRoutes:      {Group: GatewayGroup, Kind: "HTTPRoute"},
	KindGRPCRoutes:      {Group: GatewayGroup, Kind: "GRPCRoute"},
	KindReferenceGrants: {Group: GatewayGroup, Kind: "ReferenceGrant"},

	KindCRDs: {Group: "apiextensions.k8s.io", Kind: "CustomResourceDefinition"},
}

// mappingForKind resolves a kind named by the UI to the resource to watch.
//
// Custom resources take the other branch: they are named by plural and group
// rather than by kind, because that is what a CRD's own identity is, so they
// resolve through the resource side of the mapper.
func (c *clusterClient) mappingForKind(kind string) (*meta.RESTMapping, error) {
	if plural, group, ok := ParseCustomKind(kind); ok {
		gvr := schema.GroupVersionResource{Group: group, Resource: plural}
		gvk, err := c.mapper.KindFor(gvr)
		if err != nil {
			c.mapper.Reset()
			if gvk, err = c.mapper.KindFor(gvr); err != nil {
				return nil, notServed(plural+"."+group, group, err)
			}
		}
		m, err := c.mappingFor(gvk.GroupKind())
		if err != nil {
			return nil, notServed(plural+"."+group, group, err)
		}
		return m, nil
	}

	gk, ok := builtinKinds[kind]
	if !ok {
		return nil, fmt.Errorf("unknown resource kind: %s", kind)
	}
	m, err := c.mappingFor(gk)
	if err != nil {
		return nil, notServed(kind, gk.Group, err)
	}
	return m, nil
}

// notServed explains a kind the cluster has no mapping for.
//
// The Gateway API and every CRD are optional, so this is an ordinary state
// rather than a fault, and the message has to say what would fix it: naming the
// API group turns "no matches for kind GRPCRoute" into something the reader can
// act on.
func notServed(kind, group string, err error) error {
	if !meta.IsNoMatchError(err) {
		return fmt.Errorf("looking up %s: %w", kind, err)
	}
	if group == "" {
		return fmt.Errorf("this cluster does not serve %s", kind)
	}
	return fmt.Errorf("this cluster does not serve %s -- the %s API is not installed", kind, group)
}

// ErrNotServed reports whether an error came from a kind this cluster does not
// serve, which the UI treats as an empty view rather than a failure.
func ErrNotServed(err error) bool {
	return err != nil && strings.Contains(err.Error(), "does not serve")
}
