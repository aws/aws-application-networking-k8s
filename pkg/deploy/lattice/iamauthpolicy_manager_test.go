package lattice

import (
	"context"
	"testing"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestIAMAuthPolicyManager(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := mocks.NewMockLattice(c)
	cfg := pkg_aws.CloudConfig{VpcId: "vpc-id", AccountId: "account-id"}
	cl := pkg_aws.NewDefaultCloud(mockLattice, cfg)
	ctx := context.Background()

	m := IAMAuthPolicyManager{
		Cloud: cl,
	}

	t.Run("existing sn", func(t *testing.T) {
		snId := "sn-abc"

		policy := model.IAMAuthPolicy{
			ResourceId: snId,
			Policy:     "{}",
		}

		mockLattice.EXPECT().
			PutAuthPolicyWithContext(gomock.Any(), &vpclattice.PutAuthPolicyInput{
				Policy:             &policy.Policy,
				ResourceIdentifier: &snId,
			}).
			Return(&vpclattice.PutAuthPolicyOutput{
				Policy: &policy.Policy,
				State:  aws.String(vpclattice.AuthPolicyStateActive),
			}, nil).Times(1)

		statusGot, _ := m.Put(ctx, policy)
		statusWant := model.IAMAuthPolicyStatus{
			ResourceId: snId,
			State:      vpclattice.AuthPolicyStateActive,
		}
		assert.Equal(t, statusWant, statusGot)
	})

	t.Run("existing svc", func(t *testing.T) {
		svcId := "svc-abc"

		policy := model.IAMAuthPolicy{
			ResourceId: svcId,
			Policy:     "{}",
		}

		mockLattice.EXPECT().
			PutAuthPolicyWithContext(gomock.Any(), &vpclattice.PutAuthPolicyInput{
				Policy:             &policy.Policy,
				ResourceIdentifier: &svcId,
			}).
			Return(&vpclattice.PutAuthPolicyOutput{
				Policy: &policy.Policy,
				State:  aws.String(vpclattice.AuthPolicyStateActive),
			}, nil).Times(1)

		statusGot, _ := m.Put(ctx, policy)
		statusWant := model.IAMAuthPolicyStatus{
			ResourceId: svcId,
			State:      vpclattice.AuthPolicyStateActive,
		}
		assert.Equal(t, statusWant, statusGot)
	})

	t.Run("enable SN IAM Auth", func(t *testing.T) {
		snId := "snId"

		mockLattice.EXPECT().
			UpdateServiceNetworkWithContext(ctx, &vpclattice.UpdateServiceNetworkInput{
				AuthType:                 aws.String(vpclattice.AuthTypeAwsIam),
				ServiceNetworkIdentifier: &snId,
			}).Return(&vpclattice.UpdateServiceNetworkOutput{}, nil).Times(1)

		m.EnableSnIAMAuth(ctx, snId)
	})

	t.Run("enable Svc IAM Auth", func(t *testing.T) {
		svcId := "svcId"

		mockLattice.EXPECT().
			UpdateServiceWithContext(ctx, &vpclattice.UpdateServiceInput{
				AuthType:          aws.String(vpclattice.AuthTypeAwsIam),
				ServiceIdentifier: &svcId,
			}).Return(&vpclattice.UpdateServiceOutput{}, nil).Times(1)

		m.EnableSvcIAMAuth(ctx, svcId)
	})
}
