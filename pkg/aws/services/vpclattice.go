package services

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/aws/aws-sdk-go/service/vpclattice/vpclatticeiface"
	"github.com/golang/glog"
)

//go:generate mockgen -destination vpclattice_mocks.go -package services github.com/aws/aws-application-networking-k8s/pkg/aws/services Lattice

type Tags = map[string]*string

type ServiceNetworkInfo struct {
	SvcNetwork vpclattice.ServiceNetworkSummary
	Tags       Tags
}

type LatticeNameProvider interface {
	LatticeName() string
}

type NotFoundError struct {
	ResourceType string
	Name         string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %s not found", e.ResourceType, e.Name)
}

func NewNotFoundError(resourceType string, name string) error {
	return &NotFoundError{resourceType, name}
}

func IsNotFoundError(err error) bool {
	nfErr := &NotFoundError{}
	return errors.As(err, &nfErr)
}

type Lattice interface {
	vpclatticeiface.VPCLatticeAPI
	ListServiceNetworksAsList(ctx context.Context, input *vpclattice.ListServiceNetworksInput) ([]*vpclattice.ServiceNetworkSummary, error)
	ListServicesAsList(ctx context.Context, input *vpclattice.ListServicesInput) ([]*vpclattice.ServiceSummary, error)
	ListTargetGroupsAsList(ctx context.Context, input *vpclattice.ListTargetGroupsInput) ([]*vpclattice.TargetGroupSummary, error)
	ListTargetsAsList(ctx context.Context, input *vpclattice.ListTargetsInput) ([]*vpclattice.TargetSummary, error)
	ListServiceNetworkVpcAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkVpcAssociationsInput) ([]*vpclattice.ServiceNetworkVpcAssociationSummary, error)
	ListServiceNetworkServiceAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkServiceAssociationsInput) ([]*vpclattice.ServiceNetworkServiceAssociationSummary, error)
	FindServiceNetwork(ctx context.Context, name string, accountId string) (*ServiceNetworkInfo, error)
	FindService(ctx context.Context, nameProvider LatticeNameProvider) (*vpclattice.ServiceSummary, error)
}

type defaultLattice struct {
	vpclatticeiface.VPCLatticeAPI
}

func NewDefaultLattice(sess *session.Session, region string) *defaultLattice {
	var latticeSess vpclatticeiface.VPCLatticeAPI

	latticeEndpoint := "https://vpc-lattice." + region + ".amazonaws.com"
	endpoint := os.Getenv("LATTICE_ENDPOINT")

	if endpoint == "" {
		endpoint = latticeEndpoint
	}

	latticeSess = vpclattice.New(sess, aws.NewConfig().WithRegion(region).WithEndpoint(endpoint).WithMaxRetries(20))

	glog.V(2).Infoln("Lattice Service EndPoint:", endpoint)

	return &defaultLattice{latticeSess}
}

func (d *defaultLattice) ListServiceNetworksAsList(ctx context.Context, input *vpclattice.ListServiceNetworksInput) ([]*vpclattice.ServiceNetworkSummary, error) {
	result := []*vpclattice.ServiceNetworkSummary{}

	err := d.ListServiceNetworksPagesWithContext(ctx, input, func(page *vpclattice.ListServiceNetworksOutput, lastPage bool) bool {
		for _, sn := range page.Items {
			result = append(result, sn)
		}
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
		for _, svc := range page.Items {
			result = append(result, svc)
		}
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
		for _, tg := range page.Items {
			result = append(result, tg)
		}
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *defaultLattice) ListTargetsAsList(ctx context.Context, input *vpclattice.ListTargetsInput) ([]*vpclattice.TargetSummary, error) {
	result := []*vpclattice.TargetSummary{}

	err := d.ListTargetsPagesWithContext(ctx, input, func(page *vpclattice.ListTargetsOutput, lastPage bool) bool {
		for _, target := range page.Items {
			result = append(result, target)
		}
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
		for _, assoc := range page.Items {
			result = append(result, assoc)
		}
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
		for _, assoc := range page.Items {
			result = append(result, assoc)
		}
		return true
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *defaultLattice) FindServiceNetwork(ctx context.Context, name string, optionalAccountId string) (*ServiceNetworkInfo, error) {
	input := vpclattice.ListServiceNetworksInput{}

	var innerErr error
	var snMatch *ServiceNetworkInfo
	err := d.ListServiceNetworksPagesWithContext(ctx, &input, func(page *vpclattice.ListServiceNetworksOutput, lastPage bool) bool {
		for _, r := range page.Items {
			if aws.StringValue(r.Name) != name {
				continue
			}
			acctIdMatches, err1 := accountIdMatches(optionalAccountId, *r.Arn)
			if err1 != nil {
				innerErr = err1
				return false
			}
			if !acctIdMatches {
				glog.V(6).Infoln("ServiceNetwork found but does not match account id", name, r.Arn, optionalAccountId)
				continue
			}

			glog.V(6).Infoln("Found ServiceNetwork", name, r.Arn, optionalAccountId)
			tagsInput := vpclattice.ListTagsForResourceInput{
				ResourceArn: r.Arn,
			}

			tagsOutput, err2 := d.ListTagsForResourceWithContext(ctx, &tagsInput)
			if err2 != nil {
				innerErr = err2
				return false
			}

			snMatch = &ServiceNetworkInfo{
				SvcNetwork: *r,
				Tags:       tagsOutput.Tags,
			}
			return false
		}

		return true
	})

	if innerErr != nil {
		return nil, innerErr
	}
	if err != nil {
		return nil, err
	}
	if snMatch == nil {
		glog.V(6).Infoln("Service network for account not found ", name, optionalAccountId)
		return nil, NewNotFoundError("Service network", name)
	}

	return snMatch, nil
}
func (d *defaultLattice) FindService(ctx context.Context, nameProvider LatticeNameProvider) (*vpclattice.ServiceSummary, error) {
	serviceName := nameProvider.LatticeName()
	input := vpclattice.ListServicesInput{}

	var svcMatch *vpclattice.ServiceSummary
	err := d.ListServicesPagesWithContext(ctx, &input, func(page *vpclattice.ListServicesOutput, lastPage bool) bool {
		for _, svc := range page.Items {
			if *svc.Name == serviceName {
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
		glog.V(6).Infoln("Service not found", serviceName)
		return nil, NewNotFoundError("Service", serviceName)
	}

	return svcMatch, nil
}

func accountIdMatches(accountId string, itemArn string) (bool, error) {
	if accountId == "" {
		return true, nil
	}

	parsedArn, err := arn.Parse(itemArn)
	if err != nil {
		return false, err
	}

	return accountId == parsedArn.AccountID, nil
}
