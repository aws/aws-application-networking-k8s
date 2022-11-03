package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func Test_MeshModelBuild(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name          string
		gw            *v1alpha2.Gateway
		wantErr       error
		wantName      string
		wantIsDeleted bool
	}{
		{
			name: "Adding Mesh",
			gw: &v1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mesh1",
				},
			},
			wantErr:       nil,
			wantName:      "mesh1",
			wantIsDeleted: false,
		},
		{
			name: "Deleting Mesh",
			gw: &v1alpha2.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mesh1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			wantErr:       nil,
			wantName:      "mesh1",
			wantIsDeleted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewServiceNetworkModelBuilder()

			_, got, err := builder.Build(context.Background(), tt.gw)

			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.Equal(t, tt.wantName, got.Spec.Name)
				assert.Equal(t, tt.wantIsDeleted, got.Spec.IsDeleted)
			}

		})
	}
}
