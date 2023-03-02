package lattice

import (
	"context"
	"errors"
	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type ServiceNetworkManager interface {
	Create(ctx context.Context, service_network *latticemodel.ServiceNetwork) (latticemodel.ServiceNetworkStatus, error)
	List(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, service_network string) error
}

type serviceNetworkOutput struct {
	snSummary *vpclattice.ServiceNetworkSummary
	snTags    *vpclattice.ListTagsForResourceOutput
}

func NewDefaultServiceNetworkManager(cloud lattice_aws.Cloud) *defaultServiceNetworkManager {
	return &defaultServiceNetworkManager{
		cloud: cloud,
	}
}

var _service_networkManager = &defaultServiceNetworkManager{}

type defaultServiceNetworkManager struct {
	cloud lattice_aws.Cloud
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
	var service_networkAssociatedWithCurrentVPCId *string
	vpcLatticeSess := m.cloud.Lattice()
	if service_networkSummary == nil {
		glog.V(2).Infof("ServiceNetwork Create API here, service_network[%v] vpciID[%s]\n", service_network, config.VpcID)
		// Add tag to show this is the VPC create this servicenetwork
		// This means, the servicenetwork can only be deleted by the controller running in this VPC

		service_networkInput := vpclattice.CreateServiceNetworkInput{
			Name: &service_network.Spec.Name,
			Tags: make(map[string]*string),
		}
		service_networkInput.Tags[latticemodel.K8SServiceNetworkOwnedByVPC] = &config.VpcID
		resp, err := vpcLatticeSess.CreateServiceNetworkWithContext(ctx, &service_networkInput)
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
		service_networkID = aws.StringValue(service_networkSummary.snSummary.Id)
		service_networkArn = aws.StringValue(service_networkSummary.snSummary.Arn)
		isServiceNetworkAssociatedWithVPC, service_networkAssociatedWithCurrentVPCId, _, err = m.isServiceNetworkAssociatedWithVPC(ctx, service_networkID)
		if err != nil {
			return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
		}
	}

	if service_network.Spec.AssociateToVPC == true {
		if isServiceNetworkAssociatedWithVPC == false {
			createServiceNetworkVpcAssociationInput := vpclattice.CreateServiceNetworkVpcAssociationInput{
				ServiceNetworkIdentifier: &service_networkID,
				VpcIdentifier:            &config.VpcID,
			}
			glog.V(2).Infof("Create service_network/vpc association >>>> req[%v]\n", createServiceNetworkVpcAssociationInput)
			resp, err := vpcLatticeSess.CreateServiceNetworkVpcAssociationWithContext(ctx, &createServiceNetworkVpcAssociationInput)
			glog.V(2).Infof("Create service_network and vpc association here >>>> resp[%v] err [%v]\n", resp, err)
			// Associate service_network with vpc
			if err != nil {
				glog.V(6).Infoln("ServiceNetwork is created successfully, but failed to associate service_network and vpc, err: ", err)
				return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
			} else {
				service_networkVPCAssociationStatus := aws.StringValue(resp.Status)
				switch service_networkVPCAssociationStatus {
				case vpclattice.ServiceNetworkVpcAssociationStatusCreateInProgress:
					return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
				case vpclattice.ServiceNetworkVpcAssociationStatusActive:
					return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: service_networkArn, ServiceNetworkID: service_networkID}, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed:
					return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
				case vpclattice.ServiceNetworkVpcAssociationStatusDeleteFailed:
					return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
				case vpclattice.ServiceNetworkVpcAssociationStatusDeleteInProgress:
					return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
				}
			}
		}
	} else {
		if isServiceNetworkAssociatedWithVPC == true {
			glog.V(2).Infof("Disassociate service_network(%v) from vpc association", service_network.Spec.Name)

			deleteServiceNetworkVpcAssociationInput := vpclattice.DeleteServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: service_networkAssociatedWithCurrentVPCId,
			}

			resp, err := vpcLatticeSess.DeleteServiceNetworkVpcAssociationWithContext(ctx, &deleteServiceNetworkVpcAssociationInput)
			if err != nil {
				glog.V(2).Infof("Failed to delete association for %v err=%v , resp = %v\n", service_network.Spec.Name, err, resp)
			}

			// return retry and check later if disassociation workflow finishes
			return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)

		}
		glog.V(2).Infof("Create service_network(%v) without vpc association", service_network.Spec.Name)
	}
	return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: service_networkArn, ServiceNetworkID: service_networkID}, nil
}

// return all service_networkes associated with VPC
func (m *defaultServiceNetworkManager) List(ctx context.Context) ([]string, error) {
	vpcLatticeSess := m.cloud.Lattice()
	service_networkListInput := vpclattice.ListServiceNetworksInput{MaxResults: nil}
	resp, err := vpcLatticeSess.ListServiceNetworksAsList(ctx, &service_networkListInput)

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

	if service_networkSummary == nil {
		glog.V(6).Infof("Successfully deleted unknown service_network %v\n", service_network)
		return nil
	}

	vpcLatticeSess := m.cloud.Lattice()
	service_networkID := aws.StringValue(service_networkSummary.snSummary.Id)

	_, service_networkAssociatedWithCurrentVPCId, _, err := m.isServiceNetworkAssociatedWithVPC(ctx, service_networkID)
	if err != nil {
		return err
	}
	if service_networkAssociatedWithCurrentVPCId != nil {
		// current VPC is associated with this service network

		// Happy case, disassociate the VPC from service network
		deleteServiceNetworkVpcAssociationInput := vpclattice.DeleteServiceNetworkVpcAssociationInput{
			ServiceNetworkVpcAssociationIdentifier: service_networkAssociatedWithCurrentVPCId,
		}
		glog.V(2).Infof("DeleteServiceNetworkVpcAssociationInput >>>> %v\n", deleteServiceNetworkVpcAssociationInput)
		resp, err := vpcLatticeSess.DeleteServiceNetworkVpcAssociationWithContext(ctx, &deleteServiceNetworkVpcAssociationInput)
		if err != nil {
			glog.V(2).Infof("Failed to delete association %v \n", err)
		}
		glog.V(2).Infof("DeleteServiceNetworkVPCAssociationResp: service_network %v , resp %v, err %v \n", service_network, resp, err)
		// retry later to check if VPC disassociation workflow finishes
		return errors.New(LATTICE_RETRY)

	}

	// check if this VPC that created the service network
	needToDelete := false
	if service_networkSummary.snTags != nil && service_networkSummary.snTags.Tags != nil {
		snTags := service_networkSummary.snTags
		vpcOwner, ok := snTags.Tags[latticemodel.K8SServiceNetworkOwnedByVPC]
		if ok && *vpcOwner == config.VpcID {
			needToDelete = true
		}
	}

	if needToDelete {

		deleteInput := vpclattice.DeleteServiceNetworkInput{
			ServiceNetworkIdentifier: &service_networkID,
		}
		_, err = vpcLatticeSess.DeleteServiceNetworkWithContext(ctx, &deleteInput)
		if err != nil {
			return errors.New(LATTICE_RETRY)
		}

		glog.V(2).Infof("Successfully delete service_network %v\n", service_network)
		return err

	} else {
		glog.V(2).Infof("Deleting service_network (%v) Skipped, since it is owned by different VPC ", service_network)
		return nil
	}
}

// Find service_network by name return service_network,err if service_network exists, otherwise return nil, nil.
func (m *defaultServiceNetworkManager) findServiceNetworkByName(ctx context.Context, targetServiceNetwork string) (*serviceNetworkOutput, error) {
	vpcLatticeSess := m.cloud.Lattice()
	service_networkListInput := vpclattice.ListServiceNetworksInput{}
	resp, err := vpcLatticeSess.ListServiceNetworksAsList(ctx, &service_networkListInput)
	if err == nil {
		for _, r := range resp {
			if aws.StringValue(r.Name) == targetServiceNetwork {
				glog.V(6).Infoln("Found ServiceNetwork named ", targetServiceNetwork)

				tagsInput := vpclattice.ListTagsForResourceInput{
					ResourceArn: r.Arn,
				}
				tagsOutput, err := vpcLatticeSess.ListTagsForResourceWithContext(ctx, &tagsInput)

				if err != nil {
					tagsOutput = nil
				}

				snOutput := serviceNetworkOutput{
					snSummary: r,
					snTags:    tagsOutput,
				}

				return &snOutput, err
			}
		}
		return nil, err
	} else {
		return nil, err
	}
}

// If service_network exists, check if service_network has already associated with VPC
func (m *defaultServiceNetworkManager) isServiceNetworkAssociatedWithVPC(ctx context.Context, service_networkID string) (bool, *string, []*vpclattice.ServiceNetworkVpcAssociationSummary, error) {
	vpcLatticeSess := m.cloud.Lattice()
	// TODO: can pass vpc id to ListServiceNetworkVpcAssociationsInput, could return err if no associations
	associationStatusInput := vpclattice.ListServiceNetworkVpcAssociationsInput{
		ServiceNetworkIdentifier: &service_networkID,
	}

	resp, err := vpcLatticeSess.ListServiceNetworkVpcAssociationsAsList(ctx, &associationStatusInput)
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
				case vpclattice.ServiceNetworkVpcAssociationStatusActive:
					glog.V(6).Infoln("Mesh and Vpc association is active.")
					return true, r.Id, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed:
					glog.V(6).Infoln("Mesh and Vpc association does not exists, start creating service_network and vpc association")
					return false, r.Id, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusDeleteFailed:
					glog.V(6).Infoln("Mesh and Vpc association failed to delete")
					return true, r.Id, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusDeleteInProgress:
					glog.V(6).Infoln("ServiceNetwork and Vpc association is being deleted, please retry later")
					return true, r.Id, resp, errors.New(LATTICE_RETRY)
				case vpclattice.ServiceNetworkVpcAssociationStatusCreateInProgress:
					glog.V(6).Infoln("ServiceNetwork and Vpc association is being created, please retry later")
					return true, r.Id, resp, errors.New(LATTICE_RETRY)
				}
			}
		}
	}
	return false, nil, resp, err
}
