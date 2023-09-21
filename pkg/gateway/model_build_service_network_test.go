package gateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
)

func Test_MeshModelBuild(t *testing.T) {
	now := metav1.Now()
	trueBool := true
	falseBool := false
	notRelatedVpcAssociationPolicy := v1alpha1.VpcAssociationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "another-vpc-association-policy",
		},
		Spec: v1alpha1.VpcAssociationPolicySpec{
			TargetRef: &v1alpha2.PolicyTargetReference{
				Group: gateway_api.GroupName,
				Kind:  "Gateway",
				Name:  "another-mesh",
			},
			AssociateWithVpc: &falseBool,
		},
	}
	tests := []struct {
		name                 string
		gw                   *gateway_api.Gateway
		vpcAssociationPolicy *v1alpha1.VpcAssociationPolicy
		wantErr              error
		wantName             string
		wantNamespace        string
		wantIsDeleted        bool
		associateToVPC       bool
	}{
		{
			name: "Adding Mesh in default namespace, no annotation on VPC association, associate to VPC by default",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mesh1",
				},
			},
			wantErr:        nil,
			wantName:       "mesh1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: true,
		},
		{
			name: "Adding Mesh in non-default namespace, no annotation on VPC association",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mesh1",
					Namespace: "non-default",
				},
			},
			wantErr:        nil,
			wantName:       "mesh1",
			wantNamespace:  "non-default",
			wantIsDeleted:  false,
			associateToVPC: true,
		},
		{
			name: "Adding Mesh, and need VPC association",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "mesh1",
					Annotations: map[string]string{LatticeVPCAssociationAnnotation: "true"},
				},
			},
			wantErr:        nil,
			wantName:       "mesh1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: true,
		},
		{
			name: "Adding Mesh, and need VPC association",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "mesh1",
					Annotations: map[string]string{LatticeVPCAssociationAnnotation: "false"},
				},
			},
			wantErr:        nil,
			wantName:       "mesh1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: false,
		},
		{
			name: "Deleting Mesh",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mesh1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			wantErr:        nil,
			wantName:       "mesh1",
			wantNamespace:  "",
			wantIsDeleted:  true,
			associateToVPC: true,
		},
		{
			name: "Gateway has attached VpcAssociationPolicy found, VpcAssociationPolicy SecurityGroupIds are not empty",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "mesh1",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
			},
			vpcAssociationPolicy: &v1alpha1.VpcAssociationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-vpc-association-policy",
				},
				Spec: v1alpha1.VpcAssociationPolicySpec{
					TargetRef: &v1alpha2.PolicyTargetReference{
						Group: gateway_api.GroupName,
						Kind:  "Gateway",
						Name:  "mesh1",
					},
					SecurityGroupIds: []v1alpha1.SecurityGroupId{"sg-123456", "sg-654321"},
				},
			},
			wantErr:        nil,
			wantName:       "mesh1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: true,
		},
		{
			name: "Gateway does not have LatticeVPCAssociationAnnotation, it has attached VpcAssociationPolicy found, which AssociateWithVpc field set to true",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "mesh1",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
			},
			vpcAssociationPolicy: &v1alpha1.VpcAssociationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-vpc-association-policy",
				},
				Spec: v1alpha1.VpcAssociationPolicySpec{
					TargetRef: &v1alpha2.PolicyTargetReference{
						Group: gateway_api.GroupName,
						Kind:  "Gateway",
						Name:  "mesh1",
					},
					AssociateWithVpc: &trueBool,
				},
			},
			wantErr:        nil,
			wantName:       "mesh1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: true,
		},
		{
			name: "Gateway does not have LatticeVPCAssociationAnnotation, it has attached VpcAssociationPolicy found, which AssociateWithVpc field set to false",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "mesh1",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
			},
			vpcAssociationPolicy: &v1alpha1.VpcAssociationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-vpc-association-policy",
				},
				Spec: v1alpha1.VpcAssociationPolicySpec{
					TargetRef: &v1alpha2.PolicyTargetReference{
						Group: gateway_api.GroupName,
						Kind:  "Gateway",
						Name:  "mesh1",
					},
					AssociateWithVpc: &falseBool,
				},
			},
			wantErr:        nil,
			wantName:       "mesh1",
			wantNamespace:  "",
			wantIsDeleted:  false,
			associateToVPC: false,
		},
	}
	c := gomock.NewController(t)
	defer c.Finish()
	mock_client := mock_client.NewMockClient(c)
	ctx := context.Background()

	for _, tt := range tests {
		fmt.Printf("Testing >>> %v\n", tt.name)
		t.Run(tt.name, func(t *testing.T) {
			mock_client.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, policyList *v1alpha1.VpcAssociationPolicyList, arg3 ...interface{}) error {
					policyList.Items = append(policyList.Items, notRelatedVpcAssociationPolicy)
					if tt.vpcAssociationPolicy != nil {
						policyList.Items = append(policyList.Items, *tt.vpcAssociationPolicy)
					}
					return nil
				},
			)
			builder := NewServiceNetworkModelBuilder(mock_client)
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
