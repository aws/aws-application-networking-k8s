package lattice

import (
	"context"
	"fmt"

	"golang.org/x/exp/slices"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
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
	UpsertVpcAssociation(ctx context.Context, snName string, sgIds []*string) (string, error)
	DeleteVpcAssociation(ctx context.Context, snName string) error

	CreateOrUpdate(ctx context.Context, serviceNetwork *model.ServiceNetwork) (model.ServiceNetworkStatus, error)
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
	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, snName)
	if err != nil {
		return "", err
	}

	snva, err := m.getActiveVpcAssociation(ctx, *sn.SvcNetwork.Id)
	if err != nil {
		return "", err
	}
	if snva != nil {
		// association is active
		owned, err := m.cloud.TryOwn(ctx, *snva.Arn)
		if err != nil {
			return "", err
		}
		if !owned {
			return "", services.NewConflictError("snva", snName,
				fmt.Sprintf("Found existing vpc association not owned by controller: %s", *snva.Arn))
		}
		_, err = m.updateServiceNetworkVpcAssociation(ctx, &sn.SvcNetwork, sgIds, snva.Id)
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
		resp, err := m.cloud.Lattice().CreateServiceNetworkVpcAssociationWithContext(ctx, &req)
		if err != nil {
			return "", err
		}
		switch status := aws.StringValue(resp.Status); status {
		case vpclattice.ServiceNetworkVpcAssociationStatusActive:
			return *resp.Arn, nil
		default:
			return *resp.Arn, fmt.Errorf("%w, vpc association status in %s", lattice_runtime.NewRetryError(), status)
		}
	}
}

func (m *defaultServiceNetworkManager) DeleteVpcAssociation(ctx context.Context, snName string) error {
	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, snName)
	if err != nil {
		return err
	}

	snva, err := m.getActiveVpcAssociation(ctx, *sn.SvcNetwork.Id)
	if err != nil {
		return err
	}
	if snva != nil {
		// association is active
		m.log.Debugf(ctx, "Disassociating ServiceNetwork %s from VPC", snName)

		owned, err := m.cloud.IsArnManaged(ctx, *snva.Arn)
		if err != nil {
			// TODO check for vpclattice.ErrCodeAccessDeniedException or a new error type ErrorCodeNotFoundException
			// when the api no longer responds with a 404 NotFoundException instead of either of the above.
			// ErrorCodeNotFoundException currently not part of the golang sdk for the lattice api. This a is a distinct
			// error from vpclattice.ErrCodeResourceNotFoundException.

			// In a scenario that the vpc association is created by a foreign account,
			// the owner account's controller cannot read the tags of this ServiceNetworkVpcAssociation,
			// and AccessDeniedException is expected.
			m.log.Warnf(ctx, "skipping delete vpc association, association: %s, error: %s", *snva.Arn, err)

			return nil
		}
		if !owned {
			m.log.Infof(ctx, "Association %s for %s not owned by controller, skipping deletion", *snva.Arn, snName)
			return nil
		}

		deleteServiceNetworkVpcAssociationInput := vpclattice.DeleteServiceNetworkVpcAssociationInput{
			ServiceNetworkVpcAssociationIdentifier: snva.Id,
		}
		resp, err := m.cloud.Lattice().DeleteServiceNetworkVpcAssociationWithContext(ctx, &deleteServiceNetworkVpcAssociationInput)
		if err != nil {
			m.log.Infof(ctx, "Failed to delete association %s for %s, with response %s and err %s", *snva.Arn, snName, resp, err.Error())
		}
		return lattice_runtime.NewRetryError()
	}
	return nil
}

func (m *defaultServiceNetworkManager) getActiveVpcAssociation(ctx context.Context, serviceNetworkId string) (*vpclattice.ServiceNetworkVpcAssociationSummary, error) {
	vpcLatticeSess := m.cloud.Lattice()
	associationStatusInput := vpclattice.ListServiceNetworkVpcAssociationsInput{
		ServiceNetworkIdentifier: &serviceNetworkId,
		VpcIdentifier:            &config.VpcID,
	}

	resp, err := vpcLatticeSess.ListServiceNetworkVpcAssociationsAsList(ctx, &associationStatusInput)
	if err != nil {
		return nil, err
	}
	if len(resp) == 0 {
		return nil, nil
	}

	// There can be at most one response for this
	snva := resp[0]
	if aws.StringValue(snva.Status) == vpclattice.ServiceNetworkVpcAssociationStatusActive {
		return snva, nil
	}
	m.log.Debugf(ctx, "snva %s status: %s",
		aws.StringValue(snva.Arn), aws.StringValue(snva.Status))
	switch aws.StringValue(snva.Status) {
	case vpclattice.ServiceNetworkVpcAssociationStatusActive,
		vpclattice.ServiceNetworkVpcAssociationStatusDeleteFailed,
		vpclattice.ServiceNetworkVpcAssociationStatusUpdateFailed:
		// the resource exists
		return snva, nil
	case vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed:
		// consider it does not exist
		return nil, nil
	default:
		// a mutation is in progress, try later
		return nil, lattice_runtime.NewRetryError()
	}
}

// The controller does not manage service network anymore, just having to upsert SN and SNVA for default SN setup.
// This function does not care about the association status, the caller is not supposed to wait for it.
func (m *defaultServiceNetworkManager) CreateOrUpdate(ctx context.Context, serviceNetwork *model.ServiceNetwork) (model.ServiceNetworkStatus, error) {
	// check if exists
	foundSnSummary, err := m.cloud.Lattice().FindServiceNetwork(ctx, serviceNetwork.Spec.Name)
	if err != nil && !services.IsNotFoundError(err) {
		return model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
	}

	var serviceNetworkId string
	var serviceNetworkArn string
	vpcLatticeSess := m.cloud.Lattice()
	if foundSnSummary == nil {
		m.log.Debugf(ctx, "Creating ServiceNetwork %s and tagging it with vpcId %s",
			serviceNetwork.Spec.Name, config.VpcID)

		serviceNetworkInput := vpclattice.CreateServiceNetworkInput{
			Name: &serviceNetwork.Spec.Name,
			Tags: m.cloud.DefaultTags(),
		}
		resp, err := vpcLatticeSess.CreateServiceNetworkWithContext(ctx, &serviceNetworkInput)
		if err != nil {
			return model.ServiceNetworkStatus{}, err
		}

		serviceNetworkId = aws.StringValue(resp.Id)
		serviceNetworkArn = aws.StringValue(resp.Arn)
	} else {
		m.log.Debugf(ctx, "ServiceNetwork %s exists, checking its VPC association", serviceNetwork.Spec.Name)
		serviceNetworkId = aws.StringValue(foundSnSummary.SvcNetwork.Id)
		serviceNetworkArn = aws.StringValue(foundSnSummary.SvcNetwork.Arn)

		snva, err := m.getActiveVpcAssociation(ctx, serviceNetworkId)
		if err != nil {
			return model.ServiceNetworkStatus{}, err
		}
		if snva != nil {
			m.log.Debugf(ctx, "ServiceNetwork %s already has VPC association %s",
				serviceNetwork.Spec.Name, aws.StringValue(snva.Arn))
			return model.ServiceNetworkStatus{ServiceNetworkARN: serviceNetworkArn, ServiceNetworkID: serviceNetworkId}, nil
		}
	}

	m.log.Debugf(ctx, "Creating association between ServiceNetwork %s and VPC %s", serviceNetworkId, config.VpcID)
	createServiceNetworkVpcAssociationInput := vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &serviceNetworkId,
		VpcIdentifier:            &config.VpcID,
		Tags:                     m.cloud.DefaultTags(),
	}
	_, err = vpcLatticeSess.CreateServiceNetworkVpcAssociationWithContext(ctx, &createServiceNetworkVpcAssociationInput)
	if err != nil {
		return model.ServiceNetworkStatus{}, err
	}
	return model.ServiceNetworkStatus{ServiceNetworkARN: serviceNetworkArn, ServiceNetworkID: serviceNetworkId}, nil
}

func (m *defaultServiceNetworkManager) updateServiceNetworkVpcAssociation(ctx context.Context, existingSN *vpclattice.ServiceNetworkSummary, sgIds []*string, existingSnvaId *string) (model.ServiceNetworkStatus, error) {
	snva, err := m.cloud.Lattice().GetServiceNetworkVpcAssociationWithContext(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
		ServiceNetworkVpcAssociationIdentifier: existingSnvaId,
	})
	if err != nil {
		return model.ServiceNetworkStatus{}, err
	}
	sgIdsEqual := securityGroupIdsEqual(sgIds, snva.SecurityGroupIds)
	if sgIdsEqual {
		// desiredSN's security group ids are same with snva's security group ids, don't need to update
		return model.ServiceNetworkStatus{
			ServiceNetworkID:     *existingSN.Id,
			ServiceNetworkARN:    *existingSN.Arn,
			SnvaSecurityGroupIds: snva.SecurityGroupIds,
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
		return model.ServiceNetworkStatus{}, fmt.Errorf("%w, update snva status: %s", lattice_runtime.NewRetryError(), *updateSnvaResp.Status)
	}
}

func securityGroupIdsEqual(arr1, arr2 []*string) bool {
	ids1 := utils.SliceMap(arr1, aws.StringValue)
	slices.Sort(ids1)
	ids2 := utils.SliceMap(arr2, aws.StringValue)
	slices.Sort(ids2)
	return slices.Equal(ids1, ids2)
}
