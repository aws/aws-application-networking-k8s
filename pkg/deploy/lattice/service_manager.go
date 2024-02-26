package lattice

import (
	"context"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

//go:generate mockgen -destination service_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice ServiceManager

type Service = model.Service
type ServiceInfo = model.ServiceStatus
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
	Upsert(ctx context.Context, service *model.Service) (model.ServiceStatus, error)
	Delete(ctx context.Context, service *model.Service) error
}

type defaultServiceManager struct {
	log   gwlog.Logger
	cloud pkg_aws.Cloud
}

func NewServiceManager(log gwlog.Logger, cloud pkg_aws.Cloud) *defaultServiceManager {
	return &defaultServiceManager{
		log:   log,
		cloud: cloud,
	}
}

func (m *defaultServiceManager) createServiceAndAssociate(ctx context.Context, svc *Service) (ServiceInfo, error) {
	createSvcReq := m.newCreateSvcReq(svc)
	createSvcResp, err := m.cloud.Lattice().CreateServiceWithContext(ctx, createSvcReq)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed CreateService %s due to %s", aws.StringValue(createSvcReq.Name), err)
	}

	m.log.Infof("Success CreateService %s %s",
		aws.StringValue(createSvcResp.Name), aws.StringValue(createSvcResp.Id))

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
	snInfo, err := m.cloud.Lattice().FindServiceNetwork(ctx, snName)
	if err != nil {
		return err
	}

	assocReq := &CreateSnSvcAssocReq{
		ServiceIdentifier:        svcId,
		ServiceNetworkIdentifier: snInfo.SvcNetwork.Id,
		Tags:                     m.cloud.DefaultTags(),
	}
	assocResp, err := m.cloud.Lattice().CreateServiceNetworkServiceAssociationWithContext(ctx, assocReq)
	if err != nil {
		return fmt.Errorf("failed CreateServiceNetworkServiceAssociation %s %s due to %s",
			aws.StringValue(assocReq.ServiceNetworkIdentifier), aws.StringValue(assocReq.ServiceIdentifier), err)
	}
	m.log.Infof("Success CreateServiceNetworkServiceAssociation %s %s",
		aws.StringValue(assocReq.ServiceNetworkIdentifier), aws.StringValue(assocReq.ServiceIdentifier))

	err = handleCreateAssociationResp(assocResp)
	if err != nil {
		return err
	}
	return nil
}

func (m *defaultServiceManager) newCreateSvcReq(svc *Service) *CreateSvcReq {
	svcName := svc.LatticeServiceName()
	req := &vpclattice.CreateServiceInput{
		Name: &svcName,
		Tags: m.cloud.DefaultTagsMergedWith(svc.Spec.ToTags()),
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

func (m *defaultServiceManager) checkAndUpdateTags(ctx context.Context, svc *Service, svcSum *SvcSummary) error {
	tagsResp, err := m.cloud.Lattice().ListTagsForResourceWithContext(ctx, &vpclattice.ListTagsForResourceInput{
		ResourceArn: svcSum.Arn,
	})
	if err != nil {
		return err
	}

	owned, err := m.cloud.TryOwnFromTags(ctx, *svcSum.Arn, tagsResp.Tags)
	if err != nil {
		return err
	}
	if !owned {
		return services.NewConflictError("service", svc.Spec.RouteNamespace+"/"+svc.Spec.RouteName,
			fmt.Sprintf("Found existing resource not owned by controller: %s", *svcSum.Arn))
	}

	tagFields := model.ServiceTagFieldsFromTags(tagsResp.Tags)
	switch {
	case tagFields.RouteName == "" && tagFields.RouteNamespace == "":
		// backwards compatibility: If the service has no identification tags, consider this controller has
		// correct information and add tags
		_, err = m.cloud.Lattice().TagResourceWithContext(ctx, &vpclattice.TagResourceInput{
			ResourceArn: svcSum.Arn,
			Tags:        svc.Spec.ToTags(),
		})
		return err
	case tagFields != svc.Spec.ServiceTagFields:
		// Considering these scenarios:
		// - two services with same namespace-name but different routeType
		// - two services with conflict edge case such as my-namespace/service & my/namespace-service
		return services.NewConflictError("service", svc.Spec.RouteName+"/"+svc.Spec.RouteNamespace,
			fmt.Sprintf("Found existing resource with conflicting service name: %s", *svcSum.Arn))
	}
	return nil
}

func (m *defaultServiceManager) updateServiceAndAssociations(ctx context.Context, svc *Service, svcSum *SvcSummary) (ServiceInfo, error) {
	if svc.Spec.CustomerCertARN != "" {
		updReq := &UpdateSvcReq{
			CertificateArn:    aws.String(svc.Spec.CustomerCertARN),
			ServiceIdentifier: svcSum.Id,
		}
		_, err := m.cloud.Lattice().UpdateService(updReq)
		if err != nil {
			return ServiceInfo{}, err
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
		isManaged, err := m.cloud.IsArnManaged(ctx, *assoc.Arn)
		if err != nil {
			// TODO check for vpclattice.ErrCodeAccessDeniedException or a new error type ErrorCodeNotFoundException
			// when the api no longer responds with a 404 NotFoundException instead of either of the above.
			// ErrorCodeNotFoundException currently not part of the golang sdk for the lattice api. This a is a distinct
			// error from vpclattice.ErrCodeResourceNotFoundException.

			// In a scenario that the service association is created by a foreign account,
			// the owner account's controller cannot read the tags of this ServiceNetworkServiceAssociation,
			// and AccessDeniedException is expected.
			m.log.Warnf("skipping update associations  service: %s, association: %s, error: %s", svc.LatticeServiceName(), *assoc.Arn, err)

			continue
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
				RetryErr, newSn, svc.LatticeServiceName(), *oldSn.Status)
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

func (m *defaultServiceManager) deleteAllListeners(ctx context.Context, svc *SvcSummary) error {
	listeners, err := m.cloud.Lattice().ListListenersAsList(ctx, &vpclattice.ListListenersInput{
		ServiceIdentifier: svc.Id,
	})
	if err != nil {
		return err
	}
	for _, listener := range listeners {
		_, err = m.cloud.Lattice().DeleteListenerWithContext(ctx, &vpclattice.DeleteListenerInput{
			ServiceIdentifier:  svc.Id,
			ListenerIdentifier: listener.Id,
		})
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
		return fmt.Errorf("failed DeleteServiceNetworkServiceAssociation %s due to %s",
			aws.StringValue(assocArn), err)
	}

	m.log.Infof("Success DeleteServiceNetworkServiceAssociation %s", aws.StringValue(assocArn))
	return nil
}

func (m *defaultServiceManager) deleteService(ctx context.Context, svc *SvcSummary) error {
	delInput := vpclattice.DeleteServiceInput{
		ServiceIdentifier: svc.Id,
	}
	_, err := m.cloud.Lattice().DeleteServiceWithContext(ctx, &delInput)
	if err != nil {
		return fmt.Errorf("failed DeleteService %s due to %s", aws.StringValue(svc.Id), err)
	}

	m.log.Infof("Success DeleteService %s", svc.Id)
	return nil
}

// Create or update Service and ServiceNetwork-Service associations
func (m *defaultServiceManager) Upsert(ctx context.Context, svc *Service) (ServiceInfo, error) {
	svcSum, err := m.cloud.Lattice().FindService(ctx, svc.LatticeServiceName())
	if err != nil && !services.IsNotFoundError(err) {
		return ServiceInfo{}, err
	}

	var svcInfo ServiceInfo
	if svcSum == nil {
		svcInfo, err = m.createServiceAndAssociate(ctx, svc)
	} else {
		err = m.checkAndUpdateTags(ctx, svc, svcSum)
		if err != nil {
			return ServiceInfo{}, err
		}
		svcInfo, err = m.updateServiceAndAssociations(ctx, svc, svcSum)
	}
	if err != nil {
		return ServiceInfo{}, err
	}
	return svcInfo, nil
}

func (m *defaultServiceManager) Delete(ctx context.Context, svc *Service) error {
	svcSum, err := m.cloud.Lattice().FindService(ctx, svc.LatticeServiceName())
	if err != nil {
		if services.IsNotFoundError(err) {
			return nil // already deleted
		} else {
			return err
		}
	}

	err = m.checkAndUpdateTags(ctx, svc, svcSum)
	if err != nil {
		m.log.Infof("Service %s is either invalid or not owned. Skipping VPC Lattice resource deletion.", svc.LatticeServiceName())
		return nil
	}

	err = m.deleteAllAssociations(ctx, svcSum)
	if err != nil {
		return err
	}

	// deleting listeners explicitly helps ensure target groups are free to delete
	err = m.deleteAllListeners(ctx, svcSum)
	if err != nil {
		return err
	}

	err = m.deleteService(ctx, svcSum)
	if err != nil {
		return err
	}
	return nil
}
