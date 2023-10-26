package lattice

import (
	"context"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

type IAMAuthPolicyManager struct {
	Cloud pkg_aws.Cloud
}

func (m *IAMAuthPolicyManager) Put(ctx context.Context, policy model.IAMAuthPolicy) (model.IAMAuthPolicyStatus, error) {
	req := &vpclattice.PutAuthPolicyInput{
		Policy:             &policy.Policy,
		ResourceIdentifier: &policy.ResourceId,
	}
	resp, err := m.Cloud.Lattice().PutAuthPolicyWithContext(ctx, req)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	return model.IAMAuthPolicyStatus{
		ResourceId: policy.ResourceId,
		State:      *resp.State,
	}, nil
}

func (m *IAMAuthPolicyManager) Delete(ctx context.Context, resourceId string) error {
	req := &vpclattice.DeleteAuthPolicyInput{
		ResourceIdentifier: &resourceId,
	}
	_, err := m.Cloud.Lattice().DeleteAuthPolicyWithContext(ctx, req)
	if err != nil {
		return err
	}
	return nil
}

func (m *IAMAuthPolicyManager) EnableSnIAMAuth(ctx context.Context, snId string) error {
	req := &vpclattice.UpdateServiceNetworkInput{
		AuthType:                 aws.String(vpclattice.AuthTypeAwsIam),
		ServiceNetworkIdentifier: &snId,
	}
	_, err := m.Cloud.Lattice().UpdateServiceNetworkWithContext(ctx, req)
	return err
}

func (m *IAMAuthPolicyManager) EnableSvcIAMAuth(ctx context.Context, svcId string) error {
	req := &vpclattice.UpdateServiceInput{
		AuthType:          aws.String(vpclattice.AuthTypeAwsIam),
		ServiceIdentifier: &svcId,
	}
	_, err := m.Cloud.Lattice().UpdateServiceWithContext(ctx, req)
	return err
}
