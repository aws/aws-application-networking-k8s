package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/aws/aws-sdk-go/service/vpclattice/vpclatticeiface"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

//go:generate mockgen -destination vpclattice_mocks.go -package services github.com/aws/aws-application-networking-k8s/pkg/aws/services Lattice

var (
	ErrNameConflict = errors.New("name conflict")
	ErrNotFound     = errors.New("not found")
	ErrInternal     = errors.New("internal error")
)

type ServiceNetworkInfo struct {
	SvcNetwork vpclattice.ServiceNetworkSummary
	Tags       Tags
}

func NewNotFoundError(resourceType string, name string) error {
	return fmt.Errorf("%w, %s %s", ErrNotFound, resourceType, name)
}

func IsNotFoundError(err error) bool {
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == vpclattice.ErrCodeResourceNotFoundException {
			return true
		}
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

type Lattice interface {
	vpclatticeiface.VPCLatticeAPI
	ListListenersAsList(ctx context.Context, input *vpclattice.ListListenersInput) ([]*vpclattice.ListenerSummary, error)
	GetRulesAsList(ctx context.Context, input *vpclattice.ListRulesInput) ([]*vpclattice.GetRuleOutput, error)
	ListRulesAsList(ctx context.Context, input *vpclattice.ListRulesInput) ([]*vpclattice.RuleSummary, error)
	ListServiceNetworksAsList(ctx context.Context, input *vpclattice.ListServiceNetworksInput) ([]*vpclattice.ServiceNetworkSummary, error)
	ListServicesAsList(ctx context.Context, input *vpclattice.ListServicesInput) ([]*vpclattice.ServiceSummary, error)
	ListTargetGroupsAsList(ctx context.Context, input *vpclattice.ListTargetGroupsInput) ([]*vpclattice.TargetGroupSummary, error)
	ListTargetsAsList(ctx context.Context, input *vpclattice.ListTargetsInput) ([]*vpclattice.TargetSummary, error)
	ListServiceNetworkVpcAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkVpcAssociationsInput) ([]*vpclattice.ServiceNetworkVpcAssociationSummary, error)
	ListServiceNetworkServiceAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkServiceAssociationsInput) ([]*vpclattice.ServiceNetworkServiceAssociationSummary, error)
	FindServiceNetwork(ctx context.Context, nameOrId string) (*ServiceNetworkInfo, error)
	FindService(ctx context.Context, latticeServiceName string) (*vpclattice.ServiceSummary, error)
}

type defaultLattice struct {
	vpclatticeiface.VPCLatticeAPI
	ownAccount string
	cache      *expirable.LRU[string, any]
}

func NewDefaultLattice(sess *session.Session, acc string, region string) *defaultLattice {

	latticeEndpoint := "https://vpc-lattice." + region + ".amazonaws.com"
	endpoint := os.Getenv("LATTICE_ENDPOINT")

	if endpoint == "" {
		endpoint = latticeEndpoint
	}

	latticeSess := vpclattice.New(sess, aws.NewConfig().WithRegion(region).WithEndpoint(endpoint).WithMaxRetries(20))

	cache := expirable.NewLRU[string, any](1000, nil, time.Second*60)

	return &defaultLattice{
		VPCLatticeAPI: latticeSess,
		ownAccount:    acc,
		cache:         cache,
	}
}

func (d *defaultLattice) ListListenersAsList(ctx context.Context, input *vpclattice.ListListenersInput) ([]*vpclattice.ListenerSummary, error) {
	var result []*vpclattice.ListenerSummary

	err := d.ListListenersPagesWithContext(ctx, input, func(page *vpclattice.ListListenersOutput, lastPage bool) bool {
		result = append(result, page.Items...)
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *defaultLattice) GetRulesAsList(ctx context.Context, input *vpclattice.ListRulesInput) ([]*vpclattice.GetRuleOutput, error) {
	var result []*vpclattice.GetRuleOutput

	var innerErr error
	err := d.ListRulesPagesWithContext(ctx, input, func(page *vpclattice.ListRulesOutput, lastPage bool) bool {
		for _, r := range page.Items {
			grInput := vpclattice.GetRuleInput{
				ServiceIdentifier:  input.ServiceIdentifier,
				ListenerIdentifier: input.ListenerIdentifier,
				RuleIdentifier:     r.Id,
			}

			var gro *vpclattice.GetRuleOutput
			gro, innerErr = d.GetRuleWithContext(ctx, &grInput)
			if innerErr != nil {
				return false
			}
			result = append(result, gro)
		}
		return true
	})

	if innerErr != nil {
		return nil, innerErr
	}

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *defaultLattice) ListRulesAsList(ctx context.Context, input *vpclattice.ListRulesInput) ([]*vpclattice.RuleSummary, error) {
	var result []*vpclattice.RuleSummary

	err := d.ListRulesPagesWithContext(ctx, input, func(page *vpclattice.ListRulesOutput, lastPage bool) bool {
		result = append(result, page.Items...)
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *defaultLattice) ListServiceNetworksAsList(ctx context.Context, input *vpclattice.ListServiceNetworksInput) ([]*vpclattice.ServiceNetworkSummary, error) {
	result := []*vpclattice.ServiceNetworkSummary{}

	err := d.ListServiceNetworksPagesWithContext(ctx, input, func(page *vpclattice.ListServiceNetworksOutput, lastPage bool) bool {
		result = append(result, page.Items...)
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *defaultLattice) ListServicesAsList(ctx context.Context, input *vpclattice.ListServicesInput) ([]*vpclattice.ServiceSummary, error) {
	result := []*vpclattice.ServiceSummary{}

	err := d.ListServicesPagesWithContext(ctx, input, func(page *vpclattice.ListServicesOutput, lastPage bool) bool {
		result = append(result, page.Items...)
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *defaultLattice) ListTargetGroupsAsList(ctx context.Context, input *vpclattice.ListTargetGroupsInput) ([]*vpclattice.TargetGroupSummary, error) {
	result := []*vpclattice.TargetGroupSummary{}

	err := d.ListTargetGroupsPagesWithContext(ctx, input, func(page *vpclattice.ListTargetGroupsOutput, lastPage bool) bool {
		result = append(result, page.Items...)
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *defaultLattice) ListTagsForResourceWithContext(ctx context.Context, input *vpclattice.ListTagsForResourceInput, option ...request.Option) (*vpclattice.ListTagsForResourceOutput, error) {
	key := tagCacheKey(*input.ResourceArn)
	if d.cache != nil {
		r, ok := d.cache.Get(key)
		if ok {
			return r.(*vpclattice.ListTagsForResourceOutput), nil
		}
	}
	out, err := d.VPCLatticeAPI.ListTagsForResourceWithContext(ctx, input, option...)
	if err != nil {
		return nil, err
	}
	if d.cache != nil {
		d.cache.Add(key, out)
	}
	return out, nil
}

func tagCacheKey(arn string) string {
	return "tag-" + arn
}

func (d *defaultLattice) TagResourceWithContext(ctx context.Context, input *vpclattice.TagResourceInput, option ...request.Option) (*vpclattice.TagResourceOutput, error) {
	if d.cache != nil {
		key := tagCacheKey(*input.ResourceArn)
		d.cache.Remove(key)
	}
	return d.VPCLatticeAPI.TagResourceWithContext(ctx, input, option...)
}

func (d *defaultLattice) ListTargetsAsList(ctx context.Context, input *vpclattice.ListTargetsInput) ([]*vpclattice.TargetSummary, error) {
	result := []*vpclattice.TargetSummary{}

	err := d.ListTargetsPagesWithContext(ctx, input, func(page *vpclattice.ListTargetsOutput, lastPage bool) bool {
		result = append(result, page.Items...)
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *defaultLattice) ListServiceNetworkVpcAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkVpcAssociationsInput) ([]*vpclattice.ServiceNetworkVpcAssociationSummary, error) {
	result := []*vpclattice.ServiceNetworkVpcAssociationSummary{}

	err := d.ListServiceNetworkVpcAssociationsPagesWithContext(ctx, input, func(page *vpclattice.ListServiceNetworkVpcAssociationsOutput, lastPage bool) bool {
		result = append(result, page.Items...)
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *defaultLattice) ListServiceNetworkServiceAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkServiceAssociationsInput) ([]*vpclattice.ServiceNetworkServiceAssociationSummary, error) {
	result := []*vpclattice.ServiceNetworkServiceAssociationSummary{}

	err := d.ListServiceNetworkServiceAssociationsPagesWithContext(ctx, input, func(page *vpclattice.ListServiceNetworkServiceAssociationsOutput, lastPage bool) bool {
		result = append(result, page.Items...)
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *defaultLattice) snSummaryToLog(snSum []*vpclattice.ServiceNetworkSummary) string {
	out := make([]string, len(snSum))
	for i, s := range snSum {
		out[i] = fmt.Sprintf("{name=%s, id=%s}", aws.StringValue(s.Name), aws.StringValue(s.Id))
	}
	return strings.Join(out, ",")
}

// Try find by name first, if there is no single match, continue with id match. Ideally name match
// should work just fine, but in desperate scenario of shared SN when naming collision happens using
// id can be an option
func (d *defaultLattice) serviceNetworkMatch(allSn []*vpclattice.ServiceNetworkSummary, nameOrId string) (*vpclattice.ServiceNetworkSummary, error) {
	var snMatch *vpclattice.ServiceNetworkSummary
	nameMatch := utils.SliceFilter(allSn, func(snSum *vpclattice.ServiceNetworkSummary) bool {
		return aws.StringValue(snSum.Name) == nameOrId
	})
	idMatch := utils.SliceFilter(allSn, func(snSum *vpclattice.ServiceNetworkSummary) bool {
		return aws.StringValue(snSum.Id) == nameOrId
	})

	switch {
	case len(nameMatch) == 0 && len(idMatch) == 0:
		return nil, NewNotFoundError("Service network", nameOrId)
	case len(nameMatch)+len(idMatch) > 1:
		return nil, fmt.Errorf("%w, multiple SN found: nameMatch=%s idMatch=%s",
			ErrNameConflict, d.snSummaryToLog(nameMatch), d.snSummaryToLog(idMatch))
	case len(nameMatch) == 1:
		snMatch = nameMatch[0]
	case len(idMatch) == 1:
		snMatch = idMatch[0]
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

	input := &vpclattice.ListServiceNetworksInput{}
	allSn, err := d.ListServiceNetworksAsList(ctx, input)
	if err != nil {
		return nil, err
	}

	snMatch, err := d.serviceNetworkMatch(allSn, nameOrId)
	if err != nil {
		return nil, err
	}

	// try to fetch tags only if SN in the same aws account with controller's config
	tags := Tags{}
	isLocal, err := d.isLocalResource(aws.StringValue(snMatch.Arn))
	if err != nil {
		return nil, err
	}
	if isLocal {
		tagsInput := vpclattice.ListTagsForResourceInput{ResourceArn: snMatch.Arn}
		tagsOutput, err := d.ListTagsForResourceWithContext(ctx, &tagsInput)
		if err != nil {
			aerr, ok := err.(awserr.Error)
			// In case ownAccount is not set, we cant tell if SN is foreign.
			// In this case access denied is expected.
			if !ok || aerr.Code() != vpclattice.ErrCodeAccessDeniedException {
				return nil, err
			}
		}
		tags = tagsOutput.Tags
	}

	return &ServiceNetworkInfo{
		SvcNetwork: *snMatch,
		Tags:       tags,
	}, nil
}

// see utils.LatticeServiceName
func (d *defaultLattice) FindService(ctx context.Context, latticeServiceName string) (*vpclattice.ServiceSummary, error) {
	input := vpclattice.ListServicesInput{}

	var svcMatch *vpclattice.ServiceSummary
	err := d.ListServicesPagesWithContext(ctx, &input, func(page *vpclattice.ListServicesOutput, lastPage bool) bool {
		for _, svc := range page.Items {
			if *svc.Name == latticeServiceName {
				svcMatch = svc
				return false
			}
		}
		return true
	})

	if err != nil {
		return nil, err
	}
	if svcMatch == nil {
		return nil, NewNotFoundError("Service", latticeServiceName)
	}

	return svcMatch, nil
}

func IsLatticeAPINotFoundErr(err error) bool {
	if err == nil {
		return false
	}

	var aErr awserr.Error
	if errors.As(err, &aErr) {
		return aErr.Code() == vpclattice.ErrCodeResourceNotFoundException
	}
	return false
}
