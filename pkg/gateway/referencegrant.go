package gateway

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

// IsBackendRefAllowedByReferenceGrant reports whether route may reference backendRef per
// the Gateway API ReferenceGrant contract. Same-namespace references are always allowed.
// A cross-namespace reference requires a ReferenceGrant in the backendRef's namespace
// whose From matches the route's group, kind and namespace and whose To matches the
// backendRef's group and kind (and name, when the grant restricts by name).
func IsBackendRefAllowedByReferenceGrant(
	ctx context.Context,
	c client.Client,
	route core.Route,
	backendRef core.BackendRef,
) (bool, error) {
	refNamespace := route.Namespace()
	if backendRef.Namespace() != nil {
		refNamespace = string(*backendRef.Namespace())
	}
	if refNamespace == route.Namespace() {
		return true, nil
	}

	grants := &gwv1.ReferenceGrantList{}
	if err := c.List(ctx, grants, client.InNamespace(refNamespace)); err != nil {
		return false, err
	}

	routeGK := route.GroupKind()
	backendGroup, backendKind := backendRefGroupKind(backendRef)

	for i := range grants.Items {
		spec := grants.Items[i].Spec
		if !grantFromMatches(spec.From, routeGK.Group, routeGK.Kind, route.Namespace()) {
			continue
		}
		if grantToMatches(spec.To, backendGroup, backendKind, string(backendRef.Name())) {
			return true, nil
		}
	}
	return false, nil
}

func backendRefGroupKind(backendRef core.BackendRef) (string, string) {
	kind := "Service"
	if backendRef.Kind() != nil {
		kind = string(*backendRef.Kind())
	}

	group := ""
	if backendRef.Group() != nil {
		group = string(*backendRef.Group())
	}
	// The model builder dispatches on kind alone, so normalize the group to the one a
	// grant's To section names for that kind: the core group ("") for a Service and the
	// application networking group for a ServiceImport, whatever group the ref sets.
	switch kind {
	case "Service":
		group = ""
	case "ServiceImport":
		group = anv1alpha1.GroupName
	}
	return group, kind
}

func grantFromMatches(froms []gwv1.ReferenceGrantFrom, group, kind, namespace string) bool {
	for _, from := range froms {
		if string(from.Group) == group &&
			string(from.Kind) == kind &&
			string(from.Namespace) == namespace {
			return true
		}
	}
	return false
}

func grantToMatches(tos []gwv1.ReferenceGrantTo, group, kind, name string) bool {
	for _, to := range tos {
		if string(to.Group) != group || string(to.Kind) != kind {
			continue
		}
		if to.Name == nil || string(*to.Name) == name {
			return true
		}
	}
	return false
}
