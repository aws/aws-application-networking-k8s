package core

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type Policy interface {
	GetNamespacedName() types.NamespacedName
	GetTargetRef() *gwv1alpha2.PolicyTargetReference
	GetStatusConditions() []metav1.Condition
	SetStatusConditions(conditions []metav1.Condition)
}

type PolicyList interface {
	GetItems() []Policy
}
