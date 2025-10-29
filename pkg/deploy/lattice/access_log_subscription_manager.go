package lattice

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

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
		lattice.AccessLogPolicyTagKey: aws.String(accessLogSubscription.Spec.ALPNamespacedName.String()),
	})

	tags = m.cloud.MergeTags(tags, accessLogSubscription.Spec.AdditionalTags)

	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: sourceArn,
		DestinationArn:     &accessLogSubscription.Spec.DestinationArn,
		Tags:               tags,
	}

	createALSOutput, err := vpcLatticeSess.CreateAccessLogSubscriptionWithContext(ctx, createALSInput)
	if err == nil {
		return &lattice.AccessLogSubscriptionStatus{
			Arn: *createALSOutput.Arn,
		}, nil
	}

	switch e := err.(type) {
	case *vpclattice.AccessDeniedException:
		return nil, services.NewInvalidError(e.Message())
	case *vpclattice.ResourceNotFoundException:
		if *e.ResourceType == "SERVICE_NETWORK" || *e.ResourceType == "SERVICE" {
			return nil, services.NewNotFoundError(string(accessLogSubscription.Spec.SourceType), accessLogSubscription.Spec.SourceName)
		}
		return nil, services.NewInvalidError(e.Message())
	case *vpclattice.ConflictException:
		/*
		 * Conflict may arise if we retry creation due to a failure elsewhere in the controller,
		 * so we check if the conflicting ALS was created for the same ALP via its tags.
		 * If it is the same ALP, return success. Else, return ConflictError.
		 */
		listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
			ResourceIdentifier: sourceArn,
		}
		listALSOutput, err := vpcLatticeSess.ListAccessLogSubscriptionsWithContext(ctx, listALSInput)
		if err != nil {
			return nil, err
		}
		for _, als := range listALSOutput.Items {
			if *als.DestinationArn == accessLogSubscription.Spec.DestinationArn {
				listTagsInput := &vpclattice.ListTagsForResourceInput{
					ResourceArn: als.Arn,
				}
				listTagsOutput, err := vpcLatticeSess.ListTagsForResourceWithContext(ctx, listTagsInput)
				if err != nil {
					return nil, err
				}
				value, exists := listTagsOutput.Tags[lattice.AccessLogPolicyTagKey]
				if exists && *value == accessLogSubscription.Spec.ALPNamespacedName.String() {
					return &lattice.AccessLogSubscriptionStatus{
						Arn: *als.Arn,
					}, nil
				}
			}
		}
		return nil, services.NewConflictError(
			string(accessLogSubscription.Spec.SourceType),
			accessLogSubscription.Spec.SourceName,
			e.Message(),
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
	getALSOutput, err := vpcLatticeSess.GetAccessLogSubscriptionWithContext(ctx, getALSInput)
	if err != nil {
		switch e := err.(type) {
		case *vpclattice.AccessDeniedException:
			return nil, services.NewInvalidError(e.Message())
		case *vpclattice.ResourceNotFoundException:
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
	updateALSOutput, err := vpcLatticeSess.UpdateAccessLogSubscriptionWithContext(ctx, updateALSInput)
	if err == nil {
		err = m.cloud.Tagging().UpdateTags(ctx, *updateALSOutput.Arn, accessLogSubscription.Spec.AdditionalTags, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to update tags for access log subscription %s: %w", *updateALSOutput.Arn, err)
		}

		return &lattice.AccessLogSubscriptionStatus{
			Arn: *updateALSOutput.Arn,
		}, nil
	}

	switch e := err.(type) {
	case *vpclattice.AccessDeniedException:
		return nil, services.NewInvalidError(e.Message())
	case *vpclattice.ResourceNotFoundException:
		if *e.ResourceType == "SERVICE_NETWORK" || *e.ResourceType == "SERVICE" {
			return nil, services.NewNotFoundError(string(accessLogSubscription.Spec.SourceType), accessLogSubscription.Spec.SourceName)
		}
		return m.Create(ctx, accessLogSubscription)
	case *vpclattice.ConflictException:
		/*
		 * A conflict can happen when the destination type of the new ALS is different from the original.
		 * To gracefully handle this, we create a new ALS with the new destination, then delete the old one.
		 */
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
	_, err := vpcLatticeSess.DeleteAccessLogSubscriptionWithContext(ctx, deleteALSInput)
	if err != nil {
		if _, ok := err.(*vpclattice.ResourceNotFoundException); !ok {
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
