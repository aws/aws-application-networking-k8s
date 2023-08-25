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
type ServiceInfo = latticemodel.ServiceStatus
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

func (m *defaultServiceManager) createServiceAndAssociate(ctx context.Context, svc *Service) (ServiceInfo, error) {
	createSvcReq := m.newCreateSvcReq(svc)
	createSvcResp, err := m.cloud.Lattice().CreateServiceWithContext(ctx, createSvcReq)
	if err != nil {
		return ServiceInfo{}, err
	}

	for _, snName := range svc.Spec.ServiceNetworkNames {
		err = m.createAssociation(ctx, createSvcResp.Id, snName)
		if err != nil {
			return ServiceInfo{}, err
		}
	}
	svcInfo := svcStatusFromCreateSvcResp(createSvcResp)
	return svcInfo, nil
}

func (m *defaultServiceManager) createAssociation(ctx context.Context, svcId *string, snName string) error {
	sn, err := m.datastore.GetServiceNetworkStatus(snName, m.cloud.Config().AccountId)
	if err != nil {
		return err
	}
	assocReq := &CreateSnSvcAssocReq{
		ServiceIdentifier:        svcId,
		ServiceNetworkIdentifier: aws.String(sn.ID),
		Tags:                     m.cloud.DefaultTags(),
	}
	assocResp, err := m.cloud.Lattice().CreateServiceNetworkServiceAssociationWithContext(ctx, assocReq)
	if err != nil {
		return err
	}
	err = handleCreateAssociationResp(assocResp)
	if err != nil {
		return err
	}
	return nil
}

func (m *defaultServiceManager) newCreateSvcReq(svc *Service) *CreateSvcReq {
	svcName := svc.LatticeName()
	req := &vpclattice.CreateServiceInput{
		Name: &svcName,
		Tags: m.cloud.DefaultTags(),
	}

	if svc.Spec.CustomerDomainName != "" {
		req.CustomDomainName = &svc.Spec.CustomerDomainName
	}
	if svc.Spec.CustomerCertARN != "" {
		req.SetCertificateArn(svc.Spec.CustomerCertARN)
	}

	return req
}

func svcStatusFromCreateSvcResp(resp *CreateSvcResp) ServiceInfo {
	svcInfo := ServiceInfo{}
	if resp == nil {
		return svcInfo
	}
	svcInfo.Arn = aws.StringValue(resp.Arn)
	svcInfo.Id = aws.StringValue(resp.Id)
	if resp.DnsEntry != nil {
		svcInfo.Dns = aws.StringValue(resp.DnsEntry.DomainName)
	}
	return svcInfo
}

func (m *defaultServiceManager) updateServiceAndAssociations(ctx context.Context, svc *Service, svcSum *SvcSummary) (ServiceInfo, error) {
	if svc.Spec.CustomerCertARN != "" {
		updReq := &UpdateSvcReq{
			CertificateArn:    aws.String(svc.Spec.CustomerCertARN),
			ServiceIdentifier: svcSum.Id,
		}
		if updReq != nil {
			_, err := m.cloud.Lattice().UpdateService(updReq)
			if err != nil {
				return ServiceInfo{}, err
			}
		}
	}

	err := m.updateAssociations(ctx, svc, svcSum)
	if err != nil {
		return ServiceInfo{}, err
	}

	svcInfo := ServiceInfo{
		Arn: aws.StringValue(svcSum.Arn),
		Id:  aws.StringValue(svcSum.Id),
	}
	if svcSum.DnsEntry != nil {
		svcInfo.Dns = aws.StringValue(svcSum.DnsEntry.DomainName)
	}
	return svcInfo, nil
}

func (m *defaultServiceManager) getAllAssociations(ctx context.Context, svcSum *SvcSummary) ([]*SnSvcAssocSummary, error) {
	assocsReq := &ListSnSvcAssocsReq{
		ServiceIdentifier: svcSum.Id,
	}
	assocs, err := m.cloud.Lattice().ListServiceNetworkServiceAssociationsAsList(ctx, assocsReq)
	if err != nil {
		return nil, err
	}
	return assocs, err
}

// update SN-Svc associations, if svc has no SN associations will delete all of them
// does not delete associations that are not tagged by controller
func (m *defaultServiceManager) updateAssociations(ctx context.Context, svc *Service, svcSum *SvcSummary) error {
	assocs, err := m.getAllAssociations(ctx, svcSum)
	if err != nil {
		return err
	}

	toCreate, toDelete, err := associationsDiff(svc, assocs)
	if err != nil {
		return err
	}
	for _, snName := range toCreate {
		err := m.createAssociation(ctx, svcSum.Id, snName)
		if err != nil {
			return err
		}
	}

	for _, assoc := range toDelete {
		isManaged, err := m.cloud.IsArnManaged(*assoc.Arn)
		if err != nil {
			return err
		}
		if isManaged {
			err = m.deleteAssociation(ctx, assoc.Arn)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// returns RetryErr on all non-active Sn-Svc association responses
func handleCreateAssociationResp(resp *CreateSnSvcAssocResp) error {
	status := aws.StringValue(resp.Status)
	if status != vpclattice.ServiceNetworkServiceAssociationStatusActive {
		return fmt.Errorf("%w: sn-service-association-id: %s, non-active status: %s",
			RetryErr, aws.StringValue(resp.Id), status)
	}
	return nil
}

// compare current sn-svc associations with new ones,
// returns 2 slices: toCreate with SN names and toDelete with current associations
// if assoc should be created but current state is in deletion we should retry
func associationsDiff(svc *Service, curAssocs []*SnSvcAssocSummary) ([]string, []SnSvcAssocSummary, error) {
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

func (m *defaultServiceManager) deleteAllAssociations(ctx context.Context, svc *SvcSummary) error {
	assocs, err := m.getAllAssociations(ctx, svc)
	if err != nil {
		return err
	}
	for _, assoc := range assocs {
		err = m.deleteAssociation(ctx, assoc.Arn)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *defaultServiceManager) deleteAssociation(ctx context.Context, assocArn *string) error {
	delReq := &DelSnSvcAssocReq{ServiceNetworkServiceAssociationIdentifier: assocArn}
	_, err := m.cloud.Lattice().DeleteServiceNetworkServiceAssociationWithContext(ctx, delReq)
	if err != nil {
		return err
	}
	return nil
}

func (m *defaultServiceManager) deleteService(ctx context.Context, svc *SvcSummary) error {
	delInput := vpclattice.DeleteServiceInput{
		ServiceIdentifier: svc.Id,
	}
	_, err := m.cloud.Lattice().DeleteServiceWithContext(ctx, &delInput)
	return err
}

// Create or update Service and ServiceNetwork-Service associations
func (m *defaultServiceManager) Create(ctx context.Context, svc *Service) (ServiceInfo, error) {
	svcSum, err := m.getService(ctx, svc.LatticeName())
	if err != nil {
		return ServiceInfo{}, err
	}

	var svcInfo ServiceInfo
	if svcSum == nil {
		svcInfo, err = m.createServiceAndAssociate(ctx, svc)
	} else {
		svcInfo, err = m.updateServiceAndAssociations(ctx, svc, svcSum)
	}
	if err != nil {
		return ServiceInfo{}, err
	}
	return svcInfo, nil
}

func (m *defaultServiceManager) Delete(ctx context.Context, svc *Service) error {
	svcSum, err := m.getService(ctx, svc.LatticeName())
	if err != nil {
		return err
	}
	if svcSum == nil {
		return nil // already deleted
	}

	err = m.deleteAllAssociations(ctx, svcSum)
	if err != nil {
		return err
	}

	err = m.deleteService(ctx, svcSum)
	if err != nil {
		return err
	}
	return nil
}
