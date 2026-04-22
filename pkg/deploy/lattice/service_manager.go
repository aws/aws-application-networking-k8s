package lattice

import (
	"context"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"

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
type SvcSummary = types.ServiceSummary
type ListSnSvcAssocsReq = vpclattice.ListServiceNetworkServiceAssociationsInput
type SnSvcAssocSummary = types.ServiceNetworkServiceAssociationSummary

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
	createSvcResp, err := m.cloud.Lattice().CreateService(ctx, createSvcReq)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed CreateService %s due to %s", aws.ToString(createSvcReq.Name), err)
	}

	m.log.Infof(ctx, "Success CreateService %s %s",
		aws.ToString(createSvcResp.Name), aws.ToString(createSvcResp.Id))

	// Only create associations if service networks are specified (not standalone)
	if len(svc.Spec.ServiceNetworkNames) > 0 {
		for _, snName := range svc.Spec.ServiceNetworkNames {
			err = m.createAssociation(ctx, createSvcResp.Id, snName, svc)
			if err != nil {
				return ServiceInfo{}, err
			}
		}
	} else {
		m.log.Infof(ctx, "Skipping service network association for standalone service %s",
			aws.ToString(createSvcResp.Name))
	}

	svcInfo := svcStatusFromCreateSvcResp(createSvcResp)
	return svcInfo, nil
}

func (m *defaultServiceManager) createAssociation(ctx context.Context, svcId *string, snName string, svc *Service) error {
	snInfo, err := m.cloud.Lattice().FindServiceNetwork(ctx, snName)
	if err != nil {
		return err
	}

	tags := m.cloud.MergeTags(m.cloud.DefaultTags(), svc.Spec.AdditionalTags)

	assocReq := &CreateSnSvcAssocReq{
		ServiceIdentifier:        svcId,
		ServiceNetworkIdentifier: snInfo.SvcNetwork.Id,
		Tags:                     tags,
	}
	assocResp, err := m.cloud.Lattice().CreateServiceNetworkServiceAssociation(ctx, assocReq)
	if err != nil {
		return fmt.Errorf("failed CreateServiceNetworkServiceAssociation %s %s due to %s",
			aws.ToString(assocReq.ServiceNetworkIdentifier), aws.ToString(assocReq.ServiceIdentifier), err)
	}
	m.log.Infof(ctx, "Success CreateServiceNetworkServiceAssociation %s %s",
		aws.ToString(assocReq.ServiceNetworkIdentifier), aws.ToString(assocReq.ServiceIdentifier))

	err = handleCreateAssociationResp(assocResp)
	if err != nil {
		return err
	}
	return nil
}

func (m *defaultServiceManager) newCreateSvcReq(svc *Service) *CreateSvcReq {
	svcName := svc.LatticeServiceName()
	tags := m.cloud.MergeTags(m.cloud.DefaultTagsMergedWith(svc.Spec.ToTags()), svc.Spec.AdditionalTags)

	req := &vpclattice.CreateServiceInput{
		Name: &svcName,
		Tags: tags,
	}

	if svc.Spec.CustomerDomainName != "" {
		req.CustomDomainName = &svc.Spec.CustomerDomainName
	}
	if svc.Spec.CustomerCertARN != "" {
		req.CertificateArn = &svc.Spec.CustomerCertARN
	}

	return req
}

func svcStatusFromCreateSvcResp(resp *CreateSvcResp) ServiceInfo {
	svcInfo := ServiceInfo{}
	if resp == nil {
		return svcInfo
	}
	svcInfo.Arn = aws.ToString(resp.Arn)
	svcInfo.Id = aws.ToString(resp.Id)
	if resp.DnsEntry != nil {
		svcInfo.Dns = aws.ToString(resp.DnsEntry.DomainName)
	}
	return svcInfo
}

func (m *defaultServiceManager) checkAndUpdateTags(ctx context.Context, svc *Service, svcSum *SvcSummary) error {
	tagsResp, err := m.cloud.Lattice().ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
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
		if m.canTakeoverService(svc, tagsResp.Tags) {
			currentOwner := m.cloud.GetManagedByFromTags(tagsResp.Tags)
			newOwner := m.cloud.DefaultTags()[pkg_aws.TagManagedBy]
			err = m.transferServiceOwnership(ctx, svcSum.Arn, newOwner)
			if err != nil {
				return fmt.Errorf("failed to takeover service %s from %s to %s: %w", svc.LatticeServiceName(), currentOwner, newOwner, err)
			}
			m.log.Infof(ctx, "Successfully took over service %s from %s to %s", svc.LatticeServiceName(), currentOwner, newOwner)
			return nil
		} else {
			return services.NewConflictError("service", svc.Spec.RouteNamespace+"/"+svc.Spec.RouteName,
				fmt.Sprintf("Found existing resource not owned by controller: %s", *svcSum.Arn))
		}
	}
	tagFields := model.ServiceTagFieldsFromTags(tagsResp.Tags)
	switch {
	case tagFields.RouteName == "" && tagFields.RouteNamespace == "":
		// backwards compatibility: If the service has no identification tags, consider this controller has
		// correct information and add tags
		_, err = m.cloud.Lattice().TagResource(ctx, &vpclattice.TagResourceInput{
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
	err := m.cloud.Tagging().UpdateTags(ctx, aws.ToString(svcSum.Arn), svc.Spec.AdditionalTags, nil)
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("failed to update tags for service %s: %w", aws.ToString(svcSum.Id), err)
	}

	if svc.Spec.CustomerCertARN != "" {
		updReq := &UpdateSvcReq{
			CertificateArn:    aws.String(svc.Spec.CustomerCertARN),
			ServiceIdentifier: svcSum.Id,
		}
		_, err := m.cloud.Lattice().UpdateService(ctx, updReq)
		if err != nil {
			return ServiceInfo{}, err
		}
	}

	err = m.updateAssociations(ctx, svc, svcSum)
	if err != nil {
		return ServiceInfo{}, err
	}

	svcInfo := ServiceInfo{
		Arn: aws.ToString(svcSum.Arn),
		Id:  aws.ToString(svcSum.Id),
	}
	if svcSum.DnsEntry != nil {
		svcInfo.Dns = aws.ToString(svcSum.DnsEntry.DomainName)
	}
	return svcInfo, nil
}

func (m *defaultServiceManager) getAllAssociations(ctx context.Context, svcSum *SvcSummary) ([]SnSvcAssocSummary, error) {
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

	toCreate, toDelete, toUpdate, err := associationsDiff(svc, assocs)
	if err != nil {
		return err
	}
	for _, snName := range toCreate {
		err := m.createAssociation(ctx, svcSum.Id, snName, svc)
		if err != nil {
			return err
		}
	}

	var awsManagedTags services.Tags
	if svc.Spec.AllowTakeoverFrom != "" {
		awsManagedTags = services.Tags{
			pkg_aws.TagManagedBy: m.cloud.DefaultTags()[pkg_aws.TagManagedBy],
		}
	}

	for _, assoc := range toUpdate {
		err := m.cloud.Tagging().UpdateTags(ctx, aws.ToString(assoc.Arn), svc.Spec.AdditionalTags, awsManagedTags)
		if err != nil {
			return fmt.Errorf("failed to update tags for association %s: %w", aws.ToString(assoc.Arn), err)
		}
	}

	for _, assoc := range toDelete {
		isManaged, err := m.cloud.IsArnManaged(ctx, *assoc.Arn)
		if err != nil {
			// TODO check for types.AccessDeniedException or a new error type ErrorCodeNotFoundException
			// when the api no longer responds with a 404 NotFoundException instead of either of the above.
			// ErrorCodeNotFoundException currently not part of the golang sdk for the lattice api. This is a distinct
			// error from types.ResourceNotFoundException.

			// In a scenario that the service association is created by a foreign account,
			// the owner account's controller cannot read the tags of this ServiceNetworkServiceAssociation,
			// and AccessDeniedException is expected.
			m.log.Warnf(ctx, "skipping update associations  service: %s, association: %s, error: %s", svc.LatticeServiceName(), *assoc.Arn, err)

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

// returns RetryError on all non-active Sn-Svc association responses
func handleCreateAssociationResp(resp *CreateSnSvcAssocResp) error {
	status := string(resp.Status)
	if status != string(types.ServiceNetworkServiceAssociationStatusActive) {
		return fmt.Errorf("%w: sn-service-association-id: %s, non-active status: %s",
			lattice_runtime.NewRetryError(), aws.ToString(resp.Id), status)
	}
	return nil
}

// compare current sn-svc associations with new ones,
// returns 2 slices: toCreate with SN names and toDelete with current associations
// if assoc should be created but current state is in deletion we should retry
func associationsDiff(svc *Service, curAssocs []SnSvcAssocSummary) ([]string, []SnSvcAssocSummary, []SnSvcAssocSummary, error) {
	toCreate := []string{}
	toDelete := []SnSvcAssocSummary{}
	toUpdate := []SnSvcAssocSummary{}

	// create two Sets and find Difference New-Old->toCreate and Old-New->toDelete
	newSet := map[string]bool{}
	for _, sn := range svc.Spec.ServiceNetworkNames {
		newSet[sn] = true
	}
	oldSet := map[string]SnSvcAssocSummary{}
	for _, sn := range curAssocs {
		oldSet[aws.ToString(sn.ServiceNetworkName)] = sn
	}

	for newSn := range newSet {
		oldSn, ok := oldSet[newSn]
		if !ok {
			toCreate = append(toCreate, newSn)
		} else {
			toUpdate = append(toUpdate, oldSn)
		}

		// assoc should exists but in deletion state, will retry later to re-create
		// TODO: we should have something more lightweight, retrying full reconciliation looks to heavy
		if string(oldSn.Status) == string(types.ServiceNetworkServiceAssociationStatusDeleteInProgress) {
			return nil, nil, nil, fmt.Errorf("%w: want to associate sn: %s to svc: %s, but status is: %s",
				lattice_runtime.NewRetryError(), newSn, svc.LatticeServiceName(), string(oldSn.Status))
		}
		// TODO: if assoc in failed state, may be we should try to re-create?
	}

	for oldSn, sn := range oldSet {
		_, ok := newSet[oldSn]
		if !ok {
			toDelete = append(toDelete, sn)
		}
	}

	return toCreate, toDelete, toUpdate, nil
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
		_, err = m.cloud.Lattice().DeleteListener(ctx, &vpclattice.DeleteListenerInput{
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
	_, err := m.cloud.Lattice().DeleteServiceNetworkServiceAssociation(ctx, delReq)
	if err != nil {
		return fmt.Errorf("failed DeleteServiceNetworkServiceAssociation %s due to %s",
			aws.ToString(assocArn), err)
	}

	m.log.Infof(ctx, "Success DeleteServiceNetworkServiceAssociation %s", aws.ToString(assocArn))
	return nil
}

func (m *defaultServiceManager) deleteService(ctx context.Context, svc *SvcSummary) error {
	delInput := vpclattice.DeleteServiceInput{
		ServiceIdentifier: svc.Id,
	}
	_, err := m.cloud.Lattice().DeleteService(ctx, &delInput)
	if err != nil {
		return fmt.Errorf("failed DeleteService %s due to %s", aws.ToString(svc.Id), err)
	}

	m.log.Infof(ctx, "Success DeleteService %s", *svc.Id)
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
		m.log.Infof(ctx, "Service %s is either invalid or not owned. Skipping VPC Lattice resource deletion.", svc.LatticeServiceName())
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

func (m *defaultServiceManager) canTakeoverService(svc *Service, serviceTags services.Tags) bool {
	takeoverFrom := svc.Spec.AllowTakeoverFrom
	if takeoverFrom == "" {
		return false
	}

	currentOwner := m.cloud.GetManagedByFromTags(serviceTags)

	return currentOwner == takeoverFrom
}

func (m *defaultServiceManager) transferServiceOwnership(ctx context.Context, serviceArn *string, newOwner string) error {
	_, err := m.cloud.Lattice().TagResource(ctx, &vpclattice.TagResourceInput{
		ResourceArn: serviceArn,
		Tags: map[string]string{
			pkg_aws.TagManagedBy: newOwner,
		},
	})
	return err
}
