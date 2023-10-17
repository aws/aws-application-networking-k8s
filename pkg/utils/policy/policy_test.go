package policy

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	mockclient "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

func Test_checkGatewayOrRouteTargetRefExistsForPolicy(t *testing.T) {
	testNamespace := gwv1beta1.Namespace("test-ns")

	notFoundError := fmt.Errorf("k8s resource not found")
	type args struct {
		policy core.Policy
	}
	tests := []struct {
		name                         string
		args                         args
		expectedK8sClientReturnedObj client.Object
		want                         bool
		wantErr                      error
	}{
		{
			name: "AccessLogPolicy TargetRef gateway exists in the k8s, happy path",
			args: args{
				policy: &anv1alpha1.AccessLogPolicy{
					TypeMeta:   metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{},
					Spec: anv1alpha1.AccessLogPolicySpec{
						DestinationArn: aws.String("destination-arn-123"),
						TargetRef: &gwv1alpha2.PolicyTargetReference{
							Group:     gwv1beta1.GroupName,
							Kind:      "Gateway",
							Name:      "test-gw",
							Namespace: &testNamespace,
						},
					},
				},
			},
			expectedK8sClientReturnedObj: &gwv1beta1.Gateway{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gw",
					Namespace: string(testNamespace),
				},
			},
			want:    true,
			wantErr: nil,
		},
		{
			name: "AccessLogPolicy TargetRef GRPCRoute exists in the k8s, happy path",
			args: args{
				policy: &anv1alpha1.AccessLogPolicy{
					TypeMeta:   metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{},
					Spec: anv1alpha1.AccessLogPolicySpec{
						DestinationArn: aws.String("destination-arn-123"),
						TargetRef: &gwv1alpha2.PolicyTargetReference{
							Group:     gwv1beta1.GroupName,
							Kind:      "GRPCRoute",
							Name:      "test-grpcroute",
							Namespace: &testNamespace,
						},
					},
				},
			},
			expectedK8sClientReturnedObj: &gwv1alpha2.GRPCRoute{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-grpcroute",
					Namespace: string(testNamespace),
				},
			},
			want:    true,
			wantErr: nil,
		},
		{
			name: "AccessLogPolicy TargetRef GRPCRoute not exist in the k8s",
			args: args{
				policy: &anv1alpha1.AccessLogPolicy{
					TypeMeta:   metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{},
					Spec: anv1alpha1.AccessLogPolicySpec{
						DestinationArn: aws.String("destination-arn-123"),
						TargetRef: &gwv1alpha2.PolicyTargetReference{
							Group:     gwv1beta1.GroupName,
							Kind:      "GRPCRoute",
							Name:      "another-grpcroute",
							Namespace: &testNamespace,
						},
					},
				},
			},
			expectedK8sClientReturnedObj: nil,
			want:                         false,
			wantErr:                      notFoundError,
		},
		{
			name: "IAMAuthPolicy TargetRef HTTPRoute exists in the k8s, happy path",
			args: args{
				policy: &anv1alpha1.IAMAuthPolicy{
					TypeMeta:   metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{},
					Spec: anv1alpha1.IAMAuthPolicySpec{
						Policy: "policy content",
						TargetRef: &gwv1alpha2.PolicyTargetReference{
							Group:     gwv1beta1.GroupName,
							Kind:      "HTTPRoute",
							Name:      "test-httproute",
							Namespace: &testNamespace,
						},
					},
				},
			},
			expectedK8sClientReturnedObj: &gwv1beta1.HTTPRoute{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-httproute",
					Namespace: string(testNamespace),
				},
			},
			want:    true,
			wantErr: nil,
		},
		{
			name: "IAMAuthPolicy TargetRef HTTPRoute not exist in the k8s",
			args: args{
				policy: &anv1alpha1.IAMAuthPolicy{
					TypeMeta:   metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{},
					Spec: anv1alpha1.IAMAuthPolicySpec{
						Policy: "policy content",
						TargetRef: &gwv1alpha2.PolicyTargetReference{
							Group:     gwv1beta1.GroupName,
							Kind:      "HTTPRoute",
							Name:      "not-matched-httproute",
							Namespace: &testNamespace,
						},
					},
				},
			},
			expectedK8sClientReturnedObj: nil,
			want:                         false,
			wantErr:                      notFoundError,
		},
	}
	for _, tt := range tests {
		ctx := context.TODO()
		c := gomock.NewController(t)

		t.Run(tt.name, func(t *testing.T) {
			k8sClient := mockclient.NewMockClient(c)

			switch tt.args.policy.GetTargetRef().Kind {
			case "Gateway":
				k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, name types.NamespacedName, retObj *gwv1beta1.Gateway, arg3 ...interface{}) error {
						if tt.expectedK8sClientReturnedObj == nil {
							return notFoundError
						} else {
							retObj.Name = tt.expectedK8sClientReturnedObj.(*gwv1beta1.Gateway).Name
							retObj.Namespace = tt.expectedK8sClientReturnedObj.(*gwv1beta1.Gateway).Namespace
							return nil
						}
					})
			case "HTTPRoute":
				k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, name types.NamespacedName, retObj *gwv1beta1.HTTPRoute, arg3 ...interface{}) error {
						if tt.expectedK8sClientReturnedObj == nil {
							return notFoundError
						} else {
							retObj.Name = tt.expectedK8sClientReturnedObj.(*gwv1beta1.HTTPRoute).Name
							retObj.Namespace = tt.expectedK8sClientReturnedObj.(*gwv1beta1.HTTPRoute).Namespace
							return nil
						}
					})
			case "GRPCRoute":
				k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, name types.NamespacedName, retObj *gwv1alpha2.GRPCRoute, arg3 ...interface{}) error {
						if tt.expectedK8sClientReturnedObj == nil {
							return notFoundError
						} else {
							retObj.Name = tt.expectedK8sClientReturnedObj.(*gwv1alpha2.GRPCRoute).Name
							retObj.Namespace = tt.expectedK8sClientReturnedObj.(*gwv1alpha2.GRPCRoute).Namespace
							return nil
						}
					})
			}

			got, err := checkGatewayOrRouteTargetRefExistsForPolicy(ctx, k8sClient, tt.args.policy)
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				return
			}
			assert.Equalf(t, tt.want, got, "checkGatewayOrRouteTargetRefExistsForPolicy(%v, %v, %v)", ctx, k8sClient, tt.args.policy)
			assert.Equal(t, nil, err)
		})
	}
}
