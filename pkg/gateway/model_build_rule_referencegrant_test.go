package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func ruleWithBackend(name, namespace, kind string) *gwv1.HTTPRouteRule {
	return &gwv1.HTTPRouteRule{
		BackendRefs: []gwv1.HTTPBackendRef{{
			BackendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name:      gwv1.ObjectName(name),
					Namespace: ptr.To(gwv1.Namespace(namespace)),
					Kind:      ptr.To(gwv1.Kind(kind)),
				},
			},
		}},
	}
}

func buildRuleTargetGroups(t *testing.T, c client.Client, deleting bool, rule *gwv1.HTTPRouteRule) (*dummyTgBuilder, []*model.RuleTargetGroup, error) {
	t.Helper()
	httpRoute := gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route",
			Namespace: "foo",
			// Without this, a missing ServiceImport would be tolerated rather than fail the
			// build, so the ServiceImport case could not show the lookup was skipped.
			Annotations: map[string]string{"application-networking.k8s.aws/service-name-override": "svc-override"},
		},
		Spec: gwv1.HTTPRouteSpec{Rules: []gwv1.HTTPRouteRule{*rule}},
	}
	if deleting {
		now := metav1.Now()
		httpRoute.DeletionTimestamp = &now
	}
	route := core.NewHTTPRoute(httpRoute)
	tgBuilder := &dummyTgBuilder{}
	task := &latticeServiceModelBuildTask{
		log:         gwlog.FallbackLogger,
		route:       route,
		stack:       core.NewDefaultStack(core.StackID{Namespace: "foo", Name: "route"}),
		client:      c,
		brTgBuilder: tgBuilder,
	}
	tgs, err := task.getTargetGroupsForRuleAction(context.Background(), route.Spec().Rules()[0])
	return tgBuilder, tgs, err
}

func TestGetTargetGroupsForRuleAction_ReferenceGrant(t *testing.T) {
	setReferenceGrantServed(t, true)
	tests := []struct {
		name        string
		rule        *gwv1.HTTPRouteRule
		grants      []client.Object
		deleting    bool
		wantInvalid bool
	}{
		{
			name:        "cross-namespace Service without grant is invalid",
			rule:        ruleWithBackend("svc", "bar", "Service"),
			wantInvalid: true,
		},
		{
			// The ServiceImport does not exist and the route sets service-name-override,
			// which fails the build if the ServiceImport is looked up.
			name:        "cross-namespace ServiceImport without grant is invalid",
			rule:        ruleWithBackend("imp", "bar", "ServiceImport"),
			wantInvalid: true,
		},
		{
			name:   "cross-namespace Service with grant is built",
			rule:   ruleWithBackend("svc", "bar", "Service"),
			grants: []client.Object{grant("bar", httpRouteFrom("foo"), serviceTo(""))},
		},
		{
			name:     "deleting route skips the check",
			rule:     ruleWithBackend("svc", "bar", "Service"),
			deleting: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tgBuilder, tgs, err := buildRuleTargetGroups(t, rgTestClient(t, tt.grants...), tt.deleting, tt.rule)
			assert.NoError(t, err)
			assert.Len(t, tgs, 1)
			if tt.wantInvalid {
				assert.Equal(t, model.InvalidBackendRefTgId, tgs[0].StackTargetGroupId)
				assert.Zero(t, tgBuilder.i, "a blocked backend must not be built")
			} else {
				assert.NotEqual(t, model.InvalidBackendRefTgId, tgs[0].StackTargetGroupId)
				assert.Equal(t, 1, tgBuilder.i)
			}
		})
	}
}

func TestGetTargetGroupsForRuleAction_GrantListErrorFailsBuild(t *testing.T) {
	setReferenceGrantServed(t, true)
	c := testclient.NewClientBuilder().
		WithScheme(rgScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return assert.AnError
			},
		}).Build()

	_, _, err := buildRuleTargetGroups(t, c, false, ruleWithBackend("svc", "bar", "Service"))
	assert.ErrorIs(t, err, assert.AnError)
}
