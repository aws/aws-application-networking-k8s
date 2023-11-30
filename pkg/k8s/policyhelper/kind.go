package policyhelper

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type GroupKind struct {
	Group string
	Kind  string
}

func ObjToGroupKind(obj client.Object) GroupKind {
	switch obj.(type) {
	case *gwv1beta1.Gateway:
		return GroupKind{gwv1beta1.GroupName, "Gateway"}
	case *gwv1beta1.HTTPRoute:
		return GroupKind{gwv1beta1.GroupName, "HTTPRoute"}
	case *gwv1alpha2.GRPCRoute:
		return GroupKind{gwv1alpha2.GroupName, "GRPCRoute"}
	case *corev1.Service:
		return GroupKind{corev1.GroupName, "Service"}
	default:
		return GroupKind{}
	}
}

func TargetRefGroupKind(tr *TargetRef) GroupKind {
	return GroupKind{
		Group: string(tr.Group),
		Kind:  string(tr.Kind),
	}
}

func GroupKindToObj(gk GroupKind) (client.Object, bool) {
	switch gk {
	case GroupKind{gwv1beta1.GroupName, "Gateway"}:
		return &gwv1beta1.Gateway{}, true
	case GroupKind{gwv1beta1.GroupName, "HTTPRoute"}:
		return &gwv1beta1.HTTPRoute{}, true
	case GroupKind{gwv1alpha2.GroupName, "GRPCRoute"}:
		return &gwv1alpha2.GRPCRoute{}, true
	case GroupKind{corev1.GroupName, "Service"}:
		return &corev1.Service{}, true
	default:
		return nil, false
	}
}
