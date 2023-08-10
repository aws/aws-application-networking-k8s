package services

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/aws/aws-sdk-go/service/vpclattice/vpclatticeiface"
	"github.com/golang/glog"
)

//go:generate mockgen -destination vpclattice_mocks.go -package services github.com/aws/aws-application-networking-k8s/pkg/aws/services Lattice

type Lattice interface {
	vpclatticeiface.VPCLatticeAPI
	ListServiceNetworksAsList(ctx context.Context, input *vpclattice.ListServiceNetworksInput) ([]*vpclattice.ServiceNetworkSummary, error)
	ListServicesAsList(ctx context.Context, input *vpclattice.ListServicesInput) ([]*vpclattice.ServiceSummary, error)
	ListTargetGroupsAsList(ctx context.Context, input *vpclattice.ListTargetGroupsInput) ([]*vpclattice.TargetGroupSummary, error)
	ListTargetsAsList(ctx context.Context, input *vpclattice.ListTargetsInput) ([]*vpclattice.TargetSummary, error)
	ListServiceNetworkVpcAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkVpcAssociationsInput) ([]*vpclattice.ServiceNetworkVpcAssociationSummary, error)
	ListServiceNetworkServiceAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkServiceAssociationsInput) ([]*vpclattice.ServiceNetworkServiceAssociationSummary, error)
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
	resp, err := d.ListServiceNetworksWithContext(ctx, input)

	for {
		if err != nil {
			return nil, err
		}
		for _, mesh := range resp.Items {
			result = append(result, mesh)
		}
		if resp.NextToken == nil {
			break
		}
		input.NextToken = resp.NextToken
		resp, err = d.ListServiceNetworksWithContext(ctx, input)

	}
	return result, nil
}

func (d *defaultLattice) ListServicesAsList(ctx context.Context, input *vpclattice.ListServicesInput) ([]*vpclattice.ServiceSummary, error) {
	result := []*vpclattice.ServiceSummary{}
	resp, err := d.ListServicesWithContext(ctx, input)

	for {
		if err != nil {
			return nil, err
		}
		for _, mesh := range resp.Items {
			result = append(result, mesh)
		}
		if resp.NextToken == nil {
			break
		}
		input.NextToken = resp.NextToken
		resp, err = d.ListServicesWithContext(ctx, input)
	}

	return result, nil
}

func (d *defaultLattice) ListTargetGroupsAsList(ctx context.Context, input *vpclattice.ListTargetGroupsInput) ([]*vpclattice.TargetGroupSummary, error) {
	result := []*vpclattice.TargetGroupSummary{}
	resp, err := d.ListTargetGroupsWithContext(ctx, input)

	for {
		if err != nil {
			return nil, err
		}
		for _, mesh := range resp.Items {
			result = append(result, mesh)
		}
		if resp.NextToken == nil {
			break
		}
		input.NextToken = resp.NextToken
		resp, err = d.ListTargetGroupsWithContext(ctx, input)
	}

	return result, nil
}

func (d *defaultLattice) ListTargetsAsList(ctx context.Context, input *vpclattice.ListTargetsInput) ([]*vpclattice.TargetSummary, error) {
	result := []*vpclattice.TargetSummary{}
	resp, err := d.ListTargetsWithContext(ctx, input)

	for {
		if err != nil {
			return nil, err
		}
		for _, target := range resp.Items {
			result = append(result, target)
		}
		if resp.NextToken == nil {
			break
		}
		input.NextToken = resp.NextToken
		resp, err = d.ListTargetsWithContext(ctx, input)
	}

	return result, nil
}

func (d *defaultLattice) ListServiceNetworkVpcAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkVpcAssociationsInput) ([]*vpclattice.ServiceNetworkVpcAssociationSummary, error) {
	result := []*vpclattice.ServiceNetworkVpcAssociationSummary{}
	resp, err := d.ListServiceNetworkVpcAssociationsWithContext(ctx, input)

	for {
		if err != nil {
			return nil, err
		}
		for _, mesh := range resp.Items {
			result = append(result, mesh)
		}
		if resp.NextToken == nil {
			break
		}
		input.NextToken = resp.NextToken
		resp, err = d.ListServiceNetworkVpcAssociationsWithContext(ctx, input)
	}

	return result, nil
}

func (d *defaultLattice) ListServiceNetworkServiceAssociationsAsList(ctx context.Context, input *vpclattice.ListServiceNetworkServiceAssociationsInput) ([]*vpclattice.ServiceNetworkServiceAssociationSummary, error) {
	result := []*vpclattice.ServiceNetworkServiceAssociationSummary{}
	resp, err := d.ListServiceNetworkServiceAssociationsWithContext(ctx, input)

	for {
		if err != nil {
			return nil, err
		}
		for _, mesh := range resp.Items {
			result = append(result, mesh)
		}
		if resp.NextToken == nil {
			break
		}
		input.NextToken = resp.NextToken
		resp, err = d.ListServiceNetworkServiceAssociationsWithContext(ctx, input)
	}

	return result, nil
}
