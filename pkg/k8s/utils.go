package k8s

import (
	"context"
	"fmt"
	"strings"

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
// This function implements defensive programming - it handles nil objects and missing
// annotations gracefully.
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

// ValidateStandaloneAnnotation validates the standalone annotation value on an object.
// It returns the parsed boolean value and any validation errors.
// This function provides detailed validation feedback for debugging and error reporting.
func ValidateStandaloneAnnotation(obj client.Object) (bool, error) {
	if obj == nil {
		return false, fmt.Errorf("object cannot be nil")
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		// No annotations is valid - defaults to false
		return false, nil
	}

	value, exists := annotations[StandaloneAnnotation]
	if !exists {
		// Missing annotation is valid - defaults to false
		return false, nil
	}

	// Validate the annotation value
	if value == "" {
		return false, fmt.Errorf("standalone annotation cannot be empty")
	}

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false, fmt.Errorf("standalone annotation cannot be whitespace only")
	}

	// Check for valid values
	switch strings.ToLower(trimmed) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		// Invalid values are treated as false but we report the validation error
		return false, fmt.Errorf("invalid standalone annotation value '%s', expected 'true' or 'false'", trimmed)
	}
}

// GetStandaloneModeForRouteWithValidation determines standalone mode with detailed validation.
// This function provides enhanced error reporting for debugging annotation issues.
// It returns the standalone mode, validation warnings, and any critical errors.
func GetStandaloneModeForRouteWithValidation(ctx context.Context, c client.Client, route core.Route) (bool, []string, error) {
	var warnings []string

	// Validate input parameters
	if route == nil {
		return false, nil, fmt.Errorf("route cannot be nil")
	}
	if route.K8sObject() == nil {
		return false, nil, fmt.Errorf("route K8s object cannot be nil")
	}

	// Check route-level annotation first (highest precedence)
	routeAnnotations := route.K8sObject().GetAnnotations()
	if routeAnnotations != nil {
		if _, exists := routeAnnotations[StandaloneAnnotation]; exists {
			// Validate route-level annotation
			standalone, err := ValidateStandaloneAnnotation(route.K8sObject())
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("route annotation validation: %v, treating as false", err))
				return false, warnings, nil
			}
			return standalone, warnings, nil
		}
	}

	// Check gateway-level annotation with enhanced error handling
	gateways, err := FindControlledParents(ctx, c, route)
	if err != nil {
		// Handle gateway lookup failures gracefully based on context
		if route.DeletionTimestamp() != nil && !route.DeletionTimestamp().IsZero() {
			// During deletion, gateway lookup failures are acceptable
			warnings = append(warnings, fmt.Sprintf("gateway lookup failed during deletion: %v", err))
			return false, warnings, nil
		}

		// For non-deletion scenarios, gateway lookup failures should be reported
		return false, warnings, fmt.Errorf("failed to find controlled parent gateways for route %s/%s: %w",
			route.Namespace(), route.Name(), err)
	}

	// Check all parent gateways for standalone annotation with validation
	for _, gw := range gateways {
		if gw.GetAnnotations() != nil {
			if _, exists := gw.GetAnnotations()[StandaloneAnnotation]; exists {
				standalone, err := ValidateStandaloneAnnotation(gw)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("gateway %s/%s annotation validation: %v, treating as false",
						gw.GetNamespace(), gw.GetName(), err))
					continue
				}
				if standalone {
					return true, warnings, nil
				}
			} else {
				// Debug: log when gateway doesn't have the annotation
				warnings = append(warnings, fmt.Sprintf("gateway %s/%s does not have standalone annotation",
					gw.GetNamespace(), gw.GetName()))
			}
		} else {
			// Debug: log when gateway has no annotations
			warnings = append(warnings, fmt.Sprintf("gateway %s/%s has no annotations",
				gw.GetNamespace(), gw.GetName()))
		}
	}

	return false, warnings, nil
}

// ParseBoolAnnotation parses a string annotation value as a boolean.
// It returns true only if the value is "true" (case-insensitive).
// All other values, including empty string, return false.
// This function is designed to be forgiving - any invalid or unexpected
// values are treated as false to ensure graceful degradation.
func ParseBoolAnnotation(value string) bool {
	if value == "" {
		return false
	}

	// Trim whitespace to be more forgiving of user input
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}

	// Convert to lowercase for case-insensitive comparison
	switch strings.ToLower(trimmed) {
	case "true":
		return true
	default:
		// All other values (false, invalid, etc.) are treated as false
		// This provides graceful degradation for invalid annotation values
		return false
	}
}

// GetStandaloneModeForRoute determines if standalone mode should be enabled for a route.
// It checks the route-level annotation first (highest precedence), then falls back to
// the gateway-level annotation. Returns false if neither annotation is present or set to "true".
// This function implements graceful error handling - gateway lookup failures are handled
// appropriately based on the context (deletion vs normal operation).
func GetStandaloneModeForRoute(ctx context.Context, c client.Client, route core.Route) (bool, error) {
	// Validate input parameters
	if route == nil {
		return false, fmt.Errorf("route cannot be nil")
	}
	if route.K8sObject() == nil {
		return false, fmt.Errorf("route K8s object cannot be nil")
	}

	// Check route-level annotation first (highest precedence)
	routeAnnotations := route.K8sObject().GetAnnotations()
	if routeAnnotations != nil {
		if value, exists := routeAnnotations[StandaloneAnnotation]; exists {
			// Route-level annotation takes precedence regardless of value
			// ParseBoolAnnotation handles validation and treats invalid values as false
			standalone := ParseBoolAnnotation(value)
			return standalone, nil
		}
	}

	// Check gateway-level annotation with enhanced error handling
	gateways, err := FindControlledParents(ctx, c, route)
	if err != nil {
		// Handle gateway lookup failures gracefully based on context
		if route.DeletionTimestamp() != nil && !route.DeletionTimestamp().IsZero() {
			// During deletion, gateway lookup failures are acceptable
			// Return false (non-standalone) as a safe default
			return false, nil
		}

		// For non-deletion scenarios, gateway lookup failures should be reported
		// but we still return a safe default to allow processing to continue
		return false, fmt.Errorf("failed to find controlled parent gateways for route %s/%s: %w",
			route.Namespace(), route.Name(), err)
	}

	// Check all parent gateways for standalone annotation
	for _, gw := range gateways {
		if IsStandaloneAnnotationEnabled(gw) {
			return true, nil
		}
	}

	return false, nil
}
