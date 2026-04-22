package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"

	an_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

//go:generate mockgen -destination access_log_subscription_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice AccessLogSubscriptionManager

type AccessLogSubscriptionManager interface {
	Create(ctx context.Context, accessLogSubscription *lattice.AccessLogSubscription) (*lattice.AccessLogSubscriptionStatus, error)
	Update(ctx context.Context, accessLogSubscription *lattice.AccessLogSubscription) (*lattice.AccessLogSubscriptionStatus, error)
	Delete(ctx context.Context, accessLogSubscriptionArn string) error
}

type defaultAccessLogSubscriptionManager struct {
	log   gwlog.Logger
	cloud an_aws.Cloud
}

func NewAccessLogSubscriptionManager(
	log gwlog.Logger,
	cloud an_aws.Cloud,
) *defaultAccessLogSubscriptionManager {
	return &defaultAccessLogSubscriptionManager{
		log:   log,
		cloud: cloud,
	}
}

func (m *defaultAccessLogSubscriptionManager) Create(
	ctx context.Context,
	accessLogSubscription *lattice.AccessLogSubscription,
) (*lattice.AccessLogSubscriptionStatus, error) {
	vpcLatticeSess := m.cloud.Lattice()

	sourceArn, err := m.getSourceArn(ctx, accessLogSubscription.Spec.SourceType, accessLogSubscription.Spec.SourceName)
	if err != nil {
		return nil, err
	}

	tags := m.cloud.DefaultTagsMergedWith(services.Tags{
		lattice.AccessLogPolicyTagKey: accessLogSubscription.Spec.ALPNamespacedName.String(),
	})

	tags = m.cloud.MergeTags(tags, accessLogSubscription.Spec.AdditionalTags)

	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: sourceArn,
		DestinationArn:     &accessLogSubscription.Spec.DestinationArn,
		Tags:               tags,
	}

	createALSOutput, err := vpcLatticeSess.CreateAccessLogSubscription(ctx, createALSInput)
	if err == nil {
		return &lattice.AccessLogSubscriptionStatus{
			Arn: *createALSOutput.Arn,
		}, nil
	}

	var ade *types.AccessDeniedException
	var rnfe *types.ResourceNotFoundException
	var ce *types.ConflictException

	switch {
	case errors.As(err, &ade):
		return nil, services.NewInvalidError(aws.ToString(ade.Message))
	case errors.As(err, &rnfe):
		if aws.ToString(rnfe.ResourceType) == "SERVICE_NETWORK" || aws.ToString(rnfe.ResourceType) == "SERVICE" {
			return nil, services.NewNotFoundError(string(accessLogSubscription.Spec.SourceType), accessLogSubscription.Spec.SourceName)
		}
		return nil, services.NewInvalidError(aws.ToString(rnfe.Message))
	case errors.As(err, &ce):
		/*
		 * Conflict may arise if we retry creation due to a failure elsewhere in the controller,
		 * so we check if the conflicting ALS was created for the same ALP via its tags.
		 * If it is the same ALP, return success. Else, return ConflictError.
		 */
		listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
			ResourceIdentifier: sourceArn,
		}
		listALSOutput, err := vpcLatticeSess.ListAccessLogSubscriptions(ctx, listALSInput)
		if err != nil {
			return nil, err
		}
		for _, als := range listALSOutput.Items {
			if *als.DestinationArn == accessLogSubscription.Spec.DestinationArn {
				listTagsInput := &vpclattice.ListTagsForResourceInput{
					ResourceArn: als.Arn,
				}
				listTagsOutput, err := vpcLatticeSess.ListTagsForResource(ctx, listTagsInput)
				if err != nil {
					return nil, err
				}
				value, exists := listTagsOutput.Tags[lattice.AccessLogPolicyTagKey]
				if exists && value == accessLogSubscription.Spec.ALPNamespacedName.String() {
					return &lattice.AccessLogSubscriptionStatus{
						Arn: *als.Arn,
					}, nil
				}
			}
		}
		return nil, services.NewConflictError(
			string(accessLogSubscription.Spec.SourceType),
			accessLogSubscription.Spec.SourceName,
			aws.ToString(ce.Message),
		)
	default:
		return nil, err
	}
}

func (m *defaultAccessLogSubscriptionManager) Update(
	ctx context.Context,
	accessLogSubscription *lattice.AccessLogSubscription,
) (*lattice.AccessLogSubscriptionStatus, error) {
	vpcLatticeSess := m.cloud.Lattice()

	// If the source is modified, we need to replace the ALS
	getALSInput := &vpclattice.GetAccessLogSubscriptionInput{
		AccessLogSubscriptionIdentifier: aws.String(accessLogSubscription.Status.Arn),
	}
	getALSOutput, err := vpcLatticeSess.GetAccessLogSubscription(ctx, getALSInput)
	if err != nil {
		var ade2 *types.AccessDeniedException
		var rnfe2 *types.ResourceNotFoundException
		switch {
		case errors.As(err, &ade2):
			return nil, services.NewInvalidError(aws.ToString(ade2.Message))
		case errors.As(err, &rnfe2):
			return m.Create(ctx, accessLogSubscription)
		default:
			return nil, err
		}
	}
	sourceArn, err := m.getSourceArn(ctx, accessLogSubscription.Spec.SourceType, accessLogSubscription.Spec.SourceName)
	if err != nil {
		return nil, err
	}
	if *getALSOutput.ResourceArn != *sourceArn {
		return m.replaceAccessLogSubscription(ctx, accessLogSubscription)
	}

	// Source is not modified, try to update destinationArn in the existing ALS
	updateALSInput := &vpclattice.UpdateAccessLogSubscriptionInput{
		AccessLogSubscriptionIdentifier: aws.String(accessLogSubscription.Status.Arn),
		DestinationArn:                  aws.String(accessLogSubscription.Spec.DestinationArn),
	}
	updateALSOutput, err := vpcLatticeSess.UpdateAccessLogSubscription(ctx, updateALSInput)
	if err == nil {
		err = m.cloud.Tagging().UpdateTags(ctx, *updateALSOutput.Arn, accessLogSubscription.Spec.AdditionalTags, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to update tags for access log subscription %s: %w", *updateALSOutput.Arn, err)
		}

		return &lattice.AccessLogSubscriptionStatus{
			Arn: *updateALSOutput.Arn,
		}, nil
	}

	var ade3 *types.AccessDeniedException
	var rnfe3 *types.ResourceNotFoundException
	var ce3 *types.ConflictException
	switch {
	case errors.As(err, &ade3):
		return nil, services.NewInvalidError(aws.ToString(ade3.Message))
	case errors.As(err, &rnfe3):
		if aws.ToString(rnfe3.ResourceType) == "SERVICE_NETWORK" || aws.ToString(rnfe3.ResourceType) == "SERVICE" {
			return nil, services.NewNotFoundError(string(accessLogSubscription.Spec.SourceType), accessLogSubscription.Spec.SourceName)
		}
		return m.Create(ctx, accessLogSubscription)
	case errors.As(err, &ce3):
		return m.replaceAccessLogSubscription(ctx, accessLogSubscription)
	default:
		return nil, err
	}
}

func (m *defaultAccessLogSubscriptionManager) Delete(
	ctx context.Context,
	accessLogSubscriptionArn string,
) error {
	vpcLatticeSess := m.cloud.Lattice()
	deleteALSInput := &vpclattice.DeleteAccessLogSubscriptionInput{
		AccessLogSubscriptionIdentifier: aws.String(accessLogSubscriptionArn),
	}
	_, err := vpcLatticeSess.DeleteAccessLogSubscription(ctx, deleteALSInput)
	if err != nil {
		var rnfe *types.ResourceNotFoundException
		if !errors.As(err, &rnfe) {
			return err
		}
	}
	return nil
}

func (m *defaultAccessLogSubscriptionManager) getSourceArn(
	ctx context.Context,
	sourceType lattice.SourceType,
	sourceName string,
) (*string, error) {
	vpcLatticeSess := m.cloud.Lattice()

	switch sourceType {
	case lattice.ServiceNetworkSourceType:
		serviceNetwork, err := vpcLatticeSess.FindServiceNetwork(ctx, sourceName)
		if err != nil {
			return nil, err
		}
		return serviceNetwork.SvcNetwork.Arn, nil
	case lattice.ServiceSourceType:
		service, err := vpcLatticeSess.FindService(ctx, sourceName)
		if err != nil {
			return nil, err
		}
		return service.Arn, nil
	default:
		return nil, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

func (m *defaultAccessLogSubscriptionManager) replaceAccessLogSubscription(
	ctx context.Context,
	accessLogSubscription *lattice.AccessLogSubscription,
) (*lattice.AccessLogSubscriptionStatus, error) {
	newAlsStatus, err := m.Create(ctx, accessLogSubscription)
	if err != nil {
		return nil, err
	}
	err = m.Delete(ctx, accessLogSubscription.Status.Arn)
	if err != nil {
		return nil, err
	}
	return newAlsStatus, nil
}
