package policyhelper

import (
	"testing"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
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
	objs := []client.Object{&gwv1.Gateway{}, &gwv1.HTTPRoute{}, &gwv1.GRPCRoute{}}
	gks := NewGroupKindSet(objs...)
	assert.True(t, gks.Contains(GroupKind{gwv1.GroupName, "Gateway"}))
	assert.True(t, gks.Contains(GroupKind{gwv1.GroupName, "HTTPRoute"}))
	assert.True(t, gks.Contains(GroupKind{gwv1.GroupName, "GRPCRoute"}))
}
