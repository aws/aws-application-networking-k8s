package services

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/mercury"
	"github.com/aws/aws-sdk-go/service/mercury/mercuryiface"
)

type Mercury interface {
	mercuryiface.MercuryAPI
	ListMeshesAsList(ctx context.Context, input *mercury.ListMeshesInput) ([]*mercury.MeshSummary, error)
	ListServicesAsList(ctx context.Context, input *mercury.ListServicesInput) ([]*mercury.ServiceSummary, error)
	ListTargetGroupsAsList(ctx context.Context, input *mercury.ListTargetGroupsInput) ([]*mercury.TargetGroupSummary, error)
	ListTargetsAsList(ctx context.Context, input *mercury.ListTargetsInput) ([]*mercury.TargetSummary, error)
	ListMeshVpcAssociationsAsList(ctx context.Context, input *mercury.ListMeshVpcAssociationsInput) ([]*mercury.MeshVpcAssociationSummary, error)
	ListMeshServiceAssociationsAsList(ctx context.Context, input *mercury.ListMeshServiceAssociationsInput) ([]*mercury.MeshServiceAssociationSummary, error)
}

type defaultMercury struct {
	mercuryiface.MercuryAPI
}

const (
	GammaEndpoint    = "https://mercury-gamma.us-west-2.amazonaws.com/"
	BetaProdEndpoint = "https://vpc-service-network.us-west-2.amazonaws.com"
)

func NewDefaultMercury(sess *session.Session, region string) *defaultMercury {
	var mercurySess mercuryiface.MercuryAPI
	if region == "us-east-1" {
		mercurySess = mercury.New(sess, aws.NewConfig().WithRegion("us-east-1").WithEndpoint("https://mercury-beta.us-east-1.amazonaws.com/"))
	} else {
		mercurySess = mercury.New(sess, aws.NewConfig().WithRegion("us-west-2").WithEndpoint(BetaProdEndpoint))
	}
	return &defaultMercury{MercuryAPI: mercurySess}
}

func (d *defaultMercury) ListMeshesAsList(ctx context.Context, input *mercury.ListMeshesInput) ([]*mercury.MeshSummary, error) {
	result := []*mercury.MeshSummary{}
	resp, err := d.ListMeshesWithContext(ctx, input)

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
		resp, err = d.ListMeshesWithContext(ctx, input)

	}
	return result, nil
}

func (d *defaultMercury) ListServicesAsList(ctx context.Context, input *mercury.ListServicesInput) ([]*mercury.ServiceSummary, error) {
	result := []*mercury.ServiceSummary{}
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

func (d *defaultMercury) ListTargetGroupsAsList(ctx context.Context, input *mercury.ListTargetGroupsInput) ([]*mercury.TargetGroupSummary, error) {
	result := []*mercury.TargetGroupSummary{}
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

func (d *defaultMercury) ListTargetsAsList(ctx context.Context, input *mercury.ListTargetsInput) ([]*mercury.TargetSummary, error) {
	result := []*mercury.TargetSummary{}
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

func (d *defaultMercury) ListMeshVpcAssociationsAsList(ctx context.Context, input *mercury.ListMeshVpcAssociationsInput) ([]*mercury.MeshVpcAssociationSummary, error) {
	result := []*mercury.MeshVpcAssociationSummary{}
	resp, err := d.ListMeshVpcAssociationsWithContext(ctx, input)

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
		resp, err = d.ListMeshVpcAssociationsWithContext(ctx, input)
	}

	return result, nil
}

func (d *defaultMercury) ListMeshServiceAssociationsAsList(ctx context.Context, input *mercury.ListMeshServiceAssociationsInput) ([]*mercury.MeshServiceAssociationSummary, error) {
	result := []*mercury.MeshServiceAssociationSummary{}
	resp, err := d.ListMeshServiceAssociationsWithContext(ctx, input)

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
		resp, err = d.ListMeshServiceAssociationsWithContext(ctx, input)
	}

	return result, nil
}
