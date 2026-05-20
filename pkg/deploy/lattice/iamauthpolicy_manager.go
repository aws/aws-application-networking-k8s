package lattice

import (
	"context"
	"encoding/json"
	"reflect"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"

	"github.com/aws/aws-sdk-go/aws"
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
	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, policy.Name)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	resourceId := *sn.SvcNetwork.Id

	// Skip the rewrite if Lattice already reflects the desired auth policy
	// and AuthType=AWS_IAM. AuthPolicyState is "Active" only when both are
	// true. This avoids a steady-state stream of PutAuthPolicy and
	// UpdateServiceNetwork mutation calls on every drift-detection pass when
	// nothing has actually drifted.
	inSync, err := m.isAuthPolicyInSync(ctx, resourceId, policy.Policy)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	if inSync {
		return model.IAMAuthPolicyStatus{ResourceId: resourceId}, nil
	}

	err = m.putPolicy(ctx, resourceId, policy.Policy)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	err = m.enableSnIAMAuth(ctx, resourceId)
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

	// See putSn for the rationale; same skip-if-already-in-sync logic.
	inSync, err := m.isAuthPolicyInSync(ctx, resourceId, policy.Policy)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	if inSync {
		return model.IAMAuthPolicyStatus{ResourceId: resourceId}, nil
	}

	err = m.putPolicy(ctx, resourceId, policy.Policy)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	err = m.enableSvcIAMAuth(ctx, resourceId)
	if err != nil {
		return model.IAMAuthPolicyStatus{}, err
	}
	return model.IAMAuthPolicyStatus{ResourceId: resourceId}, nil
}

// isAuthPolicyInSync reports whether the given resource (service or service
// network) already has the desired auth policy attached and AuthType=AWS_IAM.
// It returns false (i.e. "needs rewrite") if the resource has no policy
// attached, if AuthType is not AWS_IAM, or if the attached policy doc is not
// semantically equivalent to the desired one.
//
// Errors from GetAuthPolicy are surfaced to the caller, with one exception:
// a NotFound response is treated as "needs rewrite" rather than an error.
// This handles the case where GetAuthPolicy returns NotFound for a resource
// that exists but has never had a policy attached. Note that the API may
// also return success with an empty Policy field for the same case.
//
// Conservative on uncertainty: any unexpected shape (e.g. policy document
// that won't parse as JSON) is treated as drifted, which costs at most one
// extra rewrite. Never prefer skipping over rewriting when in doubt — a
// silent skip when we should have written would leave the user's auth
// policy not applied.
func (m *IAMAuthPolicyManager) isAuthPolicyInSync(ctx context.Context, resourceId, desiredPolicy string) (bool, error) {
	out, err := m.cloud.Lattice().GetAuthPolicyWithContext(ctx, &vpclattice.GetAuthPolicyInput{
		ResourceIdentifier: &resourceId,
	})
	if err != nil {
		if services.IsNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	if aws.StringValue(out.State) != vpclattice.AuthPolicyStateActive {
		return false, nil
	}
	return policyDocsEqual(aws.StringValue(out.Policy), desiredPolicy), nil
}

// policyDocsEqual compares two IAM-style JSON policy documents for semantic
// equality. Whitespace and key ordering do not affect the result; list
// ordering does (lists are treated as ordered, the conservative choice — if
// Lattice ever returns a list in a different order than the user wrote it,
// we'll do one extra rewrite, which is harmless).
//
// Returns false if either side is empty or fails to parse as JSON. Never
// panics.
func policyDocsEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	var aDoc, bDoc any
	if err := json.Unmarshal([]byte(a), &aDoc); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &bDoc); err != nil {
		return false
	}
	return reflect.DeepEqual(aDoc, bDoc)
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
		sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, policy.Name)
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
