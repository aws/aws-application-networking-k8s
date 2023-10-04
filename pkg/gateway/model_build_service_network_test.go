package gateway

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
)

func Test_SNModelBuild(t *testing.T) {
	now := metav1.Now()
	trueBool := true
	falseBool := false
	notRelatedVpcAssociationPolicy := anv1alpha1.VpcAssociationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "another-vpc-association-policy",
		},
		Spec: anv1alpha1.VpcAssociationPolicySpec{
			TargetRef: &gwv1alpha2.PolicyTargetReference{
				Group: gwv1beta1.GroupName,
				Kind:  "Gateway",
				Name:  "another-gw",
			},
			AssociateWithVpc: &falseBool,
		},
	}
	tests := []struct {
		name                 string
		gw                   *gwv1beta1.Gateway
		vpcAssociationPolicy *anv1alpha1.VpcAssociationPolicy
		wantErr              error
		wantName             string
		wantNamespace        string
		wantIsDeleted        bool
		associateToVPC       bool
	}{
		{
			name: "Adding SN in default namespace, no annotation on VPC association, associate to VPC by default",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gw1",
				},
			},
			wantErr:        nil,
			wantName:       "gw1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: true,
		},
		{
			name: "Adding SN in non-default namespace, no annotation on VPC association",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw1",
					Namespace: "non-default",
				},
			},
			wantErr:        nil,
			wantName:       "gw1",
			wantNamespace:  "non-default",
			wantIsDeleted:  false,
			associateToVPC: true,
		},
		{
			name: "Adding SN, and need VPC association",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "gw1",
					Annotations: map[string]string{LatticeVPCAssociationAnnotation: "true"},
				},
			},
			wantErr:        nil,
			wantName:       "gw1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: true,
		},
		{
			name: "Adding SN, and need VPC association",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "gw1",
					Annotations: map[string]string{LatticeVPCAssociationAnnotation: "false"},
				},
			},
			wantErr:        nil,
			wantName:       "gw1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: false,
		},
		{
			name: "Deleting SN",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "gw1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			wantErr:        nil,
			wantName:       "gw1",
			wantNamespace:  "",
			wantIsDeleted:  true,
			associateToVPC: true,
		},
		{
			name: "Gateway has attached VpcAssociationPolicy found, VpcAssociationPolicy SecurityGroupIds are not empty",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "gw1",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
			},
			vpcAssociationPolicy: &anv1alpha1.VpcAssociationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-vpc-association-policy",
				},
				Spec: anv1alpha1.VpcAssociationPolicySpec{
					TargetRef: &gwv1alpha2.PolicyTargetReference{
						Group: gwv1beta1.GroupName,
						Kind:  "Gateway",
						Name:  "gw1",
					},
					SecurityGroupIds: []anv1alpha1.SecurityGroupId{"sg-123456", "sg-654321"},
				},
			},
			wantErr:        nil,
			wantName:       "gw1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: true,
		},
		{
			name: "Gateway does not have LatticeVPCAssociationAnnotation, it has attached VpcAssociationPolicy found, which AssociateWithVpc field set to true",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "gw1",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
			},
			vpcAssociationPolicy: &anv1alpha1.VpcAssociationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-vpc-association-policy",
				},
				Spec: anv1alpha1.VpcAssociationPolicySpec{
					TargetRef: &gwv1alpha2.PolicyTargetReference{
						Group: gwv1beta1.GroupName,
						Kind:  "Gateway",
						Name:  "gw1",
					},
					AssociateWithVpc: &trueBool,
				},
			},
			wantErr:        nil,
			wantName:       "gw1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: true,
		},
		{
			name: "Gateway does not have LatticeVPCAssociationAnnotation, it has attached VpcAssociationPolicy found, which AssociateWithVpc field set to false",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "gw1",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
			},
			vpcAssociationPolicy: &anv1alpha1.VpcAssociationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-vpc-association-policy",
				},
				Spec: anv1alpha1.VpcAssociationPolicySpec{
					TargetRef: &gwv1alpha2.PolicyTargetReference{
						Group: gwv1beta1.GroupName,
						Kind:  "Gateway",
						Name:  "gw1",
					},
					AssociateWithVpc: &falseBool,
				},
			},
			wantErr:        nil,
			wantName:       "gw1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: false,
		},
	}
	c := gomock.NewController(t)
	defer c.Finish()
	mockClient := mock_client.NewMockClient(c)
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, policyList *anv1alpha1.VpcAssociationPolicyList, arg3 ...interface{}) error {
					policyList.Items = append(policyList.Items, notRelatedVpcAssociationPolicy)
					if tt.vpcAssociationPolicy != nil {
						policyList.Items = append(policyList.Items, *tt.vpcAssociationPolicy)
					}
					return nil
				},
			)
			builder := NewServiceNetworkModelBuilder(mockClient)
			_, got, err := builder.Build(context.Background(), tt.gw)

			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.wantName, got.Spec.Name)
				assert.Equal(t, tt.wantNamespace, got.Spec.Namespace)
				assert.Equal(t, tt.wantIsDeleted, got.Spec.IsDeleted)
				assert.Equal(t, tt.associateToVPC, got.Spec.AssociateToVPC)
				if tt.vpcAssociationPolicy != nil {
					assert.Equal(t, securityGroupIdsToStringPointersSlice(tt.vpcAssociationPolicy.Spec.SecurityGroupIds), got.Spec.SecurityGroupIds)
				}
			}
		})
	}
}
