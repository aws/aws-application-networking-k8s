package lattice

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	an_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

//go:generate mockgen -destination access_log_subscription_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice AccessLogSubscriptionManager

type AccessLogSubscriptionManager interface {
	Create(ctx context.Context, accessLogSubscription *lattice.AccessLogSubscription) (*lattice.AccessLogSubscriptionStatus, error)
	Delete(ctx context.Context, accessLogSubscription *lattice.AccessLogSubscription) error
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

	var resourceIdentifier string
	switch accessLogSubscription.Spec.SourceType {
	case lattice.ServiceNetworkSourceType:
		serviceNetwork, err := vpcLatticeSess.FindServiceNetwork(ctx, accessLogSubscription.Spec.SourceName, config.AccountID)
		if err != nil {
			return nil, err
		}
		resourceIdentifier = *serviceNetwork.SvcNetwork.Arn
	case lattice.ServiceSourceType:
		serviceNameProvider := services.NewDefaultLatticeServiceNameProvider(accessLogSubscription.Spec.SourceName)
		service, err := vpcLatticeSess.FindService(ctx, serviceNameProvider)
		if err != nil {
			return nil, err
		}
		resourceIdentifier = *service.Arn
	default:
		return nil, fmt.Errorf("unsupported source type: %s", accessLogSubscription.Spec.SourceType)
	}

	tags := m.cloud.DefaultTags()
	tags[lattice.AccessLogPolicyTagKey] = aws.String(accessLogSubscription.Spec.ALPNamespacedName.String())

	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: &resourceIdentifier,
		DestinationArn:     &accessLogSubscription.Spec.DestinationArn,
		Tags:               tags,
	}

	createALSOutput, err := vpcLatticeSess.CreateAccessLogSubscriptionWithContext(ctx, createALSInput)
	if err != nil {
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
				ResourceIdentifier: &resourceIdentifier,
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

	return &lattice.AccessLogSubscriptionStatus{
		Arn: *createALSOutput.Arn,
	}, nil
}

func (m *defaultAccessLogSubscriptionManager) Delete(
	ctx context.Context,
	accessLogSubscription *lattice.AccessLogSubscription,
) error {
	vpcLatticeSess := m.cloud.Lattice()
	deleteALSInput := &vpclattice.DeleteAccessLogSubscriptionInput{
		AccessLogSubscriptionIdentifier: aws.String(accessLogSubscription.Status.Arn),
	}
	_, err := vpcLatticeSess.DeleteAccessLogSubscriptionWithContext(ctx, deleteALSInput)
	if err != nil {
		if _, ok := err.(*vpclattice.ResourceNotFoundException); !ok {
			return err
		}
	}
	return nil
}
