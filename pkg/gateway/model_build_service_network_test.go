package gateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func Test_MeshModelBuild(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name           string
		gw             *gateway_api.Gateway
		wantErr        error
		wantName       string
		wantNamespace  string
		wantIsDeleted  bool
		associateToVPC bool
	}{
		{
			name: "Adding Mesh in default namespace, no annotation on VPC association",
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
	}

	for _, tt := range tests {
		fmt.Printf("Testing >>> %v\n", tt.name)
		t.Run(tt.name, func(t *testing.T) {
			builder := NewServiceNetworkModelBuilder()

			_, got, err := builder.Build(context.Background(), tt.gw)

			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.wantName, got.Spec.Name)
				assert.Equal(t, tt.wantNamespace, got.Spec.Namespace)
				assert.Equal(t, tt.wantIsDeleted, got.Spec.IsDeleted)
				assert.Equal(t, tt.associateToVPC, got.Spec.AssociateToVPC)
			}

		})
	}
}
