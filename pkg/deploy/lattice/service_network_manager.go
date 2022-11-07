package lattice

import (
	"context"
	"errors"
	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/mercury"

	mercury_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type ServiceNetworkManager interface {
	Create(ctx context.Context, service_network *latticemodel.ServiceNetwork) (latticemodel.ServiceNetworkStatus, error)
	List(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, service_network string) error
}

func NewDefaultServiceNetworkManager(cloud mercury_aws.Cloud) *defaultServiceNetworkManager {
	return &defaultServiceNetworkManager{
		cloud: cloud,
	}
}

var _service_networkManager = &defaultServiceNetworkManager{}

type defaultServiceNetworkManager struct {
	cloud mercury_aws.Cloud
}

// Create will try to create a service_network and associate the service_network with vpc
// return error when:
//
//	ListServiceNetworkesWithContext returns error
//	CreateServiceNetworkWithContext returns error
//	CreateServiceNetworkVpcAssociationInput returns error
//
// return nil when:
//
//	ServiceNetwork get created and associated with vpc
//
// return errors.New(LATTICE_RETRY) when:
//
//	CreateServiceNetworkVpcAssociationInput returns ServiceNetworkVpcAssociationStatusFailed/ServiceNetworkVpcAssociationStatusCreateInProgress/MeshVpcAssociationStatusDeleteInProgress
func (m *defaultServiceNetworkManager) Create(ctx context.Context, service_network *latticemodel.ServiceNetwork) (latticemodel.ServiceNetworkStatus, error) {
	// check if exists
	service_networkSummary, err := m.findServiceNetworkByName(ctx, service_network.Spec.Name)
	if err != nil {
		return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
	}

	// pre declaration
	var service_networkID string
	var service_networkArn string
	var isServiceNetworkAssociatedWithVPC bool
	mercurySess := m.cloud.Mercury()
	if service_networkSummary == nil {
		glog.V(2).Infof("ServiceNetwork Create API here, service_network[%v] vpciID[%s]\n", service_network, config.VpcID)
		service_networkInput := mercury.CreateMeshInput{
			Name: &service_network.Spec.Name,
		}
		resp, err := mercurySess.CreateMeshWithContext(ctx, &service_networkInput)
		if err != nil {
			glog.V(6).Infoln("Failed to create service_network, err: ", err)
			return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
		}
		service_networkID = aws.StringValue(resp.Id)
		service_networkArn = aws.StringValue(resp.Arn)
		isServiceNetworkAssociatedWithVPC = false
		glog.V(2).Infof(" ServiceNetwork Create API resp [%v]\n", resp)
	} else {
		glog.V(6).Infoln("service_network exists, further check association")
		service_networkID = aws.StringValue(service_networkSummary.Id)
		service_networkArn = aws.StringValue(service_networkSummary.Arn)
		isServiceNetworkAssociatedWithVPC, _, _, err = m.isServiceNetworkAssociatedWithVPC(ctx, service_networkID)
		if err != nil {
			return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
		}
	}

	if isServiceNetworkAssociatedWithVPC == false {
		createServiceNetworkVpcAssociationInput := mercury.CreateMeshVpcAssociationInput{
			MeshIdentifier: &service_networkID,
			VpcIdentifier:  &config.VpcID,
		}
		glog.V(2).Infof("Create service_network/vpc association >>>> req[%v]\n", createServiceNetworkVpcAssociationInput)
		resp, err := mercurySess.CreateMeshVpcAssociationWithContext(ctx, &createServiceNetworkVpcAssociationInput)
		glog.V(2).Infof("Create service_network and vpc association here >>>> resp[%v] err [%v]\n", resp, err)
		// Associate service_network with vpc
		if err != nil {
			glog.V(6).Infoln("ServiceNetwork is created successfully, but failed to associate service_network and vpc, err: ", err)
			return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
		} else {
			service_networkVPCAssociationStatus := aws.StringValue(resp.Status)
			switch service_networkVPCAssociationStatus {
			case mercury.MeshVpcAssociationStatusCreateInProgress:
				return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
			case mercury.MeshVpcAssociationStatusActive:
				return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: service_networkArn, ServiceNetworkID: service_networkID}, nil
			case mercury.MeshVpcAssociationStatusCreateFailed:
				return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
			case mercury.MeshVpcAssociationStatusDeleteFailed:
				return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
			case mercury.MeshVpcAssociationStatusDeleteInProgress:
				return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
			}
		}
	}
	return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: service_networkArn, ServiceNetworkID: service_networkID}, nil
}

// return all service_networkes associated with VPC
func (m *defaultServiceNetworkManager) List(ctx context.Context) ([]string, error) {
	mercurySess := m.cloud.Mercury()
	service_networkListInput := mercury.ListMeshesInput{MaxResults: nil}
	resp, err := mercurySess.ListMeshesAsList(ctx, &service_networkListInput)

	var service_networkList = make([]string, 0)
	if err == nil {
		for _, r := range resp {
			service_networkList = append(service_networkList, aws.StringValue(r.Name))
		}
	}

	glog.V(6).Infof("defaultServiceNetworkManager: List return %v \n", service_networkList)
	return service_networkList, nil
}

func (m *defaultServiceNetworkManager) Delete(ctx context.Context, service_network string) error {
	service_networkSummary, err := m.findServiceNetworkByName(ctx, service_network)
	if err != nil {
		return err
	}

	var ifServiceNetworkExist bool
	if service_networkSummary == nil {
		ifServiceNetworkExist = false
	} else {
		ifServiceNetworkExist = true
	}

	if ifServiceNetworkExist == false {
		glog.V(6).Infof("ServiceNetworkManager-Delete, successfully deleting unknown service_network %v\n", service_network)
		return nil
	}

	mercurySess := m.cloud.Mercury()
	service_networkID := aws.StringValue(service_networkSummary.Id)
	deleteNeedRetry := false

	_, service_networkAssociatedWithCurrentVPCId, service_networkVPCAssociations, err := m.isServiceNetworkAssociatedWithVPC(ctx, service_networkID)
	if err != nil {
		return err
	}
	totalAssociation := len(service_networkVPCAssociations)
	if totalAssociation == 0 {
		deleteNeedRetry = false
	} else if service_networkAssociatedWithCurrentVPCId != nil {
		if err == errors.New(LATTICE_RETRY) {
			// The current association is in progress
			deleteNeedRetry = true
			return errors.New(LATTICE_RETRY)
		} else {
			// Happy case, this is the last association
			deleteNeedRetry = true
			deleteServiceNetworkVpcAssociationInput := mercury.DeleteMeshVpcAssociationInput{
				MeshVpcAssociationIdentifier: service_networkAssociatedWithCurrentVPCId,
			}
			glog.V(2).Infof("DeleteServiceNetworkVpcAssociationInput >>>> %v\n", deleteServiceNetworkVpcAssociationInput)
			resp, err := mercurySess.DeleteMeshVpcAssociationWithContext(ctx, &deleteServiceNetworkVpcAssociationInput)
			if err != nil {
				glog.V(6).Infof("Failed to delete association %v \n", err)
			}
			glog.V(2).Infof("DeleteServiceNetworkVPCAssociationResp: service_network %v , resp %v, err %v \n", service_network, resp, err)
		}
	} else {
		// there are other Associations left for this service_network, could not delete
		deleteNeedRetry = true
		return nil
	}

	deleteInput := mercury.DeleteMeshInput{
		MeshIdentifier: &service_networkID,
	}
	_, err = mercurySess.DeleteMeshWithContext(ctx, &deleteInput)
	if err != nil {
		return err
	}

	if deleteNeedRetry {
		return errors.New(LATTICE_RETRY)
	} else {
		glog.V(6).Infof("Successfully delete service_network %v\n", service_network)
		return err
	}
}

// Find service_network by name return service_network,err if service_network exists, otherwise return nil, nil.
func (m *defaultServiceNetworkManager) findServiceNetworkByName(ctx context.Context, targetServiceNetwork string) (*mercury.MeshSummary, error) {
	mercurySess := m.cloud.Mercury()
	service_networkListInput := mercury.ListMeshesInput{}
	resp, err := mercurySess.ListMeshesAsList(ctx, &service_networkListInput)
	if err == nil {
		for _, r := range resp {
			if aws.StringValue(r.Name) == targetServiceNetwork {
				glog.V(6).Infoln("Found ServiceNetwork named ", targetServiceNetwork)
				return r, err
			}
		}
		return nil, err
	} else {
		return nil, err
	}
}

// If service_network exists, check if service_network has already associated with VPC
func (m *defaultServiceNetworkManager) isServiceNetworkAssociatedWithVPC(ctx context.Context, service_networkID string) (bool, *string, []*mercury.MeshVpcAssociationSummary, error) {
	mercurySess := m.cloud.Mercury()
	// TODO: can pass vpc id to ListServiceNetworkVpcAssociationsInput, could return err if no associations
	associationStatusInput := mercury.ListMeshVpcAssociationsInput{
		MeshIdentifier: &service_networkID,
	}

	resp, err := mercurySess.ListMeshVpcAssociationsAsList(ctx, &associationStatusInput)
	if err != nil {
		return false, nil, nil, err
	}

	if len(resp) == 0 {
		return false, nil, nil, err
	}

	for _, r := range resp {
		if aws.StringValue(r.VpcId) == config.VpcID {
			associationStatus := aws.StringValue(r.Status)
			if err == nil {
				switch associationStatus {
				case mercury.MeshVpcAssociationStatusActive:
					glog.V(6).Infoln("Mesh and Vpc association is active.")
					return true, r.Id, resp, nil
				case mercury.MeshVpcAssociationStatusCreateFailed:
					glog.V(6).Infoln("Mesh and Vpc association does not exists, start creating service_network and vpc association")
					return false, r.Id, resp, nil
				case mercury.MeshVpcAssociationStatusDeleteFailed:
					glog.V(6).Infoln("Mesh and Vpc association failed to delete")
					return true, r.Id, resp, nil
				case mercury.MeshVpcAssociationStatusDeleteInProgress:
					glog.V(6).Infoln("ServiceNetwork and Vpc association is being deleted, please retry later")
					return true, r.Id, resp, errors.New(LATTICE_RETRY)
				case mercury.MeshVpcAssociationStatusCreateInProgress:
					glog.V(6).Infoln("ServiceNetwork and Vpc association is being created, please retry later")
					return true, r.Id, resp, errors.New(LATTICE_RETRY)
				}
			}
		}
	}
	return false, nil, resp, err
}
