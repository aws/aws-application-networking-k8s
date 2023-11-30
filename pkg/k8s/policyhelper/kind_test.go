package policyhelper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestGroupKind(t *testing.T) {
	type Test struct {
		obj  client.Object
		kind GroupKind
	}

	tests := []Test{
		{&gwv1beta1.Gateway{}, GroupKind{Group: gwv1beta1.GroupName, Kind: "Gateway"}},
		{&gwv1beta1.HTTPRoute{}, GroupKind{Group: gwv1beta1.GroupName, Kind: "HTTPRoute"}},
		{&gwv1alpha2.GRPCRoute{}, GroupKind{Group: gwv1alpha2.GroupName, Kind: "GRPCRoute"}},
		{&corev1.Service{}, GroupKind{Group: corev1.GroupName, Kind: "Service"}},
	}

	t.Run("obj to kind", func(t *testing.T) {
		for _, tt := range tests {
			assert.Equal(t, ObjToGroupKind(tt.obj), tt.kind)
		}
	})

	t.Run("kind to obj", func(t *testing.T) {
		for _, tt := range tests {
			kind, _ := GroupKindToObj(tt.kind)
			assert.Equal(t, kind, tt.obj)
		}
	})
}
