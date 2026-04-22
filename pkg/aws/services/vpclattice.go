package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/aws/smithy-go"
	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

//go:generate mockgen -destination vpclattice_mocks.go -package services github.com/aws/aws-application-networking-k8s/pkg/aws/services Lattice

var (
	ErrNameConflict = errors.New("name conflict")
	ErrNotFound     = errors.New("not found")
	ErrInternal     = errors.New("internal error")
)

type Tags = map[string]string

type ServiceNetworkInfo struct {
	SvcNetwork types.ServiceNetworkSummary
	Tags       Tags
}

func NewNotFoundError(resourceType string, name string) error {
	return fmt.Errorf("%w, %s %s", ErrNotFound, resourceType, name)
}

func IsNotFoundError(err error) bool {
	var nfe *types.ResourceNotFoundException
	if errors.As(err, &nfe) {
		return true
	}
	return errors.Is(err, ErrNotFound)
}

func IgnoreNotFound(err error) error {
	if IsNotFoundError(err) {
		return nil
	}
	return err
}

type ConflictError struct {
	ResourceType string
	Name         string
	Message      string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s %s had a conflict: %s", e.ResourceType, e.Name, e.Message)
}

func NewConflictError(resourceType string, name string, message string) error {
	return &ConflictError{resourceType, name, message}
}

func IsConflictError(err error) bool {
	conflictErr := &ConflictError{}
	return errors.As(err, &conflictErr)
}

type InvalidError struct {
	Message string
}

func (e *InvalidError) Error() string {
	return fmt.Sprintf("Invalid input: %s", e.Message)
}

func NewInvalidError(message string) error {
	return &InvalidError{message}
}

func IsInvalidError(err error) bool {
	invalidErr := &InvalidError{}
	return errors.As(err, &invalidErr)
}

// Lattice defines the VPC Lattice API methods used by this controller.
type Lattice interface {
	// Service operations
	CreateService(ctx context.Context, input *vpclattice.CreateServiceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateServiceOutput, error)
	GetService(ctx context.Context, input *vpclattice.GetServiceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetServiceOutput, error)
	UpdateService(ctx context.Context, input *vpclattice.UpdateServiceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateServiceOutput, error)
	DeleteService(ctx context.Context, input *vpclattice.DeleteServiceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteServiceOutput, error)

	// Listener operations
	CreateListener(ctx context.Context, input *vpclattice.CreateListenerInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateListenerOutput, error)
	UpdateListener(ctx context.Context, input *vpclattice.UpdateListenerInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateListenerOutput, error)
	DeleteListener(ctx context.Context, input *vpclattice.DeleteListenerInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteListenerOutput, error)
	GetListener(ctx context.Context, input *vpclattice.GetListenerInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetListenerOutput, error)
	ListListeners(ctx context.Context, input *vpclattice.ListListenersInput, optFns ...func(*vpclattice.Options)) (*vpclattice.ListListenersOutput, error)

	// Rule operations
	CreateRule(ctx context.Context, input *vpclattice.CreateRuleInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateRuleOutput, error)
	UpdateRule(ctx context.Context, input *vpclattice.UpdateRuleInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateRuleOutput, error)
	DeleteRule(ctx context.Context, input *vpclattice.DeleteRuleInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteRuleOutput, error)
	GetRule(ctx context.Context, input *vpclattice.GetRuleInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetRuleOutput, error)
	BatchUpdateRule(ctx context.Context, input *vpclattice.BatchUpdateRuleInput, optFns ...func(*vpclattice.Options)) (*vpclattice.BatchUpdateRuleOutput, error)

	// Target group operations
	GetTargetGroup(ctx context.Context, input *vpclattice.GetTargetGroupInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetTargetGroupOutput, error)
	UpdateTargetGroup(ctx context.Context, input *vpclattice.UpdateTargetGroupInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateTargetGroupOutput, error)
	RegisterTargets(ctx context.Context, input *vpclattice.RegisterTargetsInput, optFns ...func(*vpclattice.Options)) (*vpclattice.RegisterTargetsOutput, error)
	DeregisterTargets(ctx context.Context, input *vpclattice.DeregisterTargetsInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeregisterTargetsOutput, error)

	// Service network operations
	CreateServiceNetwork(ctx context.Context, input *vpclattice.CreateServiceNetworkInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateServiceNetworkOutput, error)
	GetServiceNetwork(ctx context.Context, input *vpclattice.GetServiceNetworkInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetServiceNetworkOutput, error)
	UpdateServiceNetwork(ctx context.Context, input *vpclattice.UpdateServiceNetworkInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateServiceNetworkOutput, error)
	CreateServiceNetworkServiceAssociation(ctx context.Context, input *vpclattice.CreateServiceNetworkServiceAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateServiceNetworkServiceAssociationOutput, error)
	GetServiceNetworkServiceAssociation(ctx context.Context, input *vpclattice.GetServiceNetworkServiceAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetServiceNetworkServiceAssociationOutput, error)
	DeleteServiceNetworkServiceAssociation(ctx context.Context, input *vpclattice.DeleteServiceNetworkServiceAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteServiceNetworkServiceAssociationOutput, error)
	DeleteServiceNetwork(ctx context.Context, input *vpclattice.DeleteServiceNetworkInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteServiceNetworkOutput, error)
	CreateServiceNetworkVpcAssociation(ctx context.Context, input *vpclattice.CreateServiceNetworkVpcAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateServiceNetworkVpcAssociationOutput, error)
	DeleteServiceNetworkVpcAssociation(ctx context.Context, input *vpclattice.DeleteServiceNetworkVpcAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteServiceNetworkVpcAssociationOutput, error)
	UpdateServiceNetworkVpcAssociation(ctx context.Context, input *vpclattice.UpdateServiceNetworkVpcAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateServiceNetworkVpcAssociationOutput, error)
	GetServiceNetworkVpcAssociation(ctx context.Context, input *vpclattice.GetServiceNetworkVpcAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetServiceNetworkVpcAssociationOutput, error)

	// Tag operations
	ListTagsForResource(ctx context.Context, input *vpclattice.ListTagsForResourceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.ListTagsForResourceOutput, error)
	TagResource(ctx context.Context, input *vpclattice.TagResourceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.TagResourceOutput, error)
	UntagResource(ctx context.Context, input *vpclattice.UntagResourceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UntagResourceOutput, error)

	// Auth policy operations
	GetAuthPolicy(ctx context.Context, input *vpclattice.GetAuthPolicyInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetAuthPolicyOutput, error)
	PutAuthPolicy(ctx context.Context, input *vpclattice.PutAuthPolicyInput, optFns ...func(*vpclattice.Options)) (*vpclattice.PutAuthPolicyOutput, error)
	DeleteAuthPolicy(ctx context.Context, input *vpclattice.DeleteAuthPolicyInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteAuthPolicyOutput, error)

	// Access log subscription (used by access_log_subscription_manager via synthesizer)
	CreateAccessLogSubscription(ctx context.Context, input *vpclattice.CreateAccessLogSubscriptionInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateAccessLogSubscriptionOutput, error)
	UpdateAccessLogSubscription(ctx context.Context, input *vpclattice.UpdateAccessLogSubscriptionInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateAccessLogSubscriptionOutput, error)
	DeleteAccessLogSubscription(ctx context.Context, input *vpclattice.DeleteAccessLogSubscriptionInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteAccessLogSubscriptionOutput, error)
	ListAccessLogSubscriptions(ctx context.Context, input *vpclattice.ListAccessLogSubscriptionsInput, optFns ...func(*vpclattice.Options)) (*vpclattice.ListAccessLogSubscriptionsOutput, error)
	GetAccessLogSubscription(ctx context.Context, input *vpclattice.GetAccessLogSubscriptionInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetAccessLogSubscriptionOutput, error)

	// Target group CRUD (used by target_group_manager)
	CreateTargetGroup(ctx context.Context, input *vpclattice.CreateTargetGroupInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateTargetGroupOutput, error)
	DeleteTargetGroup(ctx context.Context, input *vpclattice.DeleteTargetGroupInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteTargetGroupOutput, error)

	// Custom helper methods
	ListListenersAsList(ctx context.Context, input *vpclattice.ListListenersInput) ([]types.ListenerSummary, error)
	GetRulesAsList(ctx context.Context, input *vpclattice.ListRulesInput) ([]*vpclattice.GetRuleOutput, error)
	ListRulesAsList(ctx context.Context, input *vpclattice.ListRulesInput) ([]types.RuleSummary, error)
	ListServiceNetworksAsList(ctx context.Context, input *vpclattice.ListServiceNetworksInput) ([]types.ServiceNetworkSummary, error)
	ListServicesAsList(ctx context.Context, input *vpclattice.ListServicesInput) ([]types.ServiceSummary, error)
	ListTargetGroupsAsList(ctx context.Context, input *vpclattice.ListTargetGroupsInput) ([]types.TargetGroupSummary, error)
	ListTargetsAsList(ctx context.Context, input *vpclattice.ListTargetsInput) ([]types.TargetSummary, error)
	ListServiceNetworkVpcAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkVpcAssociationsInput) ([]types.ServiceNetworkVpcAssociationSummary, error)
	ListServiceNetworkServiceAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkServiceAssociationsInput) ([]types.ServiceNetworkServiceAssociationSummary, error)
	FindServiceNetwork(ctx context.Context, nameOrId string) (*ServiceNetworkInfo, error)
	FindService(ctx context.Context, latticeServiceName string) (*types.ServiceSummary, error)
}

type defaultLattice struct {
	client     *vpclattice.Client
	ownAccount string
	cache      *expirable.LRU[string, any]
}

func NewDefaultLattice(cfg aws.Config, acc string, region string) *defaultLattice {
	latticeEndpoint := "https://vpc-lattice." + region + ".amazonaws.com"
	endpoint := os.Getenv("LATTICE_ENDPOINT")

	if endpoint == "" {
		endpoint = latticeEndpoint
	}

	client := vpclattice.NewFromConfig(cfg, func(o *vpclattice.Options) {
		o.Region = region
		o.BaseEndpoint = &endpoint
		o.RetryMaxAttempts = 20
	})

	cache := expirable.NewLRU[string, any](1000, nil, time.Second*60)

	return &defaultLattice{
		client:     client,
		ownAccount: acc,
		cache:      cache,
	}
}

// Forward all direct SDK operations to the underlying client.
func (d *defaultLattice) CreateService(ctx context.Context, input *vpclattice.CreateServiceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateServiceOutput, error) {
	return d.client.CreateService(ctx, input, optFns...)
}

func (d *defaultLattice) GetService(ctx context.Context, input *vpclattice.GetServiceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetServiceOutput, error) {
	return d.client.GetService(ctx, input, optFns...)
}
func (d *defaultLattice) UpdateService(ctx context.Context, input *vpclattice.UpdateServiceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateServiceOutput, error) {
	return d.client.UpdateService(ctx, input, optFns...)
}
func (d *defaultLattice) DeleteService(ctx context.Context, input *vpclattice.DeleteServiceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteServiceOutput, error) {
	return d.client.DeleteService(ctx, input, optFns...)
}
func (d *defaultLattice) CreateListener(ctx context.Context, input *vpclattice.CreateListenerInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateListenerOutput, error) {
	return d.client.CreateListener(ctx, input, optFns...)
}
func (d *defaultLattice) UpdateListener(ctx context.Context, input *vpclattice.UpdateListenerInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateListenerOutput, error) {
	return d.client.UpdateListener(ctx, input, optFns...)
}
func (d *defaultLattice) DeleteListener(ctx context.Context, input *vpclattice.DeleteListenerInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteListenerOutput, error) {
	return d.client.DeleteListener(ctx, input, optFns...)
}
func (d *defaultLattice) GetListener(ctx context.Context, input *vpclattice.GetListenerInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetListenerOutput, error) {
	return d.client.GetListener(ctx, input, optFns...)
}
func (d *defaultLattice) ListListeners(ctx context.Context, input *vpclattice.ListListenersInput, optFns ...func(*vpclattice.Options)) (*vpclattice.ListListenersOutput, error) {
	return d.client.ListListeners(ctx, input, optFns...)
}
func (d *defaultLattice) CreateRule(ctx context.Context, input *vpclattice.CreateRuleInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateRuleOutput, error) {
	return d.client.CreateRule(ctx, input, optFns...)
}
func (d *defaultLattice) UpdateRule(ctx context.Context, input *vpclattice.UpdateRuleInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateRuleOutput, error) {
	return d.client.UpdateRule(ctx, input, optFns...)
}
func (d *defaultLattice) DeleteRule(ctx context.Context, input *vpclattice.DeleteRuleInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteRuleOutput, error) {
	return d.client.DeleteRule(ctx, input, optFns...)
}
func (d *defaultLattice) GetRule(ctx context.Context, input *vpclattice.GetRuleInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetRuleOutput, error) {
	return d.client.GetRule(ctx, input, optFns...)
}
func (d *defaultLattice) BatchUpdateRule(ctx context.Context, input *vpclattice.BatchUpdateRuleInput, optFns ...func(*vpclattice.Options)) (*vpclattice.BatchUpdateRuleOutput, error) {
	return d.client.BatchUpdateRule(ctx, input, optFns...)
}
func (d *defaultLattice) GetTargetGroup(ctx context.Context, input *vpclattice.GetTargetGroupInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetTargetGroupOutput, error) {
	return d.client.GetTargetGroup(ctx, input, optFns...)
}
func (d *defaultLattice) UpdateTargetGroup(ctx context.Context, input *vpclattice.UpdateTargetGroupInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateTargetGroupOutput, error) {
	return d.client.UpdateTargetGroup(ctx, input, optFns...)
}
func (d *defaultLattice) CreateTargetGroup(ctx context.Context, input *vpclattice.CreateTargetGroupInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateTargetGroupOutput, error) {
	return d.client.CreateTargetGroup(ctx, input, optFns...)
}
func (d *defaultLattice) DeleteTargetGroup(ctx context.Context, input *vpclattice.DeleteTargetGroupInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteTargetGroupOutput, error) {
	return d.client.DeleteTargetGroup(ctx, input, optFns...)
}
func (d *defaultLattice) RegisterTargets(ctx context.Context, input *vpclattice.RegisterTargetsInput, optFns ...func(*vpclattice.Options)) (*vpclattice.RegisterTargetsOutput, error) {
	return d.client.RegisterTargets(ctx, input, optFns...)
}
func (d *defaultLattice) DeregisterTargets(ctx context.Context, input *vpclattice.DeregisterTargetsInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeregisterTargetsOutput, error) {
	return d.client.DeregisterTargets(ctx, input, optFns...)
}
func (d *defaultLattice) UpdateServiceNetwork(ctx context.Context, input *vpclattice.UpdateServiceNetworkInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateServiceNetworkOutput, error) {
	return d.client.UpdateServiceNetwork(ctx, input, optFns...)
}
func (d *defaultLattice) CreateServiceNetwork(ctx context.Context, input *vpclattice.CreateServiceNetworkInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateServiceNetworkOutput, error) {
	return d.client.CreateServiceNetwork(ctx, input, optFns...)
}

func (d *defaultLattice) GetServiceNetwork(ctx context.Context, input *vpclattice.GetServiceNetworkInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetServiceNetworkOutput, error) {
	return d.client.GetServiceNetwork(ctx, input, optFns...)
}
func (d *defaultLattice) CreateServiceNetworkServiceAssociation(ctx context.Context, input *vpclattice.CreateServiceNetworkServiceAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateServiceNetworkServiceAssociationOutput, error) {
	return d.client.CreateServiceNetworkServiceAssociation(ctx, input, optFns...)
}
func (d *defaultLattice) DeleteServiceNetworkServiceAssociation(ctx context.Context, input *vpclattice.DeleteServiceNetworkServiceAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteServiceNetworkServiceAssociationOutput, error) {
	return d.client.DeleteServiceNetworkServiceAssociation(ctx, input, optFns...)
}
func (d *defaultLattice) DeleteServiceNetwork(ctx context.Context, input *vpclattice.DeleteServiceNetworkInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteServiceNetworkOutput, error) {
	return d.client.DeleteServiceNetwork(ctx, input, optFns...)
}
func (d *defaultLattice) CreateServiceNetworkVpcAssociation(ctx context.Context, input *vpclattice.CreateServiceNetworkVpcAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateServiceNetworkVpcAssociationOutput, error) {
	return d.client.CreateServiceNetworkVpcAssociation(ctx, input, optFns...)
}
func (d *defaultLattice) DeleteServiceNetworkVpcAssociation(ctx context.Context, input *vpclattice.DeleteServiceNetworkVpcAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteServiceNetworkVpcAssociationOutput, error) {
	return d.client.DeleteServiceNetworkVpcAssociation(ctx, input, optFns...)
}
func (d *defaultLattice) UpdateServiceNetworkVpcAssociation(ctx context.Context, input *vpclattice.UpdateServiceNetworkVpcAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateServiceNetworkVpcAssociationOutput, error) {
	return d.client.UpdateServiceNetworkVpcAssociation(ctx, input, optFns...)
}
func (d *defaultLattice) GetServiceNetworkVpcAssociation(ctx context.Context, input *vpclattice.GetServiceNetworkVpcAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetServiceNetworkVpcAssociationOutput, error) {
	return d.client.GetServiceNetworkVpcAssociation(ctx, input, optFns...)
}
func (d *defaultLattice) GetServiceNetworkServiceAssociation(ctx context.Context, input *vpclattice.GetServiceNetworkServiceAssociationInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetServiceNetworkServiceAssociationOutput, error) {
	return d.client.GetServiceNetworkServiceAssociation(ctx, input, optFns...)
}
func (d *defaultLattice) GetAuthPolicy(ctx context.Context, input *vpclattice.GetAuthPolicyInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetAuthPolicyOutput, error) {
	return d.client.GetAuthPolicy(ctx, input, optFns...)
}

func (d *defaultLattice) PutAuthPolicy(ctx context.Context, input *vpclattice.PutAuthPolicyInput, optFns ...func(*vpclattice.Options)) (*vpclattice.PutAuthPolicyOutput, error) {
	return d.client.PutAuthPolicy(ctx, input, optFns...)
}
func (d *defaultLattice) DeleteAuthPolicy(ctx context.Context, input *vpclattice.DeleteAuthPolicyInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteAuthPolicyOutput, error) {
	return d.client.DeleteAuthPolicy(ctx, input, optFns...)
}
func (d *defaultLattice) CreateAccessLogSubscription(ctx context.Context, input *vpclattice.CreateAccessLogSubscriptionInput, optFns ...func(*vpclattice.Options)) (*vpclattice.CreateAccessLogSubscriptionOutput, error) {
	return d.client.CreateAccessLogSubscription(ctx, input, optFns...)
}
func (d *defaultLattice) UpdateAccessLogSubscription(ctx context.Context, input *vpclattice.UpdateAccessLogSubscriptionInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UpdateAccessLogSubscriptionOutput, error) {
	return d.client.UpdateAccessLogSubscription(ctx, input, optFns...)
}
func (d *defaultLattice) DeleteAccessLogSubscription(ctx context.Context, input *vpclattice.DeleteAccessLogSubscriptionInput, optFns ...func(*vpclattice.Options)) (*vpclattice.DeleteAccessLogSubscriptionOutput, error) {
	return d.client.DeleteAccessLogSubscription(ctx, input, optFns...)
}
func (d *defaultLattice) ListAccessLogSubscriptions(ctx context.Context, input *vpclattice.ListAccessLogSubscriptionsInput, optFns ...func(*vpclattice.Options)) (*vpclattice.ListAccessLogSubscriptionsOutput, error) {
	return d.client.ListAccessLogSubscriptions(ctx, input, optFns...)
}
func (d *defaultLattice) GetAccessLogSubscription(ctx context.Context, input *vpclattice.GetAccessLogSubscriptionInput, optFns ...func(*vpclattice.Options)) (*vpclattice.GetAccessLogSubscriptionOutput, error) {
	return d.client.GetAccessLogSubscription(ctx, input, optFns...)
}

// Tag caching
func tagCacheKey(arn string) string {
	return "tag-" + arn
}

func (d *defaultLattice) ListTagsForResource(ctx context.Context, input *vpclattice.ListTagsForResourceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.ListTagsForResourceOutput, error) {
	key := tagCacheKey(*input.ResourceArn)
	if d.cache != nil {
		if r, ok := d.cache.Get(key); ok {
			return r.(*vpclattice.ListTagsForResourceOutput), nil
		}
	}
	out, err := d.client.ListTagsForResource(ctx, input, optFns...)
	if err != nil {
		return nil, err
	}
	if d.cache != nil {
		d.cache.Add(key, out)
	}
	return out, nil
}

func (d *defaultLattice) TagResource(ctx context.Context, input *vpclattice.TagResourceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.TagResourceOutput, error) {
	if d.cache != nil {
		d.cache.Remove(tagCacheKey(*input.ResourceArn))
	}
	return d.client.TagResource(ctx, input, optFns...)
}

func (d *defaultLattice) UntagResource(ctx context.Context, input *vpclattice.UntagResourceInput, optFns ...func(*vpclattice.Options)) (*vpclattice.UntagResourceOutput, error) {
	if d.cache != nil {
		d.cache.Remove(tagCacheKey(*input.ResourceArn))
	}
	return d.client.UntagResource(ctx, input, optFns...)
}

// Paginated list helpers
func (d *defaultLattice) ListListenersAsList(ctx context.Context, input *vpclattice.ListListenersInput) ([]types.ListenerSummary, error) {
	var result []types.ListenerSummary
	paginator := vpclattice.NewListListenersPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
	}
	return result, nil
}

func (d *defaultLattice) GetRulesAsList(ctx context.Context, input *vpclattice.ListRulesInput) ([]*vpclattice.GetRuleOutput, error) {
	var result []*vpclattice.GetRuleOutput
	paginator := vpclattice.NewListRulesPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range page.Items {
			gro, err := d.client.GetRule(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  input.ServiceIdentifier,
				ListenerIdentifier: input.ListenerIdentifier,
				RuleIdentifier:     r.Id,
			})
			if err != nil {
				return nil, err
			}
			result = append(result, gro)
		}
	}
	return result, nil
}

func (d *defaultLattice) ListRulesAsList(ctx context.Context, input *vpclattice.ListRulesInput) ([]types.RuleSummary, error) {
	var result []types.RuleSummary
	paginator := vpclattice.NewListRulesPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
	}
	return result, nil
}

func (d *defaultLattice) ListServiceNetworksAsList(ctx context.Context, input *vpclattice.ListServiceNetworksInput) ([]types.ServiceNetworkSummary, error) {
	var result []types.ServiceNetworkSummary
	paginator := vpclattice.NewListServiceNetworksPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
	}
	return result, nil
}

func (d *defaultLattice) ListServicesAsList(ctx context.Context, input *vpclattice.ListServicesInput) ([]types.ServiceSummary, error) {
	var result []types.ServiceSummary
	paginator := vpclattice.NewListServicesPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
	}
	return result, nil
}

func (d *defaultLattice) ListTargetGroupsAsList(ctx context.Context, input *vpclattice.ListTargetGroupsInput) ([]types.TargetGroupSummary, error) {
	var result []types.TargetGroupSummary
	paginator := vpclattice.NewListTargetGroupsPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
	}
	return result, nil
}

func (d *defaultLattice) ListTargetsAsList(ctx context.Context, input *vpclattice.ListTargetsInput) ([]types.TargetSummary, error) {
	var result []types.TargetSummary
	paginator := vpclattice.NewListTargetsPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
	}
	return result, nil
}

func (d *defaultLattice) ListServiceNetworkVpcAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkVpcAssociationsInput) ([]types.ServiceNetworkVpcAssociationSummary, error) {
	var result []types.ServiceNetworkVpcAssociationSummary
	paginator := vpclattice.NewListServiceNetworkVpcAssociationsPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
	}
	return result, nil
}

func (d *defaultLattice) ListServiceNetworkServiceAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkServiceAssociationsInput) ([]types.ServiceNetworkServiceAssociationSummary, error) {
	var result []types.ServiceNetworkServiceAssociationSummary
	paginator := vpclattice.NewListServiceNetworkServiceAssociationsPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
	}
	return result, nil
}

// Helper methods

func (d *defaultLattice) snSummaryToLog(snSum []types.ServiceNetworkSummary) string {
	out := make([]string, len(snSum))
	for i, s := range snSum {
		out[i] = fmt.Sprintf("{name=%s, id=%s}", aws.ToString(s.Name), aws.ToString(s.Id))
	}
	return strings.Join(out, ",")
}

// Try find by name first, if there is no single match, continue with id match. Ideally name match
// should work just fine, but in desperate scenario of shared SN when naming collision happens using
// id can be an option
func (d *defaultLattice) serviceNetworkMatch(allSn []types.ServiceNetworkSummary, nameOrId string) (*types.ServiceNetworkSummary, error) {
	var snMatch *types.ServiceNetworkSummary
	nameMatch := utils.SliceFilter(allSn, func(snSum types.ServiceNetworkSummary) bool {
		return aws.ToString(snSum.Name) == nameOrId
	})
	idMatch := utils.SliceFilter(allSn, func(snSum types.ServiceNetworkSummary) bool {
		return aws.ToString(snSum.Id) == nameOrId
	})

	switch {
	case len(nameMatch) == 0 && len(idMatch) == 0:
		return nil, NewNotFoundError("Service network", nameOrId)
	case len(nameMatch)+len(idMatch) > 1:
		return nil, fmt.Errorf("%w, multiple SN found: nameMatch=%s idMatch=%s",
			ErrNameConflict, d.snSummaryToLog(nameMatch), d.snSummaryToLog(idMatch))
	case len(nameMatch) == 1:
		snMatch = &nameMatch[0]
	case len(idMatch) == 1:
		snMatch = &idMatch[0]
	default:
		return nil, fmt.Errorf("%w: service network match: unreachable", ErrInternal)
	}
	return snMatch, nil
}

// checks if given string ARN belongs to given account
func (d *defaultLattice) isLocalResource(strArn string) (bool, error) {
	a, err := arn.Parse(strArn)
	if err != nil {
		return false, err
	}
	return a.AccountID == d.ownAccount || d.ownAccount == "", nil
}

func (d *defaultLattice) FindServiceNetwork(ctx context.Context, nameOrId string) (*ServiceNetworkInfo, error) {
	// When default service network is provided, override for any kind of SN search
	if config.ServiceNetworkOverrideMode {
		nameOrId = config.DefaultServiceNetwork
	}

	// Step 1: Try to find in local (owned) service networks
	input := &vpclattice.ListServiceNetworksInput{}
	allSn, err := d.ListServiceNetworksAsList(ctx, input)
	if err != nil {
		return nil, err
	}

	snMatch, err := d.serviceNetworkMatch(allSn, nameOrId)

	// If found locally, return it with tags
	if err == nil {
		return d.buildServiceNetworkInfo(ctx, snMatch)
	}

	// If error is not "not found", return the error
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// Step 2: Not found locally, try RAM-shared networks via VPC associations
	return d.findServiceNetworkViaVPCAssociation(ctx, nameOrId)
}

// buildServiceNetworkInfo constructs ServiceNetworkInfo from a matched
// service network, attempting to fetch tags if the network is local.
func (d *defaultLattice) buildServiceNetworkInfo(ctx context.Context, snMatch *types.ServiceNetworkSummary) (*ServiceNetworkInfo, error) {
	tags := Tags{}

	// Try to fetch tags only if SN is in the same AWS account
	isLocal, err := d.isLocalResource(aws.ToString(snMatch.Arn))
	if err != nil {
		return nil, err
	}

	if isLocal {
		tagsOutput, err := d.ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{ResourceArn: snMatch.Arn})
		if err != nil {
			var ade *types.AccessDeniedException
			// In case ownAccount is not set, we can't tell if SN is foreign.
			// In this case access denied is expected.
			if !errors.As(err, &ade) {
				return nil, err
			}
			// If access denied, proceed without tags
		} else {
			tags = tagsOutput.Tags
		}
	}

	return &ServiceNetworkInfo{
		SvcNetwork: *snMatch,
		Tags:       tags,
	}, nil
}

// findServiceNetworkViaVPCAssociation attempts to find a service network
// by examining VPC associations. This is used to discover RAM-shared
// service networks that don't appear in ListServiceNetworks.
func (d *defaultLattice) findServiceNetworkViaVPCAssociation(ctx context.Context, nameOrId string) (*ServiceNetworkInfo, error) {
	// Validate that VPC ID is configured
	if config.VpcID == "" {
		return nil, fmt.Errorf("cannot discover RAM-shared service networks: CLUSTER_VPC_ID environment variable is not set")
	}

	// List all VPC-to-Service Network associations for the controller's VPC
	associations, err := d.ListServiceNetworkVpcAssociationsAsList(ctx,
		&vpclattice.ListServiceNetworkVpcAssociationsInput{
			VpcIdentifier: aws.String(config.VpcID),
		})
	if err != nil {
		return nil, fmt.Errorf("failed to list VPC associations while searching for service network %s: %w", nameOrId, err)
	}

	// Find matching service network by name or ID
	var matches []types.ServiceNetworkVpcAssociationSummary
	for _, assoc := range associations {
		// Only consider active associations
		if assoc.Status != types.ServiceNetworkVpcAssociationStatusActive {
			continue
		}

		if aws.ToString(assoc.ServiceNetworkName) == nameOrId ||
			aws.ToString(assoc.ServiceNetworkId) == nameOrId {
			matches = append(matches, assoc)
		}
	}

	switch len(matches) {
	case 0:
		return nil, NewNotFoundError("Service network", nameOrId)
	case 1:
		assoc := matches[0]
		return &ServiceNetworkInfo{
			SvcNetwork: types.ServiceNetworkSummary{
				Id:   assoc.ServiceNetworkId,
				Arn:  assoc.ServiceNetworkArn,
				Name: assoc.ServiceNetworkName,
			},
			Tags: nil, // Cannot read tags for cross-account resources
		}, nil
	default:
		// Multiple matches - this shouldn't happen but handle defensively
		return nil, fmt.Errorf("%w, multiple VPC associations found for service network %s",
			ErrNameConflict, nameOrId)
	}
}

// see utils.LatticeServiceName
func (d *defaultLattice) FindService(ctx context.Context, latticeServiceName string) (*types.ServiceSummary, error) {
	var svcMatch *types.ServiceSummary
	paginator := vpclattice.NewListServicesPaginator(d.client, &vpclattice.ListServicesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for i := range page.Items {
			if aws.ToString(page.Items[i].Name) == latticeServiceName {
				svcMatch = &page.Items[i]
				return svcMatch, nil
			}
		}
	}
	return nil, NewNotFoundError("Service", latticeServiceName)
}

func IsLatticeAPINotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	var nfe *types.ResourceNotFoundException
	if errors.As(err, &nfe) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "ResourceNotFoundException"
	}
	return false
}
