package lattice

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

//go:generate mockgen -destination access_log_subscription_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice AccessLogSubscriptionManager

type AccessLogSubscriptionManager interface {
	Create(ctx context.Context, accessLogSubscription *lattice.AccessLogSubscription) (*lattice.AccessLogSubscriptionStatus, error)
}

type defaultAccessLogSubscriptionManager struct {
	log   gwlog.Logger
	cloud aws.Cloud
}

func NewAccessLogSubscriptionManager(
	log gwlog.Logger,
	cloud aws.Cloud,
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

	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: &resourceIdentifier,
		DestinationArn:     &accessLogSubscription.Spec.DestinationArn,
	}

	createALSOutput, err := vpcLatticeSess.CreateAccessLogSubscriptionWithContext(ctx, createALSInput)
	if err != nil {
		switch e := err.(type) {
		case *vpclattice.ConflictException:
			return nil, services.NewConflictError(string(accessLogSubscription.Spec.SourceType), accessLogSubscription.Spec.SourceName, e.Message())
		case *vpclattice.AccessDeniedException:
			return nil, services.NewInvalidError(e.Message())
		case *vpclattice.ResourceNotFoundException:
			if *e.ResourceType == "SERVICE_NETWORK" || *e.ResourceType == "SERVICE" {
				return nil, services.NewNotFoundError(string(accessLogSubscription.Spec.SourceType), accessLogSubscription.Spec.SourceName)
			}
			return nil, services.NewInvalidError(e.Message())
		default:
			return nil, err
		}
	}

	return &lattice.AccessLogSubscriptionStatus{
		Arn: *createALSOutput.Arn,
		Id:  *createALSOutput.Id,
	}, nil
}
