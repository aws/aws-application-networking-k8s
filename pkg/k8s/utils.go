package k8s

import (
	"context"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const AnnotationPrefix = "application-networking.k8s.aws/"

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
