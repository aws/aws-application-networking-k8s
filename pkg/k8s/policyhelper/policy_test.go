package policyhelper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
)

func TestPolicyClient(t *testing.T) {
	type iap = anv1alpha1.IAMAuthPolicy
	type iapl = anv1alpha1.IAMAuthPolicyList

	t.Run("new list and policy", func(t *testing.T) {
		c := newK8sPolicyClient[iap, iapl](nil)
		assert.NotNil(t, c.newPolicy())
		assert.NotNil(t, c.newList())
	})
}

func TestPolicyHandler(t *testing.T) {
	type iap = anv1alpha1.IAMAuthPolicy
	type iapl = anv1alpha1.IAMAuthPolicyList

	phcfg := PolicyHandlerConfig{}
	_ = NewPolicyHandler[iap, iapl](phcfg)
}

func TestGroupKindSet(t *testing.T) {
	objs := []client.Object{&gwv1beta1.Gateway{}, &gwv1beta1.HTTPRoute{}, &gwv1alpha2.GRPCRoute{}}
	gks := NewGroupKindSet(objs...)
	assert.True(t, gks.Contains(GroupKind{gwv1beta1.GroupName, "Gateway"}))
	assert.True(t, gks.Contains(GroupKind{gwv1beta1.GroupName, "HTTPRoute"}))
	assert.True(t, gks.Contains(GroupKind{gwv1alpha2.GroupName, "GRPCRoute"}))
}
