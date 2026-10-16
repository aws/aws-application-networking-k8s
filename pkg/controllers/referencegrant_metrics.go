package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	labelRouteKind      = "route_kind"
	labelRouteNamespace = "route_namespace"
	labelRouteName      = "route_name"
	labelBackendKind    = "backend_kind"
)

var backendKindBuckets = []string{"Service", "ServiceImport", "Other"}

// crossNamespaceRefsNotPermitted reports the current number of cross-namespace backendRefs
// on a route that no ReferenceGrant permits. It is a migration-safety signal for the warn
// phase: operators drive it to zero by adding grants or repointing refs before upgrading
// to a release that blocks such references. It is a gauge because the value must fall back
// to zero once the references are fixed.
var crossNamespaceRefsNotPermitted = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Subsystem: "referencegrant",
	Name:      "cross_namespace_refs_not_permitted",
	Help:      "Current number of cross-namespace backendRefs on a route with no matching ReferenceGrant.",
}, []string{labelRouteKind, labelRouteNamespace, labelRouteName, labelBackendKind})

func init() {
	ctrlmetrics.Registry.MustRegister(crossNamespaceRefsNotPermitted)
}

// bucketBackendKind collapses the backendRef kind to a bounded set so the metric label
// cannot be driven to unbounded cardinality by arbitrary user-supplied kinds.
func bucketBackendKind(kind string) string {
	switch kind {
	case "Service", "ServiceImport":
		return kind
	default:
		return "Other"
	}
}
