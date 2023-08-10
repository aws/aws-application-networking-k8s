package k8s

import (
	"k8s.io/apimachinery/pkg/types"
)

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
