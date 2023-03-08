package lattice

import (
	"context"
	"errors"
	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type ServiceManager interface {
	Create(ctx context.Context, service *latticemodel.Service) (latticemodel.ServiceStatus, error)
	Delete(ctx context.Context, service *latticemodel.Service) error
}

type defaultServiceManager struct {
	cloud            lattice_aws.Cloud
	latticeDataStore *latticestore.LatticeDataStore
}

func NewServiceManager(cloud lattice_aws.Cloud, latticeDataStore *latticestore.LatticeDataStore) *defaultServiceManager {
	return &defaultServiceManager{
		cloud:            cloud,
		latticeDataStore: latticeDataStore,
	}
}

// Create will try to create a service and associate the serviceNetwork and service
// return error when:
//
//	ListServicesAsList() returns error
//	CreateServiceWithContext returns error
//
// return nil when:
//
//	Service get created and associated with serviceNetwork
//
// return errors.New(LATTICE_RETRY) when:
//
//	CreateMeshServiceAssociationWithContext returns
//		MeshServiceAssociationStatusCreateInProgress
//		MeshServiceAssociationStatusDeleteFailed
//		MeshServiceAssociationStatusCreateFailed
//		MeshServiceAssociationStatusDeleteInProgress
func (s *defaultServiceManager) Create(ctx context.Context, service *latticemodel.Service) (latticemodel.ServiceStatus, error) {

	// check if exists
	svcName := latticestore.AWSServiceName(service.Spec.Name, service.Spec.Namespace)
	serviceSummary, err := s.findServiceByName(ctx, svcName)
	if err != nil {
		return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, err
	}
	var serviceID string
	var serviceArn string
	var serviceDNS string

	// create service
	if serviceSummary == nil {
		glog.V(6).Infof("lattice service create API here service [%v]\n", service)

		serviceInput := vpclattice.CreateServiceInput{
			Name: &svcName,
			Tags: make(map[string]*string),
		}

		if len(service.Spec.CustomerDomainName) > 0 {
			serviceInput.CustomDomainName = &service.Spec.CustomerDomainName
		}
		serviceInput.Tags[latticemodel.K8SServiceOwnedByVPC] = &config.VpcID

		if len(service.Spec.CustomerCertARN) > 0 {
			serviceInput.SetCertificateArn(service.Spec.CustomerCertARN)
		}
		latticeSess := s.cloud.Lattice()
		resp, err := latticeSess.CreateServiceWithContext(ctx, &serviceInput)
		glog.V(2).Infof("CreateServiceWithContext >>>> req %v resp %v err %v\n", serviceInput, resp, err)
		if err != nil {
			glog.V(6).Infoln("fail to create service")
			return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, err
		}
		serviceID = aws.StringValue(resp.Id)
		serviceArn = aws.StringValue(resp.Arn)
	} else {
		serviceID = aws.StringValue(serviceSummary.Id)
		serviceArn = aws.StringValue(serviceSummary.Arn)
		if serviceSummary.DnsEntry != nil {
			serviceDNS = aws.StringValue(serviceSummary.DnsEntry.DomainName)
		}

		if len(service.Spec.CustomerCertARN) > 0 {
			serviceUpdateInput := vpclattice.UpdateServiceInput{
				ServiceIdentifier: serviceSummary.Id,
				CertificateArn:    aws.String(service.Spec.CustomerCertARN),
			}

			latticeSess := s.cloud.Lattice()
			resp, err := latticeSess.UpdateServiceWithContext(ctx, &serviceUpdateInput)
			glog.V(2).Infof("UpdateServiceWithContext >>>> req %v resp %v err %v\n", serviceUpdateInput, resp, err)
			if err != nil {
				glog.V(6).Infoln("fail to update service")
				return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, err
			}

		}

	}
	err = s.serviceNetworkAssociationMgr(ctx, service.Spec.ServiceNetworkNames, serviceID)

	if err != nil {
		return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, err
	}

	return latticemodel.ServiceStatus{ServiceARN: serviceArn, ServiceID: serviceID, ServiceDNS: serviceDNS}, nil
}

// TODO: Further refactor for all manager under lattice/
func (s *defaultServiceManager) isServiceNetworkServiceAssociationStatusAssociated(associationStatus string) bool {
	switch associationStatus {
	case vpclattice.ServiceNetworkServiceAssociationStatusActive:
		return true
	case vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress:
		return false
	case vpclattice.ServiceNetworkServiceAssociationStatusDeleteFailed:
		return false
	case vpclattice.ServiceNetworkServiceAssociationStatusCreateFailed:
		return false
	case vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress:
		return false
	default:
		return false
	}
}

// find service by name return serviceNetwork,err if mesh exists, otherwise return nil,nil
func (s *defaultServiceManager) findServiceByName(ctx context.Context, serviceName string) (*vpclattice.ServiceSummary, error) {
	latticeSess := s.cloud.Lattice()
	servicesListInput := vpclattice.ListServicesInput{}
	resp, err := latticeSess.ListServicesAsList(ctx, &servicesListInput)
	glog.V(6).Infof("findServiceByName, resp %v, err: %v\n", resp, err)

	if err == nil {
		for _, r := range resp {
			if aws.StringValue(r.Name) == serviceName {
				glog.V(6).Info("service ", serviceName, " already exists with arn ", *r.Arn, "\n")
				return r, err
			}
		}
	} else {
		return nil, err
	}
	return nil, nil
}

func (s *defaultServiceManager) isServiceAssociatedWithServiceNetwork(ctx context.Context, serviceID string, serviceNetworkID string) (bool, string, error) {
	latticeSess := s.cloud.Lattice()
	listServiceNetworkServiceAssociationsInput := vpclattice.ListServiceNetworkServiceAssociationsInput{
		ServiceNetworkIdentifier: &serviceNetworkID,
		ServiceIdentifier:        &serviceID,
	}
	resp, err := latticeSess.ListServiceNetworkServiceAssociationsAsList(ctx, &listServiceNetworkServiceAssociationsInput)
	glog.V(6).Infof("ListServiceNetworkServiceAssociationsAsList, resp %v, err %v\n", resp, err)
	dnsName := ""
	if err != nil || (len(resp) == 0) {
		// return nil, let caller retry to associate VPC
		return false, dnsName, err
	} else {
		associationStatus := aws.StringValue(resp[0].Status)
		switch associationStatus {
		case vpclattice.ServiceNetworkServiceAssociationStatusActive:
			if resp[0].DnsEntry != nil {
				dnsName = *resp[0].DnsEntry.DomainName
			}
			return true, dnsName, nil
		case vpclattice.ServiceNetworkServiceAssociationStatusCreateFailed:
			return false, "", nil
		case vpclattice.ServiceNetworkServiceAssociationStatusDeleteFailed:
			return false, "", nil
		case vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress:
			return false, "", errors.New(LATTICE_RETRY)
		case vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress:
			return false, "", errors.New(LATTICE_RETRY)
		}
	}
	return false, "", nil
}

func (s *defaultServiceManager) Delete(ctx context.Context, service *latticemodel.Service) error {

	latticeSess := s.cloud.Lattice()

	svcName := latticestore.AWSServiceName(service.Spec.Name, service.Spec.Namespace)
	serviceSummary, err := s.findServiceByName(ctx, svcName)
	if err != nil || serviceSummary == nil {
		glog.V(6).Infof("defaultServiceManager: Deleting unknown service %v\n", service.Spec.Name)
		return nil
	}

	// disassociate service from ALL service network(s) first
	err = s.serviceNetworkAssociationMgr(ctx, []string{}, *serviceSummary.Id)

	if err != nil {
		glog.V(6).Infof("Disassociation is not done yet for service %v\n", service)
		return err
	}

	// delete service
	delInput := vpclattice.DeleteServiceInput{
		ServiceIdentifier: serviceSummary.Id,
	}
	delResp, err := latticeSess.DeleteServiceWithContext(ctx, &delInput)

	glog.V(2).Infof("DeleteServiceWithContext >>>> req %v resp %v, err %v\n", delInput, delResp, err)

	return err
}

func (s *defaultServiceManager) serviceNetworkAssociationMgr(ctx context.Context, snNames []string, svcID string) error {
	glog.V(2).Infof("Desire to associate svc %v to  service network names %v", svcID, snNames)
	latticeSess := s.cloud.Lattice()

	// go through desired SN list
	// check if SN is in association list,
	// if NOT, create svc-> SN association
	for _, snName := range snNames {
		serviceNetwork, err := s.latticeDataStore.GetServiceNetworkStatus(snName, config.AccountID)
		if err != nil {
			glog.V(2).Infof("Unable find service network[%v] in cache to associate sservice %v to",
				snName, svcID)
			return err
		}

		isServiceAssociatedWithServiceNetwork, _, err := s.isServiceAssociatedWithServiceNetwork(ctx, svcID, serviceNetwork.ID)

		if isServiceAssociatedWithServiceNetwork == false {
			createServiceNetworkAssociationInput := vpclattice.CreateServiceNetworkServiceAssociationInput{
				ServiceNetworkIdentifier: &serviceNetwork.ID,
				ServiceIdentifier:        &svcID,
			}
			resp, err := latticeSess.CreateServiceNetworkServiceAssociationWithContext(ctx, &createServiceNetworkAssociationInput)
			glog.V(2).Infof("Associate service %v to serviceNetwork %v, got resp %v err %v ",
				svcID, serviceNetwork, resp, err)
			if err != nil {
				glog.V(6).Infoln("fail to associate serviceNetwork and service")
				return err
			} else {
				associationStatus := aws.StringValue(resp.Status)

				switch associationStatus {
				case vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress:
					return errors.New(LATTICE_RETRY)
				case vpclattice.ServiceNetworkServiceAssociationStatusActive:
					continue
				case vpclattice.ServiceNetworkServiceAssociationStatusDeleteFailed:
					return errors.New(LATTICE_RETRY)
				case vpclattice.ServiceNetworkServiceAssociationStatusCreateFailed:
					return errors.New(LATTICE_RETRY)
				case vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress:
					return errors.New(LATTICE_RETRY)
				}
			}
		}
	}

	// go through SVC's association list
	// check SN is in desired SN list
	// if NOT, delete svc->SN association
	// TODO logic here
	listServiceNetworkServiceAssociationsInput := vpclattice.ListServiceNetworkServiceAssociationsInput{
		ServiceIdentifier: &svcID,
	}
	resp, err := latticeSess.ListServiceNetworkServiceAssociationsAsList(ctx, &listServiceNetworkServiceAssociationsInput)

	glog.V(2).Infof("ListServiceNetworkServiceAssociationsAsList req %v, resp %v err %v",
		listServiceNetworkServiceAssociationsInput, resp, err)

	for _, snAssocResp := range resp {
		// go through desired SN list
		needDelete := true

		for _, snName := range snNames {
			if snName == aws.StringValue(snAssocResp.ServiceNetworkName) {
				// snName is in the desired SN association list
				needDelete = false
			}

		}

		if needDelete {
			svcServiceNetworkInput := vpclattice.DeleteServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: snAssocResp.Id,
			}

			svcServiceNetworkOutput, err := latticeSess.DeleteServiceNetworkServiceAssociationWithContext(ctx, &svcServiceNetworkInput)

			glog.V(2).Infof("Disassociate service %v from service network %v, resp %v, err %v",
				snAssocResp.ServiceName, snAssocResp.ServiceNetworkName, svcServiceNetworkOutput, err)

			if err != nil {
				return err
			}
			// also check the status, disassociation is still in progress, retry later
			if aws.StringValue(svcServiceNetworkOutput.Status) == vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress {
				glog.V(6).Infof("Disassociate-in-progress will retry later service %v from service network %v",
					snAssocResp.ServiceName, snAssocResp.ServiceNetworkName)
				return errors.New(LATTICE_RETRY)
			}

		}

	}
	return nil
}
