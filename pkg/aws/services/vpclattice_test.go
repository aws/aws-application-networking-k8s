package services

import (
	"context"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_defaultMercury_ListServiceNetworksAsList(t *testing.T) {
	tests := []struct {
		ctx        context.Context
		nextToken  string
		maxResults int64
		wantErr    error
		testArn    string
		testId     string
		testName   string
	}{
		{
			ctx:        context.TODO(),
			maxResults: 2,
			nextToken:  "NonNil",
			wantErr:    nil,
			testArn:    "test-arn-1234",
			testId:     "test-id-1234",
			testName:   "test-1",
		},
	}
	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		mockMercuryService := NewMockVpcLatticeAPI(c)

		d := &defaultMercury{
			VpcLatticeAPI: mockMercuryService,
		}

		input := &vpclattice.ListServiceNetworksInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}
		sampleMesh := &vpclattice.ServiceNetworkSummary{
			Arn:  &tt.testArn,
			Id:   &tt.testId,
			Name: &tt.testName,
		}
		listMeshOutput1 := &vpclattice.ListServiceNetworksOutput{
			Items:     []*vpclattice.ServiceNetworkSummary{sampleMesh, sampleMesh},
			NextToken: &tt.nextToken,
		}
		listMeshOutput2 := &vpclattice.ListServiceNetworksOutput{
			Items:     []*vpclattice.ServiceNetworkSummary{sampleMesh, sampleMesh},
			NextToken: &tt.nextToken,
		}
		listMeshOutput3 := &vpclattice.ListServiceNetworksOutput{
			Items:     []*vpclattice.ServiceNetworkSummary{sampleMesh},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListServiceNetworksWithContext(tt.ctx, input).Return(listMeshOutput1, nil)
		mockMercuryService.EXPECT().ListServiceNetworksWithContext(tt.ctx, input).Return(listMeshOutput2, nil)
		mockMercuryService.EXPECT().ListServiceNetworksWithContext(tt.ctx, input).Return(listMeshOutput3, nil)
		got, err := d.ListServiceNetworksAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*vpclattice.ServiceNetworkSummary{sampleMesh, sampleMesh, sampleMesh, sampleMesh, sampleMesh})
	}
}

func Test_defaultMercury_ListServicesAsList(t *testing.T) {
	tests := []struct {
		ctx        context.Context
		nextToken  string
		maxResults int64
		testName   string
	}{
		{
			ctx:        context.TODO(),
			maxResults: 1,
			nextToken:  "NonNil",
			testName:   "test-1",
		},
	}
	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		mockMercuryService := NewMockVpcLatticeAPI(c)

		d := &defaultMercury{
			VpcLatticeAPI: mockMercuryService,
		}

		input := &vpclattice.ListServicesInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}
		sampleService := &vpclattice.ServiceSummary{
			Name: &tt.testName,
		}
		listOutput1 := &vpclattice.ListServicesOutput{
			Items:     []*vpclattice.ServiceSummary{sampleService},
			NextToken: &tt.nextToken,
		}
		listOutput2 := &vpclattice.ListServicesOutput{
			Items:     []*vpclattice.ServiceSummary{sampleService},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListServicesWithContext(tt.ctx, input).Return(listOutput1, nil)
		mockMercuryService.EXPECT().ListServicesWithContext(tt.ctx, input).Return(listOutput2, nil)
		got, err := d.ListServicesAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*vpclattice.ServiceSummary{sampleService, sampleService})
	}
}

func Test_defaultMercury_ListTGsAsList(t *testing.T) {
	tests := []struct {
		ctx        context.Context
		nextToken  string
		maxResults int64
		testName   string
	}{
		{
			ctx:        context.TODO(),
			maxResults: 1,
			nextToken:  "",
			testName:   "test-1",
		},
	}
	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		mockMercuryService := NewMockVpcLatticeAPI(c)

		d := &defaultMercury{
			VpcLatticeAPI: mockMercuryService,
		}

		input := &vpclattice.ListTargetGroupsInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}
		sample := &vpclattice.TargetGroupSummary{
			Name: &tt.testName,
		}
		listOutput1 := &vpclattice.ListTargetGroupsOutput{
			Items:     []*vpclattice.TargetGroupSummary{sample},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListTargetGroupsWithContext(tt.ctx, input).Return(listOutput1, nil)

		got, err := d.ListTargetGroupsAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*vpclattice.TargetGroupSummary{sample})
	}
}

func Test_defaultMercury_ListTargetsAsList(t *testing.T) {
	tests := []struct {
		ctx        context.Context
		nextToken  string
		maxResults int64
		testName   string
	}{
		{
			ctx:        context.TODO(),
			maxResults: 3,
			nextToken:  "NonNil",
			testName:   "test-1",
		},
	}
	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		mockMercuryService := NewMockVpcLatticeAPI(c)

		d := &defaultMercury{
			VpcLatticeAPI: mockMercuryService,
		}

		input := &vpclattice.ListTargetsInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}
		sample := &vpclattice.TargetSummary{
			Id: &tt.testName,
		}
		listOutput1 := &vpclattice.ListTargetsOutput{
			Items:     []*vpclattice.TargetSummary{sample, sample},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListTargetsWithContext(tt.ctx, input).Return(listOutput1, nil)

		got, err := d.ListTargetsAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*vpclattice.TargetSummary{sample, sample})
	}
}

func Test_defaultMercury_ListServiceNetworkVpcAssociationsAsList(t *testing.T) {
	tests := []struct {
		ctx        context.Context
		nextToken  string
		maxResults int64
		testName   string
	}{
		{
			ctx:        context.TODO(),
			maxResults: 3,
			nextToken:  "NonNil",
			testName:   "test-1",
		},
	}
	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		mockMercuryService := NewMockVpcLatticeAPI(c)

		d := &defaultMercury{
			VpcLatticeAPI: mockMercuryService,
		}

		input := &vpclattice.ListServiceNetworkVpcAssociationsInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}
		sample := &vpclattice.ServiceNetworkVpcAssociationSummary{
			ServiceNetworkName: &tt.testName,
		}
		listOutput1 := &vpclattice.ListServiceNetworkVpcAssociationsOutput{
			Items:     []*vpclattice.ServiceNetworkVpcAssociationSummary{sample},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListServiceNetworkVpcAssociationsWithContext(tt.ctx, input).Return(listOutput1, nil)

		got, err := d.ListServiceNetworkVpcAssociationsAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*vpclattice.ServiceNetworkVpcAssociationSummary{sample})
	}
}

func Test_defaultMercury_ListServiceNetworkServiceAssociationsAsList(t *testing.T) {
	tests := []struct {
		ctx        context.Context
		nextToken  string
		maxResults int64
		testName   string
	}{
		{
			ctx:        context.TODO(),
			maxResults: 3,
			nextToken:  "NonNil",
			testName:   "test-1",
		},
	}
	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		mockMercuryService := NewMockVpcLatticeAPI(c)

		d := &defaultMercury{
			VpcLatticeAPI: mockMercuryService,
		}

		input := &vpclattice.ListServiceNetworkServiceAssociationsInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}

		listOutput1 := &vpclattice.ListServiceNetworkServiceAssociationsOutput{
			Items:     []*vpclattice.ServiceNetworkServiceAssociationSummary{},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListServiceNetworkServiceAssociationsWithContext(tt.ctx, input).Return(listOutput1, nil)

		got, err := d.ListServiceNetworkServiceAssociationsAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*vpclattice.ServiceNetworkServiceAssociationSummary{})
	}
}
