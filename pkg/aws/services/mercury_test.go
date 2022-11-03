package services

import (
	"context"
	"github.com/aws/aws-sdk-go/service/mercury"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_defaultMercury_ListMeshesAsList(t *testing.T) {
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
		mockMercuryService := NewMockMercuryAPI(c)

		d := &defaultMercury{
			MercuryAPI: mockMercuryService,
		}

		input := &mercury.ListMeshesInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}
		sampleMesh := &mercury.MeshSummary{
			Arn:  &tt.testArn,
			Id:   &tt.testId,
			Name: &tt.testName,
		}
		listMeshOutput1 := &mercury.ListMeshesOutput{
			Items:     []*mercury.MeshSummary{sampleMesh, sampleMesh},
			NextToken: &tt.nextToken,
		}
		listMeshOutput2 := &mercury.ListMeshesOutput{
			Items:     []*mercury.MeshSummary{sampleMesh, sampleMesh},
			NextToken: &tt.nextToken,
		}
		listMeshOutput3 := &mercury.ListMeshesOutput{
			Items:     []*mercury.MeshSummary{sampleMesh},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListMeshesWithContext(tt.ctx, input).Return(listMeshOutput1, nil)
		mockMercuryService.EXPECT().ListMeshesWithContext(tt.ctx, input).Return(listMeshOutput2, nil)
		mockMercuryService.EXPECT().ListMeshesWithContext(tt.ctx, input).Return(listMeshOutput3, nil)
		got, err := d.ListMeshesAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*mercury.MeshSummary{sampleMesh, sampleMesh, sampleMesh, sampleMesh, sampleMesh})
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
		mockMercuryService := NewMockMercuryAPI(c)

		d := &defaultMercury{
			MercuryAPI: mockMercuryService,
		}

		input := &mercury.ListServicesInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}
		sampleService := &mercury.ServiceSummary{
			Name: &tt.testName,
		}
		listOutput1 := &mercury.ListServicesOutput{
			Items:     []*mercury.ServiceSummary{sampleService},
			NextToken: &tt.nextToken,
		}
		listOutput2 := &mercury.ListServicesOutput{
			Items:     []*mercury.ServiceSummary{sampleService},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListServicesWithContext(tt.ctx, input).Return(listOutput1, nil)
		mockMercuryService.EXPECT().ListServicesWithContext(tt.ctx, input).Return(listOutput2, nil)
		got, err := d.ListServicesAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*mercury.ServiceSummary{sampleService, sampleService})
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
		mockMercuryService := NewMockMercuryAPI(c)

		d := &defaultMercury{
			MercuryAPI: mockMercuryService,
		}

		input := &mercury.ListTargetGroupsInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}
		sample := &mercury.TargetGroupSummary{
			Name: &tt.testName,
		}
		listOutput1 := &mercury.ListTargetGroupsOutput{
			Items:     []*mercury.TargetGroupSummary{sample},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListTargetGroupsWithContext(tt.ctx, input).Return(listOutput1, nil)

		got, err := d.ListTargetGroupsAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*mercury.TargetGroupSummary{sample})
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
		mockMercuryService := NewMockMercuryAPI(c)

		d := &defaultMercury{
			MercuryAPI: mockMercuryService,
		}

		input := &mercury.ListTargetsInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}
		sample := &mercury.TargetSummary{
			Id: &tt.testName,
		}
		listOutput1 := &mercury.ListTargetsOutput{
			Items:     []*mercury.TargetSummary{sample, sample},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListTargetsWithContext(tt.ctx, input).Return(listOutput1, nil)

		got, err := d.ListTargetsAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*mercury.TargetSummary{sample, sample})
	}
}

func Test_defaultMercury_ListMeshVpcAssociationsAsList(t *testing.T) {
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
		mockMercuryService := NewMockMercuryAPI(c)

		d := &defaultMercury{
			MercuryAPI: mockMercuryService,
		}

		input := &mercury.ListMeshVpcAssociationsInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}
		sample := &mercury.MeshVpcAssociationSummary{
			MeshName: &tt.testName,
		}
		listOutput1 := &mercury.ListMeshVpcAssociationsOutput{
			Items:     []*mercury.MeshVpcAssociationSummary{sample},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListMeshVpcAssociationsWithContext(tt.ctx, input).Return(listOutput1, nil)

		got, err := d.ListMeshVpcAssociationsAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*mercury.MeshVpcAssociationSummary{sample})
	}
}

func Test_defaultMercury_ListMeshServiceAssociationsAsList(t *testing.T) {
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
		mockMercuryService := NewMockMercuryAPI(c)

		d := &defaultMercury{
			MercuryAPI: mockMercuryService,
		}

		input := &mercury.ListMeshServiceAssociationsInput{
			MaxResults: &tt.maxResults,
			NextToken:  nil,
		}

		listOutput1 := &mercury.ListMeshServiceAssociationsOutput{
			Items:     []*mercury.MeshServiceAssociationSummary{},
			NextToken: nil,
		}
		mockMercuryService.EXPECT().ListMeshServiceAssociationsWithContext(tt.ctx, input).Return(listOutput1, nil)

		got, err := d.ListMeshServiceAssociationsAsList(tt.ctx, input)
		assert.Nil(t, err)
		assert.Equal(t, got, []*mercury.MeshServiceAssociationSummary{})
	}
}
