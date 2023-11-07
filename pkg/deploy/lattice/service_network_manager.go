package lattice

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/exp/slices"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

//go:generate mockgen -destination service_network_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice ServiceNetworkManager

type ServiceNetworkManager interface {
	CreateOrUpdate(ctx context.Context, serviceNetwork *model.ServiceNetwork) (model.ServiceNetworkStatus, error)
	List(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, serviceNetwork string) error
	UpsertVpcAssociation(ctx context.Context, snName string, sgIds []*string) (string, error)
	DeleteVpcAssociation(ctx context.Context, snName string) error
}

func NewDefaultServiceNetworkManager(log gwlog.Logger, cloud pkg_aws.Cloud) *defaultServiceNetworkManager {
	return &defaultServiceNetworkManager{
		log:   log,
		cloud: cloud,
	}
}

type defaultServiceNetworkManager struct {
	log   gwlog.Logger
	cloud pkg_aws.Cloud
}

func (m *defaultServiceNetworkManager) UpsertVpcAssociation(ctx context.Context, snName string, sgIds []*string) (string, error) {
	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, snName, "")
	if err != nil {
		return "", err
	}

	associated, snva, _, err := m.isServiceNetworkAlreadyAssociatedWithVPC(ctx, *sn.SvcNetwork.Id)
	if err != nil {
		return "", err
	}
	if associated {
		owned, err := m.cloud.CheckAndAcquireOwnership(ctx, *snva.Arn)
		if err != nil {
			return "", err
		}
		if !owned {
			return "", services.NewConflictError("snva", snName,
				fmt.Sprintf("Found existing vpc association not owned by controller: %s", *snva.Arn))
		}
		_, err = m.UpdateServiceNetworkVpcAssociation(ctx, &sn.SvcNetwork, sgIds, snva.Id)
		if err != nil {
			return "", err
		}
		return *snva.Arn, nil
	} else {
		req := vpclattice.CreateServiceNetworkVpcAssociationInput{
			ServiceNetworkIdentifier: sn.SvcNetwork.Id,
			VpcIdentifier:            &config.VpcID,
			SecurityGroupIds:         sgIds,
			Tags:                     m.cloud.DefaultTags(),
		}
		m.log.Debugf("Creating association between ServiceNetwork %s and VPC %s", sn.SvcNetwork.Id, config.VpcID)
		resp, err := m.cloud.Lattice().CreateServiceNetworkVpcAssociationWithContext(ctx, &req)
		if err != nil {
			return "", err
		}

		serviceNetworkVpcAssociationStatus := aws.StringValue(resp.Status)
		if serviceNetworkVpcAssociationStatus == vpclattice.ServiceNetworkVpcAssociationStatusActive {
			return *resp.Arn, nil
		} else {
			m.log.Infof("Service network/vpc association is not in the active state. State is %s, will retry",
				serviceNetworkVpcAssociationStatus)

			return *resp.Arn, errors.New(LATTICE_RETRY)
		}
	}
}

func (m *defaultServiceNetworkManager) DeleteVpcAssociation(ctx context.Context, snName string) error {
	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, snName, "")
	if err != nil {
		return err
	}

	associated, snva, _, err := m.isServiceNetworkAlreadyAssociatedWithVPC(ctx, *sn.SvcNetwork.Id)
	if err != nil {
		return err
	}
	if associated {
		m.log.Debugf("Disassociating ServiceNetwork %s from VPC", snName)

		owned, err := m.cloud.IsArnManaged(ctx, *snva.Arn)
		if err != nil {
			return err
		}
		if !owned {
			m.log.Infof("Association %s for %s not owned by controller, skipping deletion", *snva.Arn, snName)
			return nil
		}

		deleteServiceNetworkVpcAssociationInput := vpclattice.DeleteServiceNetworkVpcAssociationInput{
			ServiceNetworkVpcAssociationIdentifier: snva.Id,
		}
		resp, err := m.cloud.Lattice().DeleteServiceNetworkVpcAssociationWithContext(ctx, &deleteServiceNetworkVpcAssociationInput)
		if err != nil {
			m.log.Infof("Failed to delete association %s for %s, with response %s and err %s", *snva.Arn, snName, resp, err.Error())
		}
		return errors.New(LATTICE_RETRY)
	}
	return nil
}

// CreateOrUpdate will try to create a service_network and associate the service_network with vpc.
// Or try to update the security groups for the serviceNetworkVpcAssociation if security groups are changed.
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
//	CreateServiceNetworkVpcAssociationInput returns ServiceNetworkVpcAssociationStatusFailed/ServiceNetworkVpcAssociationStatusCreateInProgress/ServiceNetworkVpcAssociationStatusDeleteInProgress

func (m *defaultServiceNetworkManager) CreateOrUpdate(ctx context.Context, serviceNetwork *model.ServiceNetwork) (model.ServiceNetworkStatus, error) {
	// check if exists
	foundSnSummary, err := m.cloud.Lattice().FindServiceNetwork(ctx, serviceNetwork.Spec.Name, "")
	if err != nil && !services.IsNotFoundError(err) {
		return model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
	}

	// pre declaration
	var serviceNetworkId string
	var serviceNetworkArn string
	var isSnAlreadyAssociatedWithCurrentVpc bool
	var snvaAssociatedWithCurrentVPC *vpclattice.ServiceNetworkVpcAssociationSummary
	vpcLatticeSess := m.cloud.Lattice()
	if foundSnSummary == nil {
		m.log.Debugf("Creating ServiceNetwork %s and tagging it with vpcId %s",
			serviceNetwork.Spec.Name, config.VpcID)
		// Add tag to show this is the VPC create this service network
		// This means, the service network can only be deleted by the controller running in this VPC
		serviceNetworkInput := vpclattice.CreateServiceNetworkInput{
			Name: &serviceNetwork.Spec.Name,
			Tags: m.cloud.DefaultTags(),
		}
		serviceNetworkInput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID

		m.log.Debugf("Creating ServiceNetwork %+v", serviceNetworkInput)
		resp, err := vpcLatticeSess.CreateServiceNetworkWithContext(ctx, &serviceNetworkInput)
		if err != nil {
			return model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
		}

		serviceNetworkId = aws.StringValue(resp.Id)
		serviceNetworkArn = aws.StringValue(resp.Arn)
	} else {
		m.log.Debugf("ServiceNetwork %s exists, checking its VPC association", serviceNetwork.Spec.Name)
		serviceNetworkId = aws.StringValue(foundSnSummary.SvcNetwork.Id)
		serviceNetworkArn = aws.StringValue(foundSnSummary.SvcNetwork.Arn)
		isSnAlreadyAssociatedWithCurrentVpc, snvaAssociatedWithCurrentVPC, _, err = m.isServiceNetworkAlreadyAssociatedWithVPC(ctx, serviceNetworkId)
		if err != nil {
			return model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
		}
		if serviceNetwork.Spec.AssociateToVPC == true && isSnAlreadyAssociatedWithCurrentVpc == true &&
			snvaAssociatedWithCurrentVPC.Status != nil && aws.StringValue(snvaAssociatedWithCurrentVPC.Status) == vpclattice.ServiceNetworkVpcAssociationStatusActive {
			return m.UpdateServiceNetworkVpcAssociation(ctx, &foundSnSummary.SvcNetwork, serviceNetwork.Spec.SecurityGroupIds, snvaAssociatedWithCurrentVPC.Id)
		}
	}

	if serviceNetwork.Spec.AssociateToVPC == true {
		if isSnAlreadyAssociatedWithCurrentVpc == false {
			// current state:  service network is associated to VPC
			// desired state:  associate this service network to VPC
			createServiceNetworkVpcAssociationInput := vpclattice.CreateServiceNetworkVpcAssociationInput{
				ServiceNetworkIdentifier: &serviceNetworkId,
				VpcIdentifier:            &config.VpcID,
				SecurityGroupIds:         serviceNetwork.Spec.SecurityGroupIds,
			}
			m.log.Debugf("Creating association between ServiceNetwork %s and VPC %s", serviceNetworkId, config.VpcID)
			resp, err := vpcLatticeSess.CreateServiceNetworkVpcAssociationWithContext(ctx, &createServiceNetworkVpcAssociationInput)
			if err != nil {
				return model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
			}

			serviceNetworkVpcAssociationStatus := aws.StringValue(resp.Status)
			if serviceNetworkVpcAssociationStatus == vpclattice.ServiceNetworkVpcAssociationStatusActive {
				return model.ServiceNetworkStatus{ServiceNetworkARN: serviceNetworkArn, ServiceNetworkID: serviceNetworkId}, nil
			} else {
				m.log.Infof("Service network/vpc association is not in the active state. State is %s, will retry",
					serviceNetworkVpcAssociationStatus)

				return model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
			}
		}
	} else {
		// serviceNetwork.Spec.AssociateToVPC == false
		if isSnAlreadyAssociatedWithCurrentVpc == true {
			// current state: service network is associated to VPC
			// desired state: not to associate this service network to VPC
			m.log.Debugf("Disassociating ServiceNetwork %s from VPC", serviceNetwork.Spec.Name)
			deleteServiceNetworkVpcAssociationInput := vpclattice.DeleteServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: snvaAssociatedWithCurrentVPC.Id,
			}
			resp, err := vpcLatticeSess.DeleteServiceNetworkVpcAssociationWithContext(ctx, &deleteServiceNetworkVpcAssociationInput)
			if err != nil {
				m.log.Errorf("Failed to delete association for %s, with response %s and err %s",
					serviceNetwork.Spec.Name, resp, err)
			}

			// return retry and check later if disassociation workflow finishes
			return model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, errors.New(LATTICE_RETRY)
		}
		m.log.Debugf("Created ServiceNetwork %s without VPC association", serviceNetwork.Spec.Name)
	}
	return model.ServiceNetworkStatus{ServiceNetworkARN: serviceNetworkArn, ServiceNetworkID: serviceNetworkId}, nil
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
	_, snvaAssociatedWithCurrentVPC, assocResp, err := m.isServiceNetworkAlreadyAssociatedWithVPC(ctx, serviceNetworkId)
	if err != nil {
		return err
	}
	if snvaAssociatedWithCurrentVPC != nil && snvaAssociatedWithCurrentVPC.Id != nil {
		// Current VPC is associated with this service network
		// Happy case, disassociate the VPC from service network
		deleteServiceNetworkVpcAssociationInput := vpclattice.DeleteServiceNetworkVpcAssociationInput{
			ServiceNetworkVpcAssociationIdentifier: snvaAssociatedWithCurrentVPC.Id,
		}
		m.log.Debugf("Deleting ServiceNetworkVpcAssociation %s", *snvaAssociatedWithCurrentVPC.Id)
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
		vpcOwner, ok := serviceNetworkSummary.Tags[model.K8SServiceNetworkOwnedByVPC]
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
			return fmt.Errorf("%w: failed to delete service network %s due to %s", RetryErr, snName, resp)
		}

		return err
	} else {
		m.log.Debugf("Skipped deleting service network %s since it is owned by different VPC", snName)
		return nil
	}
}

// If service_network exists, check if service_network has already associated with VPC
func (m *defaultServiceNetworkManager) isServiceNetworkAlreadyAssociatedWithVPC(ctx context.Context, serviceNetworkId string) (bool, *vpclattice.ServiceNetworkVpcAssociationSummary, []*vpclattice.ServiceNetworkVpcAssociationSummary, error) {
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
					m.log.Debugf("ServiceNetwork and Vpc association is active.")
					return true, r, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed:
					m.log.Debugf("ServiceNetwork and Vpc association does not exists, start creating service_network and vpc association")
					return false, r, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusDeleteFailed:
					m.log.Debugf("ServiceNetwork and Vpc association failed to delete")
					return true, r, resp, nil
				case vpclattice.ServiceNetworkVpcAssociationStatusDeleteInProgress:
					m.log.Debugf("ServiceNetwork and Vpc association is being deleted, retry later")
					return true, r, resp, errors.New(LATTICE_RETRY)
				case vpclattice.ServiceNetworkVpcAssociationStatusCreateInProgress:
					m.log.Debugf("ServiceNetwork and Vpc association is being created, retry later")
					return true, r, resp, errors.New(LATTICE_RETRY)
				}
			}
		}
	}
	return false, nil, resp, err
}

func (m *defaultServiceNetworkManager) UpdateServiceNetworkVpcAssociation(ctx context.Context, existingSN *vpclattice.ServiceNetworkSummary, sgIds []*string, existingSnvaId *string) (model.ServiceNetworkStatus, error) {
	retrievedSnva, err := m.cloud.Lattice().GetServiceNetworkVpcAssociationWithContext(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
		ServiceNetworkVpcAssociationIdentifier: existingSnvaId,
	})
	if err != nil {
		return model.ServiceNetworkStatus{}, err
	}
	sgIdsEqual := securityGroupIdsEqual(sgIds, retrievedSnva.SecurityGroupIds)
	if sgIdsEqual {
		// desiredSN's security group ids are same with retrievedSnva's security group ids, don't need to update
		return model.ServiceNetworkStatus{
			ServiceNetworkID:     *existingSN.Id,
			ServiceNetworkARN:    *existingSN.Arn,
			SnvaSecurityGroupIds: retrievedSnva.SecurityGroupIds,
		}, nil
	}
	updateSnvaResp, err := m.cloud.Lattice().UpdateServiceNetworkVpcAssociationWithContext(ctx, &vpclattice.UpdateServiceNetworkVpcAssociationInput{
		ServiceNetworkVpcAssociationIdentifier: existingSnvaId,
		SecurityGroupIds:                       sgIds,
	})
	if err != nil {
		return model.ServiceNetworkStatus{}, err
	}
	if *updateSnvaResp.Status == vpclattice.ServiceNetworkVpcAssociationStatusActive {
		return model.ServiceNetworkStatus{
			ServiceNetworkID:     *existingSN.Id,
			ServiceNetworkARN:    *existingSN.Arn,
			SnvaSecurityGroupIds: updateSnvaResp.SecurityGroupIds,
		}, nil
	} else {
		return model.ServiceNetworkStatus{}, fmt.Errorf("%w, update svna status: %s", RetryErr, *updateSnvaResp.Status)
	}
}

func securityGroupIdsEqual(arr1, arr2 []*string) bool {
	ids1 := utils.SliceMap(arr1, aws.StringValue)
	slices.Sort(ids1)
	ids2 := utils.SliceMap(arr2, aws.StringValue)
	slices.Sort(ids2)
	return slices.Equal(ids1, ids2)
}
