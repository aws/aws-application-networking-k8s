package lattice

import (
	"context"
	"errors"
	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/mercury"

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

	// get serviceNetwork info
	serviceNetwork, err := s.latticeDataStore.GetServiceNetworkStatus(service.Spec.ServiceNetworkName, config.AccountID)
	if err != nil {
		glog.V(6).Infof("defaultServiceManager: fail to get serviceNetwork status for service %v\n", service)
		return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, err
	}

	// check if exists
	svcName := latticestore.AWSServiceName(service.Spec.Name, service.Spec.Namespace)
	serviceSummary, err := s.findServiceByName(ctx, svcName)
	if err != nil {
		return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, err
	}
	var serviceID string
	var serviceArn string
	var serviceDNS string
	var isServiceAssociatedWithServiceNetwork bool
	latticeSess := s.cloud.Mercury()

	// create service
	if serviceSummary == nil {
		glog.V(6).Infof("lattice service create API here service [%v]\n", service)
		serviceInput := mercury.CreateServiceInput{
			Name: &svcName,
			Tags: nil,
		}
		latticeSess := s.cloud.Mercury()
		resp, err := latticeSess.CreateServiceWithContext(ctx, &serviceInput)
		glog.V(2).Infof("CreateServiceWithContext >>>> req %v resp %v err %v\n", serviceInput, resp, err)
		if err != nil {
			glog.V(6).Infoln("fail to create service")
			return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, err
		}
		serviceID = aws.StringValue(resp.Id)
		serviceArn = aws.StringValue(resp.Arn)
		isServiceAssociatedWithServiceNetwork = false
	} else {
		serviceID = aws.StringValue(serviceSummary.Id)
		serviceArn = aws.StringValue(serviceSummary.Arn)
		if serviceSummary.DnsEntry != nil {
			serviceDNS = aws.StringValue(serviceSummary.DnsEntry.DomainName)
		}
		isServiceAssociatedWithServiceNetwork, serviceDNS, err = s.isServiceAssociatedWithServiceNetwork(ctx, serviceID, serviceNetwork.ID)
		if err != nil {
			return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, err
		}
	}

	// associate service with serviceNetwork
	if isServiceAssociatedWithServiceNetwork == false {
		createServiceNetworkAssociationInput := mercury.CreateMeshServiceAssociationInput{
			MeshIdentifier:    &serviceNetwork.ID,
			ServiceIdentifier: &serviceID,
		}
		resp, err := latticeSess.CreateMeshServiceAssociationWithContext(ctx, &createServiceNetworkAssociationInput)
		glog.V(6).Infof("create-associate  for service %v, in serviceNetwork %v, with resp %v err %v \n",
			service, serviceNetwork, resp, err)
		if err != nil {
			glog.V(6).Infoln("fail to associate serviceNetwork and service")
			return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, err
		} else {
			associationStatus := aws.StringValue(resp.Status)
			respDNS := ""
			if resp.DnsEntry != nil {
				respDNS = aws.StringValue(resp.DnsEntry.DomainName)

			}
			switch associationStatus {
			case mercury.MeshServiceAssociationStatusCreateInProgress:
				return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, errors.New(LATTICE_RETRY)
			case mercury.MeshServiceAssociationStatusActive:
				return latticemodel.ServiceStatus{ServiceARN: serviceArn, ServiceID: serviceID, ServiceDNS: respDNS}, nil
			case mercury.MeshServiceAssociationStatusDeleteFailed:
				return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, errors.New(LATTICE_RETRY)
			case mercury.MeshServiceAssociationStatusCreateFailed:
				return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, errors.New(LATTICE_RETRY)
			case mercury.MeshServiceAssociationStatusDeleteInProgress:
				return latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""}, errors.New(LATTICE_RETRY)
			}
		}
	}
	return latticemodel.ServiceStatus{ServiceARN: serviceArn, ServiceID: serviceID, ServiceDNS: serviceDNS}, nil
}

// TODO: Further refactor for all manager under lattice/
func (s *defaultServiceManager) isServiceNetworkServiceAssociationStatusAssociated(associationStatus string) bool {
	switch associationStatus {
	case mercury.MeshServiceAssociationStatusActive:
		return true
	case mercury.MeshServiceAssociationStatusCreateInProgress:
		return false
	case mercury.MeshServiceAssociationStatusDeleteFailed:
		return false
	case mercury.MeshServiceAssociationStatusCreateFailed:
		return false
	case mercury.MeshServiceAssociationStatusDeleteInProgress:
		return false
	default:
		return false
	}
}

// find service by name return serviceNetwork,err if mesh exists, otherwise return nil,nil
func (s *defaultServiceManager) findServiceByName(ctx context.Context, serviceName string) (*mercury.ServiceSummary, error) {
	latticeSess := s.cloud.Mercury()
	servicesListInput := mercury.ListServicesInput{}
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
	latticeSess := s.cloud.Mercury()
	listServiceNetworkServiceAssociationsInput := mercury.ListMeshServiceAssociationsInput{
		MeshIdentifier:    &serviceNetworkID,
		ServiceIdentifier: &serviceID,
	}
	resp, err := latticeSess.ListMeshServiceAssociationsAsList(ctx, &listServiceNetworkServiceAssociationsInput)
	glog.V(6).Infof("ListMeshServiceAssociationsAsList, resp %v, err %v\n", resp, err)
	dnsName := ""
	if err != nil || (len(resp) == 0) {
		// return nil, let caller retry to associate VPC
		return false, dnsName, err
	} else {
		associationStatus := aws.StringValue(resp[0].Status)
		switch associationStatus {
		case mercury.MeshServiceAssociationStatusActive:
			if resp[0].DnsEntry != nil {
				dnsName = *resp[0].DnsEntry.DomainName
			}
			return true, dnsName, nil
		case mercury.MeshServiceAssociationStatusCreateFailed:
			return false, "", nil
		case mercury.MeshServiceAssociationStatusDeleteFailed:
			return false, "", nil
		case mercury.MeshServiceAssociationStatusDeleteInProgress:
			return false, "", errors.New(LATTICE_RETRY)
		case mercury.MeshServiceAssociationStatusCreateInProgress:
			return false, "", errors.New(LATTICE_RETRY)
		}
	}
	return false, "", nil
}

func (s *defaultServiceManager) Delete(ctx context.Context, service *latticemodel.Service) error {

	latticeSess := s.cloud.Mercury()

	svcName := latticestore.AWSServiceName(service.Spec.Name, service.Spec.Namespace)
	serviceSummary, err := s.findServiceByName(ctx, svcName)
	if err != nil || serviceSummary == nil {
		glog.V(6).Infof("defaultServiceManager: Deleting unknown service %v\n", service.Spec.Name)
		return nil
	}

	// find out serviceNetworkID
	serviceNetwork, err := s.latticeDataStore.GetServiceNetworkStatus(service.Spec.ServiceNetworkName, config.AccountID)
	if err != nil {
		glog.V(6).Infof("defaultServiceManager: fail to get serviceNetwork status for service %v\n", service)
		return err
	}

	listServiceNetworkInput := mercury.ListMeshServiceAssociationsInput{
		MeshIdentifier:    &serviceNetwork.ID,
		ServiceIdentifier: serviceSummary.Id,
	}

	listServiceNetworkOutput, err := latticeSess.ListMeshServiceAssociationsAsList(ctx, &listServiceNetworkInput)

	glog.V(6).Infof("defaultServiceManager - ListServiceNetworkServiceAssociations input %v output %v err %v \n", listServiceNetworkInput, listServiceNetworkOutput, err)

	if err == nil && (len(listServiceNetworkOutput) != 0) {
		svcServiceNetworkInput := mercury.DeleteMeshServiceAssociationInput{
			MeshServiceAssociationIdentifier: listServiceNetworkOutput[0].Id,
		}

		svcServiceNetworkOutput, err := latticeSess.DeleteMeshServiceAssociationWithContext(ctx, &svcServiceNetworkInput)

		glog.V(6).Infof("defaultServiceManager-DeleteServiceNetworkServiceAssociation: input %v output %v err %v \n", svcServiceNetworkInput, svcServiceNetworkOutput, err)
	}

	// delete service
	delInput := mercury.DeleteServiceInput{
		ServiceIdentifier: serviceSummary.Id,
	}
	delResp, err := latticeSess.DeleteServiceWithContext(ctx, &delInput)

	glog.V(2).Infof("DeleteServiceWithContext >>>> req %v resp %v, err %v\n", delInput, delResp, err)

	return err
}
