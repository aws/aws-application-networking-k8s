package lattice

import (
	"context"
	"fmt"

	"slices"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

//go:generate mockgen -destination service_network_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice ServiceNetworkManager

type ServiceNetworkManager interface {
	UpsertVpcAssociation(ctx context.Context, snName string, sgIds []string, additionalTags services.Tags) (string, error)
	DeleteVpcAssociation(ctx context.Context, snName string) error

	CreateOrUpdate(ctx context.Context, serviceNetwork *model.ServiceNetwork) (model.ServiceNetworkStatus, error)

	// Upsert finds or creates a service network by name and ensures ownership.
	// Does not create a VPC association.
	Upsert(ctx context.Context, name string, additionalTags services.Tags) (model.ServiceNetworkStatus, error)

	// Delete deletes a service network by name if owned by this controller.
	Delete(ctx context.Context, snName string) error
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

func (m *defaultServiceNetworkManager) UpsertVpcAssociation(ctx context.Context, snName string, sgIds []string, additionalTags services.Tags) (string, error) {
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
		// Check if this is a RAM-shared network by examining the service network ARN
		isLocal, err := m.isLocalServiceNetwork(sn.SvcNetwork.Arn)
		if err != nil {
			return "", err
		}

		if isLocal {
			// For local networks, check ownership as before
			owned, err := m.cloud.TryOwn(ctx, *snva.Arn)
			if err != nil {
				return "", err
			}
			if !owned {
				return "", services.NewConflictError("snva", snName,
					fmt.Sprintf("Found existing vpc association not owned by controller: %s", *snva.Arn))
			}

			// Update if needed
			_, err = m.updateServiceNetworkVpcAssociation(ctx, &sn.SvcNetwork, sgIds, snva.Id, additionalTags)
			if err != nil {
				return "", err
			}
		} else {
			// For RAM-shared networks, we can't modify the association
			// Just return the existing ARN and log
			m.log.Infof(ctx, "Using existing VPC association for RAM-shared service network %s: %s",
				snName, *snva.Arn)
		}

		return *snva.Arn, nil
	} else {
		tags := m.cloud.MergeTags(m.cloud.DefaultTags(), additionalTags)

		req := vpclattice.CreateServiceNetworkVpcAssociationInput{
			ServiceNetworkIdentifier: sn.SvcNetwork.Id,
			VpcIdentifier:            &config.VpcID,
			SecurityGroupIds:         sgIds,
			Tags:                     tags,
		}
		resp, err := m.cloud.Lattice().CreateServiceNetworkVpcAssociation(ctx, &req)
		if err != nil {
			return "", err
		}
		switch status := string(resp.Status); status {
		case string(types.ServiceNetworkVpcAssociationStatusActive):
			return *resp.Arn, nil
		default:
			return *resp.Arn, fmt.Errorf("%w, vpc association status in %s", lattice_runtime.NewRetryError(), status)
		}
	}
}

// isLocalServiceNetwork determines if a service network belongs to the current AWS account
func (m *defaultServiceNetworkManager) isLocalServiceNetwork(arnStr *string) (bool, error) {
	if arnStr == nil {
		return false, fmt.Errorf("service network ARN is nil")
	}

	parsedArn, err := arn.Parse(*arnStr)
	if err != nil {
		return false, fmt.Errorf("failed to parse service network ARN %s: %w", *arnStr, err)
	}

	// Compare with controller's account
	controllerAccount := m.cloud.Config().AccountId
	if controllerAccount == "" {
		// If controller account is not set, assume it's local (backward compatibility)
		m.log.Debugf(context.Background(), "Controller account ID not set, assuming service network %s is local", *arnStr)
		return true, nil
	}

	return parsedArn.AccountID == controllerAccount, nil
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
			// TODO check for types.AccessDeniedException or a new error type ErrorCodeNotFoundException
			// when the api no longer responds with a 404 NotFoundException instead of either of the above.
			// ErrorCodeNotFoundException currently not part of the golang sdk for the lattice api. This is a distinct
			// error from types.ResourceNotFoundException.

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
		resp, err := m.cloud.Lattice().DeleteServiceNetworkVpcAssociation(ctx, &deleteServiceNetworkVpcAssociationInput)
		if err != nil {
			m.log.Infof(ctx, "Failed to delete association %s for %s, with response %v and err %s", *snva.Arn, snName, resp, err.Error())
		}
		return lattice_runtime.NewRetryError()
	}
	return nil
}

func (m *defaultServiceNetworkManager) getActiveVpcAssociation(ctx context.Context, serviceNetworkId string) (*types.ServiceNetworkVpcAssociationSummary, error) {
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
	if string(snva.Status) == string(types.ServiceNetworkVpcAssociationStatusActive) {
		return &snva, nil
	}
	m.log.Debugf(ctx, "snva %s status: %s",
		aws.ToString(snva.Arn), string(snva.Status))
	switch string(snva.Status) {
	case string(types.ServiceNetworkVpcAssociationStatusActive),
		string(types.ServiceNetworkVpcAssociationStatusDeleteFailed),
		string(types.ServiceNetworkVpcAssociationStatusUpdateFailed):
		// the resource exists
		return &snva, nil
	case string(types.ServiceNetworkVpcAssociationStatusCreateFailed):
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
		resp, err := vpcLatticeSess.CreateServiceNetwork(ctx, &serviceNetworkInput)
		if err != nil {
			return model.ServiceNetworkStatus{}, err
		}

		serviceNetworkId = aws.ToString(resp.Id)
		serviceNetworkArn = aws.ToString(resp.Arn)
	} else {
		m.log.Debugf(ctx, "ServiceNetwork %s exists, checking its VPC association", serviceNetwork.Spec.Name)
		serviceNetworkId = aws.ToString(foundSnSummary.SvcNetwork.Id)
		serviceNetworkArn = aws.ToString(foundSnSummary.SvcNetwork.Arn)

		snva, err := m.getActiveVpcAssociation(ctx, serviceNetworkId)
		if err != nil {
			return model.ServiceNetworkStatus{}, err
		}
		if snva != nil {
			m.log.Debugf(ctx, "ServiceNetwork %s already has VPC association %s",
				serviceNetwork.Spec.Name, aws.ToString(snva.Arn))
			return model.ServiceNetworkStatus{ServiceNetworkARN: serviceNetworkArn, ServiceNetworkID: serviceNetworkId}, nil
		}
	}

	m.log.Debugf(ctx, "Creating association between ServiceNetwork %s and VPC %s", serviceNetworkId, config.VpcID)
	createServiceNetworkVpcAssociationInput := vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &serviceNetworkId,
		VpcIdentifier:            &config.VpcID,
		Tags:                     m.cloud.DefaultTags(),
	}
	_, err = vpcLatticeSess.CreateServiceNetworkVpcAssociation(ctx, &createServiceNetworkVpcAssociationInput)
	if err != nil {
		return model.ServiceNetworkStatus{}, err
	}
	return model.ServiceNetworkStatus{ServiceNetworkARN: serviceNetworkArn, ServiceNetworkID: serviceNetworkId}, nil
}

func (m *defaultServiceNetworkManager) Upsert(ctx context.Context, name string, additionalTags services.Tags) (model.ServiceNetworkStatus, error) {
	allTags := m.cloud.MergeTags(m.cloud.DefaultTags(), additionalTags)

	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, name)
	if err != nil && !services.IsNotFoundError(err) {
		return model.ServiceNetworkStatus{}, err
	}

	if sn == nil {
		// SN not found, create it
		resp, err := m.cloud.Lattice().CreateServiceNetwork(ctx, &vpclattice.CreateServiceNetworkInput{
			Name: &name,
			Tags: allTags,
		})
		if err != nil {
			return model.ServiceNetworkStatus{}, err
		}
		m.log.Infof(ctx, "Created ServiceNetwork %s (id: %s)", name, aws.ToString(resp.Id))
		return model.ServiceNetworkStatus{
			ServiceNetworkARN: aws.ToString(resp.Arn),
			ServiceNetworkID:  aws.ToString(resp.Id),
		}, nil
	}

	// SN exists, adopt it
	snArn := aws.ToString(sn.SvcNetwork.Arn)
	owned, err := m.cloud.TryOwn(ctx, snArn)
	if err != nil {
		return model.ServiceNetworkStatus{}, fmt.Errorf("failed to check ownership of ServiceNetwork %s: %w", name, err)
	}
	if !owned {
		return model.ServiceNetworkStatus{}, fmt.Errorf("ServiceNetwork %s is owned by another controller", name)
	}

	m.log.Infof(ctx, "Adopted existing ServiceNetwork %s (arn: %s)", name, snArn)

	if err := m.cloud.Tagging().UpdateTags(ctx, snArn, additionalTags, m.cloud.DefaultTags()); err != nil {
		return model.ServiceNetworkStatus{}, fmt.Errorf("failed to update tags for ServiceNetwork %s: %w", name, err)
	}

	return model.ServiceNetworkStatus{
		ServiceNetworkARN: snArn,
		ServiceNetworkID:  aws.ToString(sn.SvcNetwork.Id),
	}, nil
}

func (m *defaultServiceNetworkManager) Delete(ctx context.Context, snName string) error {
	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, snName)
	if err != nil {
		if services.IsNotFoundError(err) {
			return nil // already gone
		}
		return err
	}

	snArn := aws.ToString(sn.SvcNetwork.Arn)
	owned, err := m.cloud.IsArnManaged(ctx, snArn)
	if err != nil {
		return fmt.Errorf("failed to check ownership of ServiceNetwork %s: %w", snName, err)
	}
	if !owned {
		m.log.Infof(ctx, "ServiceNetwork %s not owned by controller, skipping deletion", snName)
		return nil
	}

	_, err = m.cloud.Lattice().DeleteServiceNetwork(ctx, &vpclattice.DeleteServiceNetworkInput{
		ServiceNetworkIdentifier: sn.SvcNetwork.Id,
	})
	if err != nil {
		return err
	}

	m.log.Infof(ctx, "Deleted ServiceNetwork %s", snName)
	return nil
}

func (m *defaultServiceNetworkManager) updateServiceNetworkVpcAssociation(ctx context.Context, existingSN *types.ServiceNetworkSummary, sgIds []string, existingSnvaId *string, additionalTags services.Tags) (model.ServiceNetworkStatus, error) {
	snva, err := m.cloud.Lattice().GetServiceNetworkVpcAssociation(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
		ServiceNetworkVpcAssociationIdentifier: existingSnvaId,
	})
	if err != nil {
		return model.ServiceNetworkStatus{}, err
	}

	err = m.cloud.Tagging().UpdateTags(ctx, aws.ToString(snva.Arn), additionalTags, nil)
	if err != nil {
		return model.ServiceNetworkStatus{}, fmt.Errorf("failed to update tags for service network vpc association %s: %w", aws.ToString(snva.Id), err)
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
	updateSnvaResp, err := m.cloud.Lattice().UpdateServiceNetworkVpcAssociation(ctx, &vpclattice.UpdateServiceNetworkVpcAssociationInput{
		ServiceNetworkVpcAssociationIdentifier: existingSnvaId,
		SecurityGroupIds:                       sgIds,
	})
	if err != nil {
		return model.ServiceNetworkStatus{}, err
	}
	if string(updateSnvaResp.Status) == string(types.ServiceNetworkVpcAssociationStatusActive) {
		return model.ServiceNetworkStatus{
			ServiceNetworkID:     *existingSN.Id,
			ServiceNetworkARN:    *existingSN.Arn,
			SnvaSecurityGroupIds: updateSnvaResp.SecurityGroupIds,
		}, nil
	} else {
		return model.ServiceNetworkStatus{}, fmt.Errorf("%w, update snva status: %s", lattice_runtime.NewRetryError(), string(updateSnvaResp.Status))
	}
}

func securityGroupIdsEqual(arr1, arr2 []string) bool {
	if len(arr1) == 0 && len(arr2) == 0 {
		return true
	}
	ids1 := make([]string, len(arr1))
	copy(ids1, arr1)
	slices.Sort(ids1)
	ids2 := make([]string, len(arr2))
	copy(ids2, arr2)
	slices.Sort(ids2)
	return slices.Equal(ids1, ids2)
}
