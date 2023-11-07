package policyhelper

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
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
		name                            string
		args                            args
		expectedK8sClientReturnedPolicy core.Policy
		want                            core.Policy
		expectPolicyNotFound            bool
	}
	ns := "test-ns"
	typedNs := gwv1alpha2.Namespace(ns)
	prtocol := "HTTP"
	protocolVersion := "HTTP2"
	trueBool := true
	targetGroupPolicyHappyPath := &anv1alpha1.TargetGroupPolicy{
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
			Protocol:        &prtocol,
			ProtocolVersion: &protocolVersion,
		},
	}

	policyTargetRefSectionNil := targetGroupPolicyHappyPath.DeepCopyObject().(*anv1alpha1.TargetGroupPolicy)
	policyTargetRefSectionNil.Spec.TargetRef = nil

	policyTargetRefKindWrong := targetGroupPolicyHappyPath.DeepCopyObject().(*anv1alpha1.TargetGroupPolicy)
	policyTargetRefKindWrong.Spec.TargetRef.Kind = "ServiceImport"

	notRelatedTargetGroupPolicy := targetGroupPolicyHappyPath.DeepCopyObject().(*anv1alpha1.TargetGroupPolicy)
	notRelatedTargetGroupPolicy.Spec.TargetRef.Name = "another-svc"

	vpcAssociationPolicyHappyPath := &anv1alpha1.VpcAssociationPolicy{
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
			AssociateWithVpc: &trueBool,
		},
	}

	notRelatedVpcAssociationPolicy := vpcAssociationPolicyHappyPath.DeepCopyObject().(*anv1alpha1.VpcAssociationPolicy)
	notRelatedVpcAssociationPolicy.Spec.TargetRef.Name = "another-gw"

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
			expectedK8sClientReturnedPolicy: targetGroupPolicyHappyPath,
			want:                            targetGroupPolicyHappyPath,
			expectPolicyNotFound:            false,
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
			want:                            nil,
			expectedK8sClientReturnedPolicy: targetGroupPolicyHappyPath,
			expectPolicyNotFound:            true,
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
			want:                            nil,
			expectedK8sClientReturnedPolicy: nil,
			expectPolicyNotFound:            true,
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
			expectedK8sClientReturnedPolicy: policyTargetRefSectionNil,
			want:                            nil,
			expectPolicyNotFound:            true,
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
			expectedK8sClientReturnedPolicy: policyTargetRefKindWrong,
			want:                            nil,
			expectPolicyNotFound:            true,
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
			expectedK8sClientReturnedPolicy: vpcAssociationPolicyHappyPath,
			want:                            vpcAssociationPolicyHappyPath,
			expectPolicyNotFound:            false,
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
			expectedK8sClientReturnedPolicy: nil,
			want:                            nil,
			expectPolicyNotFound:            true,
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
						list.Items = append(list.Items, *notRelatedTargetGroupPolicy)
						if tt.expectedK8sClientReturnedPolicy != nil {
							policy := tt.expectedK8sClientReturnedPolicy.(*anv1alpha1.TargetGroupPolicy)
							list.Items = append(list.Items, *policy)
						}
						return nil
					})
			} else if _, ok := tt.args.policy.(*anv1alpha1.VpcAssociationPolicy); ok {
				mockK8sClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, list *anv1alpha1.VpcAssociationPolicyList, arg3 ...interface{}) error {
						list.Items = append(list.Items, *notRelatedVpcAssociationPolicy)
						if tt.expectedK8sClientReturnedPolicy != nil {
							policy := tt.expectedK8sClientReturnedPolicy.(*anv1alpha1.VpcAssociationPolicy)
							list.Items = append(list.Items, *policy)
						}
						return nil
					})
			}

			got, err := GetAttachedPolicies(ctx, mockK8sClient, tt.args.refObjNamespacedName, tt.args.policy)
			if tt.expectPolicyNotFound {
				assert.Nil(t, err)
				assert.Empty(t, got)
				return
			}
			assert.Contains(t, got, tt.want, "GetAttachedPolicies(%v, %v, %v, %v)", ctx, mockK8sClient, tt.args.refObjNamespacedName, tt.args.policy)
		})
	}
}
