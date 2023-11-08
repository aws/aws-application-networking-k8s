package k8s

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

const AnnotationPrefix = "application-networking.k8s.aws/"

type NamespacedAndNamed interface {
	GetNamespace() string
	GetName() string
}

// NamespacedName returns the namespaced name for k8s objects
func NamespacedName(obj NamespacedAndNamed) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

func NamespaceOrDefault(namespace *v1beta1.Namespace) string {
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
