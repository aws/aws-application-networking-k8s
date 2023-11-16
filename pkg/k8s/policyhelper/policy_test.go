package policyhelper

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apimachineryv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

func Test_GetAttachedPolicies(t *testing.T) {
	type args struct {
		refObjNamespacedName types.NamespacedName
		policy               core.Policy
	}
	type testCase struct {
		name        string
		args        args
		k8sReturned []core.Policy
		want        []core.Policy
	}
	ns := "test-ns"
	typedNs := gwv1alpha2.Namespace(ns)
	tgpHappyPath := &anv1alpha1.TargetGroupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-target-group-policy",
			Namespace: ns,
		},
		Spec: anv1alpha1.TargetGroupPolicySpec{
			TargetRef: &gwv1alpha2.PolicyTargetReference{
				Group:     corev1.GroupName,
				Name:      "svc-1",
				Kind:      "Service",
				Namespace: &typedNs,
			},
		},
	}

	tgpTargetRefSectionNil := tgpHappyPath.DeepCopy()
	tgpTargetRefSectionNil.Spec.TargetRef = nil

	tgpTargetRefKindWrong := tgpHappyPath.DeepCopy()
	tgpTargetRefKindWrong.Spec.TargetRef.Kind = "ServiceImport"

	tgpUnrelated := tgpHappyPath.DeepCopy()
	tgpUnrelated.Spec.TargetRef.Name = "svc-unrelated"

	tgpDuplicated := tgpHappyPath.DeepCopy()
	tgpDuplicated.ObjectMeta.Name = "test-policy-2"

	vapHappyPath := &anv1alpha1.VpcAssociationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vpc-association-policy",
			Namespace: ns,
		},
		Spec: anv1alpha1.VpcAssociationPolicySpec{
			TargetRef: &gwv1alpha2.PolicyTargetReference{
				Group: gwv1alpha2.GroupName,
				Name:  "gw-1",
				Kind:  "Gateway",
			},
			SecurityGroupIds: []anv1alpha1.SecurityGroupId{"sg-1", "sg-2"},
		},
	}

	vapUnrelated := vapHappyPath.DeepCopy()
	vapUnrelated.Spec.TargetRef.Name = "gw-unrelated"

	var tests = []testCase{
		{
			name: "Get k8sService attached TargetGroupPolicy, happy path",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "svc-1",
				},
				policy: &anv1alpha1.TargetGroupPolicy{},
			},
			k8sReturned: []core.Policy{tgpHappyPath, tgpUnrelated},
			want:        []core.Policy{tgpHappyPath},
		},
		{
			name: "Get multiple k8sService attached TargetGroupPolicies, happy path",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "svc-1",
				},
				policy: &anv1alpha1.TargetGroupPolicy{},
			},
			k8sReturned: []core.Policy{tgpHappyPath, tgpDuplicated, tgpUnrelated},
			want:        []core.Policy{tgpHappyPath, tgpDuplicated},
		},
		{
			name: "Get k8sService attached TargetGroupPolicy, policy not found due to input refObj name mismatch",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "another-svc",
				},
				policy: &anv1alpha1.TargetGroupPolicy{},
			},
			k8sReturned: []core.Policy{tgpHappyPath, tgpUnrelated},
			want:        nil,
		},
		{
			name: "Get k8sService attached TargetGroupPolicy, policy not found due to cluster don't have matched policy",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "svc-1",
				},
				policy: &anv1alpha1.TargetGroupPolicy{},
			},
			k8sReturned: []core.Policy{tgpUnrelated},
			want:        nil,
		},
		{
			name: "Get k8sService attached TargetGroupPolicy, policy not found due to policy targetRef section is nil",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "svc-1",
				},
				policy: &anv1alpha1.TargetGroupPolicy{},
			},
			k8sReturned: []core.Policy{tgpTargetRefSectionNil, tgpUnrelated},
			want:        nil,
		},
		{
			name: "Get k8sService attached TargetGroupPolicy, policy not found due to policy targetRef Kind mismatch",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "svc-1",
				},
				policy: &anv1alpha1.TargetGroupPolicy{},
			},
			k8sReturned: []core.Policy{tgpTargetRefKindWrong, tgpUnrelated},
			want:        nil,
		},
		{
			name: "Get k8sGateway attached VpcAssociationPolicy, happy path",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "gw-1",
				},
				policy: &anv1alpha1.VpcAssociationPolicy{},
			},
			k8sReturned: []core.Policy{vapHappyPath, vapUnrelated},
			want:        []core.Policy{vapHappyPath},
		},
		{
			name: "Get k8sGateway attached VpcAssociationPolicy, Not found",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "gw-1",
				},
				policy: &anv1alpha1.VpcAssociationPolicy{},
			},
			k8sReturned: []core.Policy{vapUnrelated},
			want:        nil,
		},
	}
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockK8sClient := mock_client.NewMockClient(c)

			if _, ok := tt.args.policy.(*anv1alpha1.TargetGroupPolicy); ok {
				mockK8sClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, list *anv1alpha1.TargetGroupPolicyList, arg3 ...interface{}) error {
						for _, item := range tt.k8sReturned {
							list.Items = append(list.Items, (*item.(*anv1alpha1.TargetGroupPolicy)))
						}
						return nil
					})
			} else if _, ok := tt.args.policy.(*anv1alpha1.VpcAssociationPolicy); ok {
				mockK8sClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, list *anv1alpha1.VpcAssociationPolicyList, arg3 ...interface{}) error {
						for _, item := range tt.k8sReturned {
							list.Items = append(list.Items, (*item.(*anv1alpha1.VpcAssociationPolicy)))
						}
						return nil
					})
			}

			got, err := GetAttachedPolicies(ctx, mockK8sClient, tt.args.refObjNamespacedName, tt.args.policy)
			assert.Nil(t, err)
			if len(tt.want) == 0 {
				assert.Empty(t, got)
				return
			}
			assert.ElementsMatch(t, got, tt.want)
		})
	}
}

func TestPolicyConditionMatch(t *testing.T) {
	type Condition = apimachineryv1.Condition
	type Reason = gwv1alpha2.PolicyConditionReason

	type Test struct {
		name   string
		cnds   []Condition
		filter []Reason
		want   bool
	}

	tests := []Test{
		{"nil", nil, nil, true},
		{"nil filter", []Condition{{}}, nil, true},
		{"nil conditions with filter", nil, []Reason{"x"}, false},
		{"0 len filter", []Condition{{}}, []Reason{}, true},
		{"condition not found", []Condition{{}}, []Reason{"Accepted"}, false},
		{
			"condition does not match",
			[]Condition{{Type: string(gwv1alpha2.PolicyConditionAccepted), Reason: "NotReconciled"}},
			[]Reason{"Accepted"}, false,
		},
		{
			"condtion match",
			[]Condition{{Type: string(gwv1alpha2.PolicyConditionAccepted), Reason: "Accepted"}},
			[]Reason{"Accepted"},
			true,
		},
		{
			"condtion match mutli-filter",
			[]Condition{{Type: string(gwv1alpha2.PolicyConditionAccepted), Reason: "Accepted"}},
			[]Reason{"NotFound", "Accepted"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := policyConditionMatch(tt.cnds, tt.filter)
			if match != tt.want {
				t.Errorf("filter does not match, cnds=%v, filter=%v, want=%t", tt.cnds, tt.filter, tt.want)
			}
		})
	}

}
