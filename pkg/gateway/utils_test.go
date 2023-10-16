package gateway

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
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

func Test_GetAttachedPolicy(t *testing.T) {
	type args struct {
		refObjNamespacedName types.NamespacedName
		policy               core.Policy
	}
	type testCase struct {
		name                              string
		args                              args
		expectedK8sClientReturnedPolicies []core.Policy
		want                              []core.Policy
		expectPolicyNotFound              bool
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

	tgpTargetRefKindWrong := targetGroupPolicyHappyPath.DeepCopyObject().(*anv1alpha1.TargetGroupPolicy)
	tgpTargetRefKindWrong.Spec.TargetRef.Kind = "ServiceImport"

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

	iamAuthPolicyHappyPath := &anv1alpha1.IAMAuthPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-iam-auth-policy",
			Namespace: ns,
		},
		Spec: anv1alpha1.IAMAuthPolicySpec{
			TargetRef: &gwv1alpha2.PolicyTargetReference{
				Group:     gwv1alpha2.GroupName,
				Name:      "gw-1",
				Kind:      "Gateway",
				Namespace: &typedNs,
			},
			Policy: "policy content",
		},
	}

	notRelatedIAMAuthPolicy := iamAuthPolicyHappyPath.DeepCopyObject().(*anv1alpha1.IAMAuthPolicy)
	notRelatedIAMAuthPolicy.Spec.TargetRef.Name = "another-gw"

	accessLogPolicy1HappyPath := &anv1alpha1.AccessLogPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-iam-auth-policy",
			Namespace: ns,
		},
		Spec: anv1alpha1.AccessLogPolicySpec{
			TargetRef: &gwv1alpha2.PolicyTargetReference{
				Group:     gwv1alpha2.GroupName,
				Name:      "grpcroute-1",
				Kind:      "GRPCRoute",
				Namespace: &typedNs,
			},
			DestinationArn: aws.String("test-destination-arn-1"),
		},
	}
	accessLogPolicy2HappyPath := &anv1alpha1.AccessLogPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-iam-auth-policy",
			Namespace: ns,
		},
		Spec: anv1alpha1.AccessLogPolicySpec{
			TargetRef: &gwv1alpha2.PolicyTargetReference{
				Group:     gwv1alpha2.GroupName,
				Name:      "grpcroute-1",
				Kind:      "GRPCRoute",
				Namespace: &typedNs,
			},
			DestinationArn: aws.String("test-destination-arn-2"),
		},
	}
	notRelatedAccessLogPolicy := accessLogPolicy1HappyPath.DeepCopyObject().(*anv1alpha1.AccessLogPolicy)
	notRelatedAccessLogPolicy.Spec.TargetRef.Name = "another-grpcroute"

	alpTargetRefKindWrong := accessLogPolicy1HappyPath.DeepCopyObject().(*anv1alpha1.AccessLogPolicy)
	alpTargetRefKindWrong.Spec.TargetRef.Kind = "Service"

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
			expectedK8sClientReturnedPolicies: []core.Policy{targetGroupPolicyHappyPath},
			want:                              []core.Policy{targetGroupPolicyHappyPath},
			expectPolicyNotFound:              false,
		},
		{
			name: "Get gateway attached IAMAuthPolicy, happy path",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "gw-1",
				},
				policy: &anv1alpha1.IAMAuthPolicy{},
			},
			expectedK8sClientReturnedPolicies: []core.Policy{iamAuthPolicyHappyPath},
			want:                              []core.Policy{iamAuthPolicyHappyPath},
			expectPolicyNotFound:              false,
		},
		{
			name: "Get GRPCRoute attached AccessLogPolicy, happy path, be able to get all targetRef Matched policies",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "grpcroute-1",
				},
				policy: &anv1alpha1.AccessLogPolicy{},
			},
			expectedK8sClientReturnedPolicies: []core.Policy{accessLogPolicy1HappyPath, accessLogPolicy2HappyPath},
			want:                              []core.Policy{accessLogPolicy1HappyPath, accessLogPolicy2HappyPath},
			expectPolicyNotFound:              false,
		},
		{
			name: "Get k8sService attached TargetGroupPolicy, policy not found due to input refObj name mismatch",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "another-svc-1",
				},
				policy: &anv1alpha1.TargetGroupPolicy{},
			},
			want:                              nil,
			expectedK8sClientReturnedPolicies: []core.Policy{targetGroupPolicyHappyPath},
			expectPolicyNotFound:              true,
		},
		{
			name: "Get gateway attached IAMAuthPolicy, policy not found due to input refObj name mismatch",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "another-gw-1",
				},
				policy: &anv1alpha1.IAMAuthPolicy{},
			},
			want:                              nil,
			expectedK8sClientReturnedPolicies: []core.Policy{iamAuthPolicyHappyPath},
			expectPolicyNotFound:              true,
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
			want:                              nil,
			expectedK8sClientReturnedPolicies: nil,
			expectPolicyNotFound:              true,
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
			expectedK8sClientReturnedPolicies: []core.Policy{policyTargetRefSectionNil},
			want:                              nil,
			expectPolicyNotFound:              true,
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
			expectedK8sClientReturnedPolicies: []core.Policy{tgpTargetRefKindWrong},
			want:                              nil,
			expectPolicyNotFound:              true,
		},
		{
			name: "Get GRPCRoute attached AccessLogPolicy, policy not found due to policy targetRef Kind mismatch",
			args: args{
				refObjNamespacedName: types.NamespacedName{
					Namespace: ns,
					Name:      "grpcroute-1",
				},
				policy: &anv1alpha1.AccessLogPolicy{},
			},
			expectedK8sClientReturnedPolicies: []core.Policy{alpTargetRefKindWrong},
			want:                              nil,
			expectPolicyNotFound:              true,
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
			expectedK8sClientReturnedPolicies: []core.Policy{vpcAssociationPolicyHappyPath},
			want:                              []core.Policy{vpcAssociationPolicyHappyPath},
			expectPolicyNotFound:              false,
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
			expectedK8sClientReturnedPolicies: nil,
			want:                              nil,
			expectPolicyNotFound:              true,
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
						for _, p := range tt.expectedK8sClientReturnedPolicies {
							policy := p.(*anv1alpha1.TargetGroupPolicy)
							list.Items = append(list.Items, *policy)

						}
						return nil
					})
			} else if _, ok := tt.args.policy.(*anv1alpha1.VpcAssociationPolicy); ok {
				mockK8sClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, list *anv1alpha1.VpcAssociationPolicyList, arg3 ...interface{}) error {
						list.Items = append(list.Items, *notRelatedVpcAssociationPolicy)
						for _, p := range tt.expectedK8sClientReturnedPolicies {
							policy := p.(*anv1alpha1.VpcAssociationPolicy)
							list.Items = append(list.Items, *policy)

						}
						return nil
					})
			} else if _, ok := tt.args.policy.(*anv1alpha1.IAMAuthPolicy); ok {
				mockK8sClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, list *anv1alpha1.IAMAuthPolicyList, arg3 ...interface{}) error {
						list.Items = append(list.Items, *notRelatedIAMAuthPolicy)
						for _, p := range tt.expectedK8sClientReturnedPolicies {
							policy := p.(*anv1alpha1.IAMAuthPolicy)
							list.Items = append(list.Items, *policy)
						}
						return nil
					})
			} else if _, ok := tt.args.policy.(*anv1alpha1.AccessLogPolicy); ok {
				mockK8sClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, list *anv1alpha1.AccessLogPolicyList, arg3 ...interface{}) error {
						list.Items = append(list.Items, *notRelatedAccessLogPolicy)
						for _, p := range tt.expectedK8sClientReturnedPolicies {
							policy := p.(*anv1alpha1.AccessLogPolicy)
							list.Items = append(list.Items, *policy)
						}
						return nil
					})
			} else {
				t.Errorf("unexpected policy type: %v", tt.args.policy)
			}

			got, err := GetAttachedPolicies(ctx, mockK8sClient, tt.args.refObjNamespacedName, tt.args.policy)
			if tt.expectPolicyNotFound {
				assert.Nil(t, err)
				assert.Nil(t, got)
				return
			}
			assert.Equalf(t, tt.want, got, "GetAttachedPolicies(%v, %v, %v, %v)", ctx, mockK8sClient, tt.args.refObjNamespacedName, tt.args.policy)
		})
	}
}
