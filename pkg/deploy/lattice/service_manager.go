package lattice

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

//go:generate mockgen -destination service_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice ServiceManager

type Service = latticemodel.Service
type ServiceStatus = latticemodel.ServiceStatus
type CreateSvcReq = vpclattice.CreateServiceInput
type CreateSvcResp = vpclattice.CreateServiceOutput
type UpdateSvcReq = vpclattice.UpdateServiceInput
type UpdateSvcResp = vpclattice.UpdateServiceOutput
type CreateSnSvcAssocReq = vpclattice.CreateServiceNetworkServiceAssociationInput
type CreateSnSvcAssocResp = vpclattice.CreateServiceNetworkServiceAssociationOutput
type DelSnSvcAssocReq = vpclattice.DeleteServiceNetworkServiceAssociationInput
type DelSnSvcAssocResp = vpclattice.DeleteServiceNetworkServiceAssociationOutput
type GetSvcReq = vpclattice.GetServiceInput
type SvcSummary = vpclattice.ServiceSummary
type GetResourcesTagsReq = vpclattice.ListTagsForResourceInput
type GetResourcesTagsResp = vpclattice.ListTagsForResourceOutput
type ListSnSvcAssocsReq = vpclattice.ListServiceNetworkServiceAssociationsInput
type SnSvcAssocSummary = vpclattice.ServiceNetworkServiceAssociationSummary

type ServiceManager interface {
	Create(ctx context.Context, service *latticemodel.Service) (latticemodel.ServiceStatus, error)
	Delete(ctx context.Context, service *latticemodel.Service) error
}

type defaultServiceManager struct {
	cloud     lattice_aws.Cloud
	datastore *latticestore.LatticeDataStore
}

func NewServiceManager(cloud lattice_aws.Cloud, latticeDataStore *latticestore.LatticeDataStore) *defaultServiceManager {
	return &defaultServiceManager{
		cloud:     cloud,
		datastore: latticeDataStore,
	}
}

// find service by name, if not found returns nil without error
func (m *defaultServiceManager) getService(ctx context.Context, svcName string) (*vpclattice.ServiceSummary, error) {
	svcsReq := vpclattice.ListServicesInput{}
	svcsResp, err := m.cloud.Lattice().ListServicesAsList(ctx, &svcsReq)
	if err != nil {
		return nil, err
	}
	for _, r := range svcsResp {
		if aws.StringValue(r.Name) == svcName {
			return r, nil
		}
	}
	return nil, nil
}

func (m *defaultServiceManager) createServiceAndSnAssoc(ctx context.Context, svc *Service) (ServiceStatus, error) {
	emtpyStatus := ServiceStatus{}

	createSvcReq := m.newCreateSvcReq(svc)
	createSvcResp, err := m.cloud.Lattice().CreateServiceWithContext(ctx, createSvcReq)
	if err != nil {
		return emtpyStatus, err
	}

	for _, snName := range svc.Spec.ServiceNetworkNames {
		err = m.createSnSvcAssoc(ctx, createSvcResp.Id, snName)
		if err != nil {
			return emtpyStatus, err
		}
	}
	status := svcStatusFromCreateSvcResp(createSvcResp)
	return status, nil
}

func (m *defaultServiceManager) createSnSvcAssoc(ctx context.Context, svcId *string, snName string) error {
	sn, err := m.datastore.GetServiceNetworkStatus(snName, m.cloud.Config().AccountId)
	if err != nil {
		return err
	}
	assocReq := &CreateSnSvcAssocReq{
		ServiceIdentifier:        svcId,
		ServiceNetworkIdentifier: aws.String(sn.ID),
		Tags:                     m.cloud.NewTagsWithManagedBy(),
	}
	assocResp, err := m.cloud.Lattice().CreateServiceNetworkServiceAssociationWithContext(ctx, assocReq)
	if err != nil {
		return err
	}
	err = handleSnSvcAssocResp(assocResp)
	if err != nil {
		return err
	}
	return nil
}

func (m *defaultServiceManager) deleteSnSvcAssoc(ctx context.Context, assocArn *string) error {
	delReq := &DelSnSvcAssocReq{ServiceNetworkServiceAssociationIdentifier: assocArn}
	_, err := m.cloud.Lattice().DeleteServiceNetworkServiceAssociationWithContext(ctx, delReq)
	if err != nil {
		return err
	}
	return nil
}

func (m *defaultServiceManager) updateServiceAndSnAssoc(ctx context.Context, svc *Service, svcSum *SvcSummary) (ServiceStatus, error) {
	emptyStatus := ServiceStatus{}

	if svc.Spec.CustomerCertARN != "" {
		updReq := &UpdateSvcReq{
			CertificateArn:    aws.String(svc.Spec.CustomerCertARN),
			ServiceIdentifier: svcSum.Id,
		}
		if updReq != nil {
			_, err := m.cloud.Lattice().UpdateService(updReq)
			if err != nil {
				return emptyStatus, err
			}
		}
	}

	err := m.updateSnSvcAssocs(ctx, svc, svcSum)
	if err != nil {
		return emptyStatus, err
	}

	status := ServiceStatus{
		Arn: aws.StringValue(svcSum.Arn),
		Id:  aws.StringValue(svcSum.Id),
	}
	if svcSum.DnsEntry != nil {
		status.Dns = aws.StringValue(svcSum.DnsEntry.DomainName)
	}
	return status, nil
}

// update SN-Svc associations, if svc has no SN associations will delete all of them
// does not delete associations that are not tagged by controller
func (m *defaultServiceManager) updateSnSvcAssocs(ctx context.Context, svc *Service, svcSum *SvcSummary) error {
	assocsReq := &ListSnSvcAssocsReq{
		ServiceIdentifier: svcSum.Id,
	}
	assocs, err := m.cloud.Lattice().ListServiceNetworkServiceAssociationsAsList(ctx, assocsReq)
	if err != nil {
		return err
	}
	toCreate, toDelete, err := snSvcAssocsDiff(svc, assocs)
	if err != nil {
		return err
	}
	for _, snName := range toCreate {
		err := m.createSnSvcAssoc(ctx, svcSum.Id, snName)
		if err != nil {
			return err
		}
	}

	for _, assoc := range toDelete {
		isManaged, err := m.cloud.IsArnManaged(assoc.Arn)
		if err != nil {
			return err
		}
		if !isManaged {
			continue
		}
		m.deleteSnSvcAssoc(ctx, assoc.Arn)
	}

	return nil
}

// returns RetryErr on all non-active Sn-Svc association responses
func handleSnSvcAssocResp(resp *CreateSnSvcAssocResp) error {
	status := aws.StringValue(resp.Status)
	if status != vpclattice.ServiceNetworkServiceAssociationStatusActive {
		return fmt.Errorf("%w: sn-service-association-id: %s, non-active status: %s",
			RetryErr, aws.StringValue(resp.Id), status)
	}
	return nil
}

func (m *defaultServiceManager) newCreateSvcReq(svc *Service) *CreateSvcReq {
	svcName := svc.LatticeName()
	req := &vpclattice.CreateServiceInput{
		Name: &svcName,
		Tags: m.cloud.NewTagsWithManagedBy(),
	}

	if svc.Spec.CustomerDomainName != "" {
		req.CustomDomainName = &svc.Spec.CustomerDomainName
	}
	if svc.Spec.CustomerCertARN != "" {
		req.SetCertificateArn(svc.Spec.CustomerCertARN)
	}

	return req
}

// compare current sn-svc associations with new ones,
// returns 2 slices: toCreate with SN names and toDelete with current associations
// if assoc should be created but current state is in deletion we should retry
func snSvcAssocsDiff(svc *Service, curAssocs []*SnSvcAssocSummary) ([]string, []SnSvcAssocSummary, error) {
	toCreate := []string{}
	toDelete := []SnSvcAssocSummary{}

	// create two Sets and find Difference New-Old->toCreate and Old-New->toDelete
	newSet := map[string]bool{}
	for _, sn := range svc.Spec.ServiceNetworkNames {
		newSet[sn] = true
	}
	oldSet := map[string]SnSvcAssocSummary{}
	for _, sn := range curAssocs {
		oldSet[*sn.ServiceNetworkName] = *sn
	}

	for newSn := range newSet {
		oldSn, ok := oldSet[newSn]
		if !ok {
			toCreate = append(toCreate, newSn)
		}

		// assoc should exists but in deletion state, will retry later to re-create
		// TODO: we should have something more lightweight, retrying full reconciliation looks to heavy
		if aws.StringValue(oldSn.Status) == vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress {
			return nil, nil, fmt.Errorf("%w: want to associate sn: %s to svc: %s, but status is: %s",
				RetryErr, newSn, svc.LatticeName(), *oldSn.Status)
		}
		// TODO: if assoc in failed state, may be we should try to re-create?
	}

	for oldSn, sn := range oldSet {
		_, ok := newSet[oldSn]
		if !ok {
			toDelete = append(toDelete, sn)
		}
	}

	return toCreate, toDelete, nil
}

func svcStatusFromCreateSvcResp(resp *CreateSvcResp) ServiceStatus {
	status := ServiceStatus{
		Arn: aws.StringValue(resp.Arn),
		Id:  aws.StringValue(resp.Id),
	}
	if resp.DnsEntry != nil {
		status.Dns = aws.StringValue(resp.DnsEntry.DomainName)
	}
	return status
}

// Create or update Service and ServiceNetwork-Service associations
func (m *defaultServiceManager) Create(ctx context.Context, svc *Service) (ServiceStatus, error) {
	emptyStatus := ServiceStatus{}

	svcSum, err := m.getService(ctx, svc.LatticeName())
	if err != nil {
		return emptyStatus, err
	}

	var status ServiceStatus
	if svcSum == nil {
		status, err = m.createServiceAndSnAssoc(ctx, svc)
	} else {
		status, err = m.updateServiceAndSnAssoc(ctx, svc, svcSum)
	}
	if err != nil {
		return emptyStatus, err
	}
	return status, nil
}

func (m *defaultServiceManager) Delete(ctx context.Context, svc *Service) error {
	svcSum, err := m.getService(ctx, svc.LatticeName())
	if err != nil {
		return err
	}
	if svcSum == nil {
		return nil
	}

	svc.Spec.ServiceNetworkNames = []string{}
	err = m.updateSnSvcAssocs(ctx, svc, svcSum)
	if err != nil {
		return err
	}

	delInput := vpclattice.DeleteServiceInput{
		ServiceIdentifier: svcSum.Id,
	}
	_, err = m.cloud.Lattice().DeleteServiceWithContext(ctx, &delInput)
	return err
}
