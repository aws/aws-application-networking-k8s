package core

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type Policy interface {
	GetNamespacedName() types.NamespacedName
	GetTargetRef() *v1alpha2.PolicyTargetReference
}
