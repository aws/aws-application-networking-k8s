package lattice

import (
	"context"
	"errors"

	"github.com/golang/glog"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
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
func (m *defaultServiceNetworkManager) Create(
	ctx context.Context,
	serviceNetwork *latticemodel.ServiceNetwork,
) (latticemodel.ServiceNetworkStatus, error) {
	// check if exists
	serviceNetworkSummary, err := m.cloud.Lattice().FindServiceNetwork(ctx, serviceNetwork.Spec.Name, "")
	if err != nil && !services.IsNotFoundError(err) {
		return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
	}

	// pre declaration
	var serviceNetworkId string
	var serviceNetworkArn string
	var isServiceNetworkAssociatedWithVpc bool
	var serviceNetworkAssociatedWithCurrentVpcId *string
	vpcLatticeSess := m.cloud.Lattice()

	if serviceNetworkSummary == nil {
		m.log.Debugf("Creating ServiceNetwork %s and tagging it with vpcId %s",
			serviceNetwork.Spec.Name, config.VpcID)

		// Add tag to show this is the VPC create this service network
		// This means, the service network can only be deleted by the controller running in this VPC
		serviceNetworkInput := vpclattice.CreateServiceNetworkInput{
			Name: &serviceNetwork.Spec.Name,
			Tags: make(map[string]*string),
		}
		serviceNetworkInput.Tags[latticemodel.K8SServiceNetworkOwnedByVPC] = &config.VpcID

		m.log.Debugf("Creating serviceNetwork %+v", serviceNetworkInput)
		resp, err := vpcLatticeSess.CreateServiceNetworkWithContext(ctx, &serviceNetworkInput)
		if err != nil {
			return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
		}

		serviceNetworkId = aws.StringValue(resp.Id)
		serviceNetworkArn = aws.StringValue(resp.Arn)
		isServiceNetworkAssociatedWithVpc = false
	} else {
		glog.V(6).Infof("serviceNetwork %s exists, checking its VPC association", serviceNetwork.Spec.Name)
		serviceNetworkId = aws.StringValue(serviceNetworkSummary.SvcNetwork.Id)
		serviceNetworkArn = aws.StringValue(serviceNetworkSummary.SvcNetwork.Arn)
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
			m.log.Debugf("Creating association between ServiceNetwork %s and VPC %s", serviceNetworkId, config.VpcID)
			resp, err := vpcLatticeSess.CreateServiceNetworkVpcAssociationWithContext(ctx, &createServiceNetworkVpcAssociationInput)
			if err != nil {
				return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
			}

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
	} else {
		if isServiceNetworkAssociatedWithVpc == true {
			// current state: service network is associated to VPC
			// desired state: not to associate this service network to VPC
			m.log.Debugf("Disassociating ServiceNetwork %s from VPC", serviceNetwork.Spec.Name)
			deleteServiceNetworkVpcAssociationInput := vpclattice.DeleteServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: serviceNetworkAssociatedWithCurrentVpcId,
			}
			resp, err := vpcLatticeSess.DeleteServiceNetworkVpcAssociationWithContext(ctx, &deleteServiceNetworkVpcAssociationInput)
			if err != nil {
				m.log.Errorf("Failed to delete association for %s, with response %s and err %s",
					serviceNetwork.Spec.Name, resp, err)
			}

			// return retry and check later if disassociation workflow finishes
			return latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
		}
		m.log.Debugf("Created serviceNetwork %s without VPC association", serviceNetwork.Spec.Name)
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

	return serviceNetworkList, nil
}

func (m *defaultServiceNetworkManager) Delete(ctx context.Context, snName string) error {
	serviceNetworkSummary, err := m.cloud.Lattice().FindServiceNetwork(ctx, snName, "")
	if err != nil {
		if services.IsNotFoundError(err) {
			m.log.Debugf("ServiceNetwork %s not found, assuming it's already deleted", snName)
			return nil
		} else {
			return err
		}
	}

	vpcLatticeSess := m.cloud.Lattice()
	serviceNetworkId := aws.StringValue(serviceNetworkSummary.SvcNetwork.Id)

	_, serviceNetworkAssociatedWithCurrentVpcId, assocResp, err := m.isServiceNetworkAssociatedWithVPC(ctx, serviceNetworkId)
	if err != nil {
		return err
	}

	if serviceNetworkAssociatedWithCurrentVpcId != nil {
		// Current VPC is associated with this service network
		// Happy case, disassociate the VPC from service network
		deleteServiceNetworkVpcAssociationInput := vpclattice.DeleteServiceNetworkVpcAssociationInput{
			ServiceNetworkVpcAssociationIdentifier: serviceNetworkAssociatedWithCurrentVpcId,
		}
		m.log.Debugf("Deleting ServiceNetworkVpcAssociation %s", *serviceNetworkAssociatedWithCurrentVpcId)
		_, err := vpcLatticeSess.DeleteServiceNetworkVpcAssociationWithContext(ctx, &deleteServiceNetworkVpcAssociationInput)
		if err != nil {
			m.log.Debugf("Failed to delete association for %s, err: %s", snName, err)
		}
		// retry later to check if VPC disassociation workflow finishes
		return errors.New(LATTICE_RETRY)
	}

	// check if this VPC is the one created the service network
	needToDelete := false
	if serviceNetworkSummary.Tags != nil {
		vpcOwner, ok := serviceNetworkSummary.Tags[latticemodel.K8SServiceNetworkOwnedByVPC]
		if ok && *vpcOwner == config.VpcID {
			needToDelete = true
		} else {
			if ok {
				m.log.Debugf("Skip deleting, service network %s is created by VPC %s", snName, *vpcOwner)
			} else {
				m.log.Debugf("Skip deleting, service network %s is not created by K8S, since there is no tag", snName)
			}
		}
	}

	if needToDelete {
		if len(assocResp) != 0 {
			m.log.Debugf("Retry deleting service network %s later since it still has VPCs associated", snName)
			return errors.New(LATTICE_RETRY)
		}

		m.log.Debugf("Deleting service network %s", snName)
		deleteInput := vpclattice.DeleteServiceNetworkInput{
			ServiceNetworkIdentifier: &serviceNetworkId,
		}
		resp, err := vpcLatticeSess.DeleteServiceNetworkWithContext(ctx, &deleteInput)
		if err != nil {
			m.log.Debugf("Failed to delete service network %s due to %s", snName, resp)
			return errors.New(LATTICE_RETRY)
		}

		return err
	} else {
		m.log.Debugf("Skipped deleting service network %s since it is owned by different VPC", snName)
		return nil
	}
}

// If service_network exists, check if service_network has already associated with VPC
func (m *defaultServiceNetworkManager) isServiceNetworkAssociatedWithVPC(
	ctx context.Context,
	serviceNetworkId string,
) (bool, *string, []*vpclattice.ServiceNetworkVpcAssociationSummary, error) {
	vpcLatticeSess := m.cloud.Lattice()
	// TODO: can pass vpc id to ListServiceNetworkVpcAssociationsInput, could return err if no associations
	associationStatusInput := vpclattice.ListServiceNetworkVpcAssociationsInput{
		ServiceNetworkIdentifier: &serviceNetworkId,
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
					m.log.Debugf("Mesh and Vpc association is active.")
					return true, r.Id, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed:
					m.log.Debugf("Mesh and Vpc association does not exists, start creating service_network and vpc association")
					return false, r.Id, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusDeleteFailed:
					m.log.Debugf("Mesh and Vpc association failed to delete")
					return true, r.Id, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusDeleteInProgress:
					m.log.Debugf("ServiceNetwork and Vpc association is being deleted, retry later")
					return true, r.Id, resp, errors.New(LATTICE_RETRY)
				case vpclattice.ServiceNetworkVpcAssociationStatusCreateInProgress:
					m.log.Debugf("ServiceNetwork and Vpc association is being created, retry later")
					return true, r.Id, resp, errors.New(LATTICE_RETRY)
				}
			}
		}
	}
	return false, nil, resp, err
}
