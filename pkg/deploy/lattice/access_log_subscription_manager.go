package lattice

import (
	"context"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
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
	if accessLogSubscription.Spec.SourceType == lattice.ServiceNetworkSourceType {
		serviceNetwork, err := getServiceNetworkWithName(ctx, vpcLatticeSess, accessLogSubscription.Spec.SourceName)
		if err != nil {
			return nil, err
		}
		resourceIdentifier = *serviceNetwork.Arn
	} else {
		service, err := getServiceWithName(ctx, vpcLatticeSess, accessLogSubscription.Spec.SourceName)
		if err != nil {
			return nil, err
		}
		resourceIdentifier = *service.Arn
	}

	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: &resourceIdentifier,
		DestinationArn:     &accessLogSubscription.Spec.DestinationArn,
	}

	createALSOutput, err := vpcLatticeSess.CreateAccessLogSubscriptionWithContext(ctx, createALSInput)
	if err != nil {
		if e, ok := err.(*vpclattice.ConflictException); ok {
			return nil, services.NewConflictError(string(accessLogSubscription.Spec.SourceType), accessLogSubscription.Spec.SourceName, e.Message())
		} else if e, ok := err.(*vpclattice.AccessDeniedException); ok {
			return nil, services.NewInvalidError(e.Message())
		} else if e, ok := err.(*vpclattice.ResourceNotFoundException); ok {
			if *e.ResourceType == "SERVICE_NETWORK" || *e.ResourceType == "SERVICE" {
				return nil, services.NewNotFoundError(string(accessLogSubscription.Spec.SourceType), accessLogSubscription.Spec.SourceName)
			}
			return nil, services.NewInvalidError(e.Message())
		}
		return nil, err
	}

	return &lattice.AccessLogSubscriptionStatus{
		Arn: *createALSOutput.Arn,
		Id:  *createALSOutput.Id,
	}, nil
}

func getServiceNetworkWithName(ctx context.Context, vpcLatticeSess services.Lattice, name string) (*vpclattice.ServiceNetworkSummary, error) {
	serviceNetworkSummaries, err := vpcLatticeSess.ListServiceNetworksAsList(ctx, &vpclattice.ListServiceNetworksInput{})
	if err != nil {
		return nil, err
	}
	for _, serviceNetworkSummary := range serviceNetworkSummaries {
		if *serviceNetworkSummary.Name == name {
			return serviceNetworkSummary, nil
		}
	}
	return nil, services.NewNotFoundError("ServiceNetwork", name)
}

func getServiceWithName(ctx context.Context, vpcLatticeSess services.Lattice, name string) (*vpclattice.ServiceSummary, error) {
	serviceSummaries, err := vpcLatticeSess.ListServicesAsList(ctx, &vpclattice.ListServicesInput{})
	if err != nil {
		return nil, err
	}
	for _, serviceSummary := range serviceSummaries {
		if *serviceSummary.Name == name {
			return serviceSummary, nil
		}
	}
	return nil, services.NewNotFoundError("Service", name)
}
