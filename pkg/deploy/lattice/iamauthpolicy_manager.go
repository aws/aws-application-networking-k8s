package lattice

import (
	"context"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"

	"github.com/aws/aws-sdk-go/service/vpclattice"
)

type IAMAuthPolicyManager struct {
	cloud pkg_aws.Cloud
}

func NewIAMAuthPolicyManager(cloud pkg_aws.Cloud) *IAMAuthPolicyManager {
	return &IAMAuthPolicyManager{cloud: cloud}
}

func (m *IAMAuthPolicyManager) Put(ctx context.Context, policy model.IAMAuthPolicy) (model.IAMAuthPolicyStatus, error) {
	switch policy.Type {
	case model.ServiceNetworkType:
		return m.putSn(ctx, policy)
	case model.ServiceType:
		return m.putSvc(ctx, policy)
	default:
		panic("unknown policy resource type: " + policy.Type)
	}
}

func (m *IAMAuthPolicyManager) putSn(ctx context.Context, policy model.IAMAuthPolicy) (model.IAMAuthPolicyStatus, error) {
	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, policy.Name, "")
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	resourceId := *sn.SvcNetwork.Id
	err = m.enableSnIAMAuth(ctx, resourceId)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	err = m.putPolicy(ctx, resourceId, policy.Policy)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	return model.IAMAuthPolicyStatus{ResourceId: resourceId}, nil
}

func (m *IAMAuthPolicyManager) putSvc(ctx context.Context, policy model.IAMAuthPolicy) (model.IAMAuthPolicyStatus, error) {
	svc, err := m.cloud.Lattice().FindService(ctx, policy.Name)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	resourceId := *svc.Id
	err = m.enableSvcIAMAuth(ctx, resourceId)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	err = m.putPolicy(ctx, resourceId, policy.Policy)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	return model.IAMAuthPolicyStatus{ResourceId: resourceId}, nil
}

func (m *IAMAuthPolicyManager) putPolicy(ctx context.Context, id, policy string) error {
	req := &vpclattice.PutAuthPolicyInput{
		Policy:             &policy,
		ResourceIdentifier: &id,
	}
	_, err := m.cloud.Lattice().PutAuthPolicyWithContext(ctx, req)
	return err
}

func (m *IAMAuthPolicyManager) Delete(ctx context.Context, policy model.IAMAuthPolicy) (model.IAMAuthPolicyStatus, error) {
	switch policy.Type {
	case model.ServiceNetworkType:
		return m.deleteSn(ctx, policy)
	case model.ServiceType:
		return m.deleteSvc(ctx, policy)
	default:
		panic("unknown policy resource type: " + policy.Type)
	}
}

func (m *IAMAuthPolicyManager) deleteSn(ctx context.Context, policy model.IAMAuthPolicy) (model.IAMAuthPolicyStatus, error) {
	if policy.ResourceId == "" {
		sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, policy.Name, "")
		if err != nil {
			return model.IAMAuthPolicyStatus{}, err
		}
		policy.ResourceId = *sn.SvcNetwork.Id
	}
	err := m.disableSnIAMAuth(ctx, policy.ResourceId)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	err = m.deletePolicy(ctx, policy.ResourceId)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	return model.IAMAuthPolicyStatus{ResourceId: policy.ResourceId}, nil
}

func (m *IAMAuthPolicyManager) deleteSvc(ctx context.Context, policy model.IAMAuthPolicy) (model.IAMAuthPolicyStatus, error) {
	if policy.ResourceId == "" {
		svc, err := m.cloud.Lattice().FindService(ctx, policy.Name)
		if err != nil {
			return model.IAMAuthPolicyStatus{}, err
		}
		policy.ResourceId = *svc.Id
	}
	err := m.disableSvcIAMAuth(ctx, policy.ResourceId)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	err = m.deletePolicy(ctx, policy.ResourceId)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	return model.IAMAuthPolicyStatus{ResourceId: policy.ResourceId}, nil
}

func (m *IAMAuthPolicyManager) deletePolicy(ctx context.Context, resId string) error {
	req := &vpclattice.DeleteAuthPolicyInput{ResourceIdentifier: &resId}
	_, err := m.cloud.Lattice().DeleteAuthPolicy(req)
	return err
}

func (m *IAMAuthPolicyManager) enableSnIAMAuth(ctx context.Context, snId string) error {
	return m.setSnAuthType(ctx, snId, vpclattice.AuthTypeAwsIam)
}

func (m *IAMAuthPolicyManager) disableSnIAMAuth(ctx context.Context, snId string) error {
	return m.setSnAuthType(ctx, snId, vpclattice.AuthTypeNone)
}

func (m *IAMAuthPolicyManager) setSnAuthType(ctx context.Context, snId, authType string) error {
	req := &vpclattice.UpdateServiceNetworkInput{
		AuthType:                 &authType,
		ServiceNetworkIdentifier: &snId,
	}
	_, err := m.cloud.Lattice().UpdateServiceNetworkWithContext(ctx, req)
	return err
}

func (m *IAMAuthPolicyManager) enableSvcIAMAuth(ctx context.Context, svcId string) error {
	return m.setSvcAuthType(ctx, svcId, vpclattice.AuthTypeAwsIam)
}

func (m *IAMAuthPolicyManager) disableSvcIAMAuth(ctx context.Context, svcId string) error {
	return m.setSvcAuthType(ctx, svcId, vpclattice.AuthTypeNone)
}

func (m *IAMAuthPolicyManager) setSvcAuthType(ctx context.Context, svcId, authType string) error {
	req := &vpclattice.UpdateServiceInput{
		AuthType:          &authType,
		ServiceIdentifier: &svcId,
	}
	_, err := m.cloud.Lattice().UpdateServiceWithContext(ctx, req)
	return err
}
