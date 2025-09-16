package k8s

import (
	"context"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	AnnotationPrefix = "application-networking.k8s.aws/"
	
	// Standalone annotation controls whether VPC Lattice services are created
	// without automatic service network association
	StandaloneAnnotation = AnnotationPrefix + "standalone"
)

// NamespacedName returns the namespaced name for k8s objects
func NamespacedName(obj client.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

func NamespaceOrDefault(namespace *gwv1.Namespace) string {
	if namespace == nil {
		return "default"
	}
	return string(*namespace)
}

func IsGVKSupported(mgr ctrl.Manager, groupVersion string, kind string) (bool, error) {
	gv, err := schema.ParseGroupVersion(groupVersion)
	if err != nil {
		return false, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return false, err
	}
	apiResources, err := discoveryClient.ServerResourcesForGroupVersion(gv.Group + "/" + gv.Version)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	for i := range apiResources.APIResources {
		if apiResources.APIResources[i].Kind == kind {
			return true, nil
		}
	}
	return false, nil
}

// validate if the gateway is managed by the lattice gateway controller
func IsControlledByLatticeGatewayController(ctx context.Context, c client.Client, gw *gwv1.Gateway) bool {
	gwClass := &gwv1.GatewayClass{}
	// GatewayClass is cluster-scoped resource, so we don't need to specify namespace
	err := c.Get(ctx, client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, gwClass)
	if err != nil {
		return false
	}
	return gwClass.Spec.ControllerName == config.LatticeGatewayControllerName
}

// FindControlledParents returns parent gateways that are controlled by lattice gateway controller
func FindControlledParents(ctx context.Context, client client.Client, route core.Route) ([]*gwv1.Gateway, error) {
	var result []*gwv1.Gateway
	gwNamespace := route.Namespace()
	misses := []string{}
	for _, parentRef := range route.Spec().ParentRefs() {
		gw := &gwv1.Gateway{}
		if parentRef.Namespace != nil {
			gwNamespace = string(*parentRef.Namespace)
		}
		gwName := types.NamespacedName{
			Namespace: gwNamespace,
			Name:      string(parentRef.Name),
		}
		if err := client.Get(ctx, gwName, gw); err != nil {
			misses = append(misses, gwName.String())
			continue
		}
		if IsControlledByLatticeGatewayController(ctx, client, gw) {
			result = append(result, gw)
		}
	}
	var err error
	if len(misses) > 0 {
		err = fmt.Errorf("failed to get gateways, %s", misses)
	}
	return result, err
}

func ObjExists(ctx context.Context, c client.Client, key types.NamespacedName, obj client.Object) (bool, error) {
	err := c.Get(ctx, key, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// IsStandaloneAnnotationEnabled checks if the standalone annotation is set to "true"
// on the given object. It returns false for any other value or if the annotation is missing.
func IsStandaloneAnnotationEnabled(obj client.Object) bool {
	if obj == nil {
		return false
	}
	
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	
	value, exists := annotations[StandaloneAnnotation]
	if !exists {
		return false
	}
	
	return ParseBoolAnnotation(value)
}

// ParseBoolAnnotation parses a string annotation value as a boolean.
// It returns true only if the value is "true" (case-insensitive).
// All other values, including empty string, return false.
func ParseBoolAnnotation(value string) bool {
	if value == "" {
		return false
	}
	
	// Convert to lowercase for case-insensitive comparison
	switch value {
	case "true", "True", "TRUE":
		return true
	default:
		return false
	}
}

// GetStandaloneModeForRoute determines if standalone mode should be enabled for a route.
// It checks the route-level annotation first (highest precedence), then falls back to
// the gateway-level annotation. Returns false if neither annotation is present or set to "true".
func GetStandaloneModeForRoute(ctx context.Context, c client.Client, route core.Route) (bool, error) {
	// Check route-level annotation first (highest precedence)
	routeAnnotations := route.K8sObject().GetAnnotations()
	if routeAnnotations != nil {
		if value, exists := routeAnnotations[StandaloneAnnotation]; exists {
			// Route-level annotation takes precedence regardless of value
			return ParseBoolAnnotation(value), nil
		}
	}
	
	// Check gateway-level annotation
	gateways, err := FindControlledParents(ctx, c, route)
	if err != nil {
		return false, fmt.Errorf("failed to find controlled parent gateways: %w", err)
	}
	
	// Check all parent gateways for standalone annotation
	for _, gw := range gateways {
		if IsStandaloneAnnotationEnabled(gw) {
			return true, nil
		}
	}
	
	return false, nil
}
