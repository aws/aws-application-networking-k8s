package eventhandlers

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

const routeBackendRefIndex = "route.backendRefs"

func backendRefIndexKey(kind, namespace, name string) string {
	return kind + "/" + namespace + "/" + name
}

// routeBackendRefKeys skips backendRefs without a kind, since isBackendRefUsedByRoute never matches them.
func routeBackendRefKeys(obj client.Object) []string {
	route, err := core.NewRoute(obj)
	if err != nil {
		return nil
	}
	var keys []string
	for _, rule := range route.Spec().Rules() {
		for _, backendRef := range rule.BackendRefs() {
			if backendRef.Kind() == nil {
				continue
			}
			namespace := route.Namespace()
			if backendRef.Namespace() != nil {
				namespace = string(*backendRef.Namespace())
			}
			keys = append(keys, backendRefIndexKey(string(*backendRef.Kind()), namespace, string(backendRef.Name())))
		}
	}
	return keys
}

// IndexRouteBackendRefs lets backendRefToRoutes look up routes by backendRef instead of listing every route.
func IndexRouteBackendRefs(ctx context.Context, indexer client.FieldIndexer) error {
	for _, obj := range []client.Object{&gwv1.HTTPRoute{}, &gwv1.GRPCRoute{}, &gwv1.TLSRoute{}} {
		if err := indexer.IndexField(ctx, obj, routeBackendRefIndex, routeBackendRefKeys); err != nil {
			return err
		}
	}
	return nil
}
