package lattice

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type ServiceNetworkManager interface {
	Create(ctx context.Context, serviceNetwork *latticemodel.ServiceNetwork) (latticemodel.ServiceNetworkStatus, error)
	List(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, serviceNetwork string) error
}

type serviceNetworkOutput struct {
	snSummary *vpclattice.ServiceNetworkSummary
	snTags    *vpclattice.ListTagsForResourceOutput
}

func NewDefaultServiceNetworkManager(log gwlog.Logger, cloud lattice_aws.Cloud) *defaultServiceNetworkManager {
	return &defaultServiceNetworkManager{
		log:   log,
		cloud: cloud,
	}
}

type defaultServiceNetworkManager struct {
	log   gwlog.Logger
	cloud lattice_aws.Cloud
}

// Create will try to create a service_network and associate the service_network with vpc
// return error when:
//
//	ListServiceNetworksWithContext returns error
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
func (m *defaultServiceNetworkManager) Create(ctx context.Context, serviceNetwork *latticemodel.ServiceNetwork) (latticemodel.ServiceNetworkStatus, error) {
	// check if exists
	serviceNetworkSummary, err := m.findServiceNetworkByName(ctx, serviceNetwork.Spec.Name)
	if err != nil {
		return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
	}

	// pre declaration
	var serviceNetworkId string
	var serviceNetworkArn string
	var isServiceNetworkAssociatedWithVpc bool
	var serviceNetworkAssociatedWithCurrentVpcId *string
	vpcLatticeSess := m.cloud.Lattice()
	if serviceNetworkSummary == nil {
		m.log.Debugf("Create ServiceNetwork, service_network[%v] and tag it with vpciID[%s]", serviceNetwork, config.VpcID)
		// Add tag to show this is the VPC create this servicenetwork
		// This means, the servicenetwork can only be deleted by the controller running in this VPC

		serviceNetworkInput := vpclattice.CreateServiceNetworkInput{
			Name: &serviceNetwork.Spec.Name,
			Tags: make(map[string]*string),
		}
		serviceNetworkInput.Tags[latticemodel.K8SServiceNetworkOwnedByVPC] = &config.VpcID

		m.log.Debugf("Create service_network >>>> req[%v]", serviceNetworkInput)
		resp, err := vpcLatticeSess.CreateServiceNetworkWithContext(ctx, &serviceNetworkInput)
		m.log.Debugf("Create service_network >>>> resp[%v], err : %v", resp, err)
		if err != nil {
			m.log.Debugf("Failed to create service_network[%v], err: %v", serviceNetwork, err)
			return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
		}
		serviceNetworkId = aws.StringValue(resp.Id)
		serviceNetworkArn = aws.StringValue(resp.Arn)
		isServiceNetworkAssociatedWithVpc = false
		m.log.Infof(" ServiceNetwork Create API resp [%v]", resp)

	} else {
		m.log.Infof("service_network[%v] exists, further check association", serviceNetwork)
		serviceNetworkId = aws.StringValue(serviceNetworkSummary.snSummary.Id)
		serviceNetworkArn = aws.StringValue(serviceNetworkSummary.snSummary.Arn)
		isServiceNetworkAssociatedWithVpc, serviceNetworkAssociatedWithCurrentVpcId, _, err = m.isServiceNetworkAssociatedWithVPC(ctx, serviceNetworkId)
		if err != nil {
			return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
		}
	}

	if serviceNetwork.Spec.AssociateToVPC == true {
		if isServiceNetworkAssociatedWithVpc == false {
			// current state:  service network is associated to VPC
			// desired state:  associate this service network to VPC
			createServiceNetworkVpcAssociationInput := vpclattice.CreateServiceNetworkVpcAssociationInput{
				ServiceNetworkIdentifier: &serviceNetworkId,
				VpcIdentifier:            &config.VpcID,
			}
			m.log.Debugf("Create service_network/vpc association >>>> req[%v]", createServiceNetworkVpcAssociationInput)
			resp, err := vpcLatticeSess.CreateServiceNetworkVpcAssociationWithContext(ctx, &createServiceNetworkVpcAssociationInput)
			m.log.Debugf("Create service_network and vpc association here >>>> resp[%v] err [%v]", resp, err)
			// Associate service_network with vpc
			if err != nil {
				m.log.Debugf("Failed to associate service_network[%v] and vpc, err: %v", serviceNetwork, err)
				return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
			} else {
				serviceNetworkVpcAssociationStatus := aws.StringValue(resp.Status)
				switch serviceNetworkVpcAssociationStatus {
				case vpclattice.ServiceNetworkVpcAssociationStatusCreateInProgress:
					return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
				case vpclattice.ServiceNetworkVpcAssociationStatusActive:
					return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: serviceNetworkArn, ServiceNetworkID: serviceNetworkId}, nil
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
		if isServiceNetworkAssociatedWithVpc == true {
			// current state: service network is associated to VPC
			// desired state: not to associate this service network to VPC
			m.log.Infof("Disassociate service_network(%v) from vpc association", serviceNetwork.Spec.Name)

			deleteServiceNetworkVpcAssociationInput := vpclattice.DeleteServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: serviceNetworkAssociatedWithCurrentVpcId,
			}

			m.log.Debugf("Delete service_network association >>>> req[%v]", deleteServiceNetworkVpcAssociationInput)
			resp, err := vpcLatticeSess.DeleteServiceNetworkVpcAssociationWithContext(ctx, &deleteServiceNetworkVpcAssociationInput)
			m.log.Debugf("Delete service_network association >>>> resp[%v],err [%v]", resp, err)
			if err != nil {
				m.log.Debugf("Failed to delete association for %v err=%v , resp = %v", serviceNetwork.Spec.Name, err, resp)
			}

			// return retry and check later if disassociation workflow finishes
			return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)

		}
		m.log.Debugf("Created service_network(%v) without vpc association", serviceNetwork.Spec.Name)
	}
	return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: serviceNetworkArn, ServiceNetworkID: serviceNetworkId}, nil
}

// return all service_networkes associated with VPC
func (m *defaultServiceNetworkManager) List(ctx context.Context) ([]string, error) {
	vpcLatticeSess := m.cloud.Lattice()
	serviceNetworkListInput := vpclattice.ListServiceNetworksInput{MaxResults: nil}
	resp, err := vpcLatticeSess.ListServiceNetworksAsList(ctx, &serviceNetworkListInput)

	var serviceNetworkList = make([]string, 0)
	if err == nil {
		for _, r := range resp {
			serviceNetworkList = append(serviceNetworkList, aws.StringValue(r.Name))
		}
	}

	m.log.Infof("defaultServiceNetworkManager: List return %v", serviceNetworkList)
	return serviceNetworkList, nil
}

func (m *defaultServiceNetworkManager) Delete(ctx context.Context, serviceNetwork string) error {
	serviceNetworkSummary, err := m.findServiceNetworkByName(ctx, serviceNetwork)
	if err != nil {
		return err
	}

	if serviceNetworkSummary == nil {
		m.log.Debugf("Successfully deleted unknown service_network %v", serviceNetwork)
		return nil
	}

	vpcLatticeSess := m.cloud.Lattice()
	serviceNetworkId := aws.StringValue(serviceNetworkSummary.snSummary.Id)

	_, serviceNetworkAssociatedWithCurrentVpcId, assocResp, err := m.isServiceNetworkAssociatedWithVPC(ctx, serviceNetworkId)
	if err != nil {
		return err
	}
	if serviceNetworkAssociatedWithCurrentVpcId != nil {
		// current VPC is associated with this service network

		// Happy case, disassociate the VPC from service network
		deleteServiceNetworkVpcAssociationInput := vpclattice.DeleteServiceNetworkVpcAssociationInput{
			ServiceNetworkVpcAssociationIdentifier: serviceNetworkAssociatedWithCurrentVpcId,
		}
		m.log.Debugf("DeleteServiceNetworkVpcAssociationInput >>>> %v", deleteServiceNetworkVpcAssociationInput)
		resp, err := vpcLatticeSess.DeleteServiceNetworkVpcAssociationWithContext(ctx, &deleteServiceNetworkVpcAssociationInput)
		m.log.Debugf("DeleteServiceNetworkVPCAssociationResp: service_network %v , resp %v, err %v", serviceNetwork, resp, err)
		if err != nil {
			m.log.Debugf("Failed to delete association for %v, err: %v", serviceNetwork, err)
		}
		// retry later to check if VPC disassociation workflow finishes
		return errors.New(LATTICE_RETRY)

	}

	// check if this VPC is the one created the service network
	needToDelete := false
	if serviceNetworkSummary.snTags != nil && serviceNetworkSummary.snTags.Tags != nil {
		snTags := serviceNetworkSummary.snTags
		vpcOwner, ok := snTags.Tags[latticemodel.K8SServiceNetworkOwnedByVPC]
		if ok && *vpcOwner == config.VpcID {
			needToDelete = true
		} else {
			if ok {
				m.log.Debugf("Skip deleting, the service network[%v] is created by VPC %v", serviceNetwork, *vpcOwner)
			} else {
				m.log.Debugf("Skip deleting, the service network[%v] is not created by K8S, since there is no tag", serviceNetwork)
			}
		}
	}

	if needToDelete {

		if len(assocResp) != 0 {
			m.log.Debugf("Retry deleting %v later, due to service network still has VPCs associated", serviceNetwork)
			return errors.New(LATTICE_RETRY)
		}

		deleteInput := vpclattice.DeleteServiceNetworkInput{
			ServiceNetworkIdentifier: &serviceNetworkId,
		}
		m.log.Debugf("DeleteServiceNetworkWithContext: service_network %v", serviceNetwork)
		resp, err := vpcLatticeSess.DeleteServiceNetworkWithContext(ctx, &deleteInput)
		m.log.Debugf("DeleteServiceNetworkWithContext: service_network %v , resp %v, err %v", serviceNetwork, resp, err)
		if err != nil {
			return errors.New(LATTICE_RETRY)
		}

		m.log.Debugf("Successfully delete service_network %v", serviceNetwork)
		return err

	} else {
		m.log.Debugf("Deleting service_network (%v) Skipped, since it is owned by different VPC ", serviceNetwork)
		return nil
	}
}

// Find service_network by name return service_network,err if service_network exists, otherwise return nil, nil.
func (m *defaultServiceNetworkManager) findServiceNetworkByName(ctx context.Context, targetServiceNetwork string) (*serviceNetworkOutput, error) {
	vpcLatticeSess := m.cloud.Lattice()
	serviceNetworkListInput := vpclattice.ListServiceNetworksInput{}
	resp, err := vpcLatticeSess.ListServiceNetworksAsList(ctx, &serviceNetworkListInput)
	if err == nil {
		for _, r := range resp {
			if aws.StringValue(r.Name) == targetServiceNetwork {
				m.log.Infoln("Found ServiceNetwork named ", targetServiceNetwork)

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

				// treat err as no tag
				return &snOutput, nil
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
					m.log.Infoln("Mesh and Vpc association is active.")
					return true, r.Id, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed:
					m.log.Infoln("Mesh and Vpc association does not exists, start creating service_network and vpc association")
					return false, r.Id, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusDeleteFailed:
					m.log.Infoln("Mesh and Vpc association failed to delete")
					return true, r.Id, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusDeleteInProgress:
					m.log.Infoln("ServiceNetwork and Vpc association is being deleted, please retry later")
					return true, r.Id, resp, errors.New(LATTICE_RETRY)
				case vpclattice.ServiceNetworkVpcAssociationStatusCreateInProgress:
					m.log.Infoln("ServiceNetwork and Vpc association is being created, please retry later")
					return true, r.Id, resp, errors.New(LATTICE_RETRY)
				}
			}
		}
	}
	return false, nil, resp, err
}
