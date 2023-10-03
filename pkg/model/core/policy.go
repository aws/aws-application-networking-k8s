package core

import (
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type Policy interface {
	GetNamespacedName() types.NamespacedName
	GetTargetRef() *gwv1alpha2.PolicyTargetReference
}

type PolicyList interface {
	GetItems() []Policy
}
