package services

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func Test_IsNotFoundError(t *testing.T) {
	err := errors.New("ERROR")
	nfErr := NewNotFoundError("type", "name")
	blankNfEff := &NotFoundError{}

	assert.False(t, IsNotFoundError(err))
	assert.False(t, IsNotFoundError(nil))
	assert.True(t, IsNotFoundError(nfErr))
	assert.True(t, IsNotFoundError(blankNfEff))
}

func Test_defaultLattice_ListServiceNetworksAsList(t *testing.T) {
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
		t.Run(tt.testName, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			mockLattice := NewMockLattice(c)

			d := &defaultLattice{
				VPCLatticeAPI: mockLattice,
			}

			input := &vpclattice.ListServiceNetworksInput{
				MaxResults: &tt.maxResults,
				NextToken:  nil,
			}
			sn := &vpclattice.ServiceNetworkSummary{
				Arn:  &tt.testArn,
				Id:   &tt.testId,
				Name: &tt.testName,
			}
			listOutput1 := &vpclattice.ListServiceNetworksOutput{
				Items:     []*vpclattice.ServiceNetworkSummary{sn, sn},
				NextToken: &tt.nextToken,
			}
			listOutput2 := &vpclattice.ListServiceNetworksOutput{
				Items:     []*vpclattice.ServiceNetworkSummary{sn, sn},
				NextToken: &tt.nextToken,
			}
			listOutput3 := &vpclattice.ListServiceNetworksOutput{
				Items:     []*vpclattice.ServiceNetworkSummary{sn},
				NextToken: nil,
			}
			mockLattice.EXPECT().ListServiceNetworksPagesWithContext(tt.ctx, input, gomock.Any()).DoAndReturn(
				func(ctx aws.Context, input *vpclattice.ListServiceNetworksInput, f func(*vpclattice.ListServiceNetworksOutput, bool) bool, opts ...request.Option) error {
					cont := f(listOutput1, false)
					if cont {
						cont = f(listOutput2, false)
					}
					if cont {
						f(listOutput3, true)
					}
					return nil
				}).Times(1)

			got, err := d.ListServiceNetworksAsList(tt.ctx, input)
			assert.Nil(t, err)
			assert.Equal(t, got, []*vpclattice.ServiceNetworkSummary{sn, sn, sn, sn, sn})
		})
	}
}

func Test_defaultLattice_ListServicesAsList(t *testing.T) {
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
		t.Run(tt.testName, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			mockLattice := NewMockLattice(c)

			d := &defaultLattice{
				VPCLatticeAPI: mockLattice,
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

			mockLattice.EXPECT().ListServicesPagesWithContext(tt.ctx, input, gomock.Any()).DoAndReturn(
				func(ctx aws.Context, input *vpclattice.ListServicesInput, f func(*vpclattice.ListServicesOutput, bool) bool, opts ...request.Option) error {
					cont := f(listOutput1, false)
					if cont {
						f(listOutput2, true)
					}
					return nil
				}).Times(1)

			got, err := d.ListServicesAsList(tt.ctx, input)
			assert.Nil(t, err)
			assert.Equal(t, got, []*vpclattice.ServiceSummary{sampleService, sampleService})
		})
	}
}

func Test_defaultLattice_ListTGsAsList(t *testing.T) {
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
		t.Run(tt.testName, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			mockLattice := NewMockLattice(c)

			d := &defaultLattice{
				VPCLatticeAPI: mockLattice,
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

			mockLattice.EXPECT().ListTargetGroupsPagesWithContext(tt.ctx, input, gomock.Any()).DoAndReturn(
				func(ctx aws.Context, input *vpclattice.ListTargetGroupsInput, f func(*vpclattice.ListTargetGroupsOutput, bool) bool, opts ...request.Option) error {
					f(listOutput1, true)
					return nil
				}).Times(1)

			got, err := d.ListTargetGroupsAsList(tt.ctx, input)
			assert.Nil(t, err)
			assert.Equal(t, got, []*vpclattice.TargetGroupSummary{sample})
		})
	}
}

func Test_defaultLattice_ListTargetsAsList(t *testing.T) {
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
		t.Run(tt.testName, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			mockLattice := NewMockLattice(c)

			d := &defaultLattice{
				VPCLatticeAPI: mockLattice,
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
			mockLattice.EXPECT().ListTargetsPagesWithContext(tt.ctx, input, gomock.Any()).DoAndReturn(
				func(ctx aws.Context, input *vpclattice.ListTargetsInput, f func(*vpclattice.ListTargetsOutput, bool) bool, opts ...request.Option) error {
					f(listOutput1, true)
					return nil
				}).Times(1)

			got, err := d.ListTargetsAsList(tt.ctx, input)
			assert.Nil(t, err)
			assert.Equal(t, got, []*vpclattice.TargetSummary{sample, sample})
		})
	}
}

func Test_defaultLattice_ListServiceNetworkVpcAssociationsAsList(t *testing.T) {
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
		t.Run(tt.testName, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			mockLattice := NewMockLattice(c)

			d := &defaultLattice{
				VPCLatticeAPI: mockLattice,
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
			mockLattice.EXPECT().ListServiceNetworkVpcAssociationsPagesWithContext(tt.ctx, input, gomock.Any()).DoAndReturn(
				func(ctx aws.Context, input *vpclattice.ListServiceNetworkVpcAssociationsInput, f func(*vpclattice.ListServiceNetworkVpcAssociationsOutput, bool) bool, opts ...request.Option) error {
					f(listOutput1, true)
					return nil
				}).Times(1)

			got, err := d.ListServiceNetworkVpcAssociationsAsList(tt.ctx, input)
			assert.Nil(t, err)
			assert.Equal(t, got, []*vpclattice.ServiceNetworkVpcAssociationSummary{sample})
		})
	}
}

func Test_defaultLattice_ListServiceNetworkServiceAssociationsAsList(t *testing.T) {
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
		t.Run(tt.testName, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			mockLattice := NewMockLattice(c)

			d := &defaultLattice{
				VPCLatticeAPI: mockLattice,
			}

			input := &vpclattice.ListServiceNetworkServiceAssociationsInput{
				MaxResults: &tt.maxResults,
				NextToken:  nil,
			}

			listOutput1 := &vpclattice.ListServiceNetworkServiceAssociationsOutput{
				Items:     []*vpclattice.ServiceNetworkServiceAssociationSummary{},
				NextToken: nil,
			}
			mockLattice.EXPECT().ListServiceNetworkServiceAssociationsPagesWithContext(tt.ctx, input, gomock.Any()).DoAndReturn(
				func(ctx aws.Context, input *vpclattice.ListServiceNetworkServiceAssociationsInput, f func(*vpclattice.ListServiceNetworkServiceAssociationsOutput, bool) bool, opts ...request.Option) error {
					f(listOutput1, true)
					return nil
				}).Times(1)

			got, err := d.ListServiceNetworkServiceAssociationsAsList(tt.ctx, input)
			assert.Nil(t, err)
			assert.Equal(t, got, []*vpclattice.ServiceNetworkServiceAssociationSummary{})
		})
	}
}

func Test_defaultLattice_FindService_happyPath(t *testing.T) {
	ctx := context.TODO()
	c := gomock.NewController(t)
	mockLattice := NewMockLattice(c)
	d := &defaultLattice{VPCLatticeAPI: mockLattice}

	name := "s-name"

	item := &vpclattice.ServiceSummary{
		Name: &name,
		Arn:  aws.String("arn"),
		Id:   aws.String("id"),
	}

	listOutput1 := &vpclattice.ListServicesOutput{
		Items:     []*vpclattice.ServiceSummary{item},
		NextToken: nil,
	}

	mockLattice.EXPECT().ListServicesPagesWithContext(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx aws.Context, input *vpclattice.ListServicesInput, f func(*vpclattice.ListServicesOutput, bool) bool, opts ...request.Option) error {
			f(listOutput1, true)
			return nil
		}).AnyTimes()

	itemFound, err1 := d.FindService(ctx, name)
	assert.Nil(t, err1)
	assert.NotNil(t, itemFound)
	assert.Equal(t, name, *itemFound.Name)

	itemNotFound, err2 := d.FindService(ctx, "no-name")
	assert.True(t, IsNotFoundError(err2))
	assert.Nil(t, itemNotFound)
}

func Test_defaultLattice_FindService_pagedResults(t *testing.T) {
	ctx := context.TODO()
	c := gomock.NewController(t)
	mockLattice := NewMockLattice(c)
	d := &defaultLattice{VPCLatticeAPI: mockLattice}

	item1 := &vpclattice.ServiceSummary{
		Name: aws.String("name1"),
		Arn:  aws.String("arn1"),
		Id:   aws.String("id1"),
	}
	item2 := &vpclattice.ServiceSummary{
		Name: aws.String("name2"),
		Arn:  aws.String("arn2"),
		Id:   aws.String("id2"),
	}

	listOutput1 := &vpclattice.ListServicesOutput{
		Items:     []*vpclattice.ServiceSummary{item1},
		NextToken: aws.String("next"),
	}
	listOutput2 := &vpclattice.ListServicesOutput{
		Items:     []*vpclattice.ServiceSummary{item2},
		NextToken: nil,
	}

	mockLattice.EXPECT().ListServicesPagesWithContext(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx aws.Context, input *vpclattice.ListServicesInput, f func(*vpclattice.ListServicesOutput, bool) bool, opts ...request.Option) error {
			cont := f(listOutput1, false)
			if cont {
				f(listOutput2, true)
			}
			return nil
		}).AnyTimes()

	itemFound1, err1 := d.FindService(ctx, "name1")
	assert.Nil(t, err1)
	assert.NotNil(t, itemFound1)
	assert.Equal(t, "name1", *itemFound1.Name)

	itemFound2, err2 := d.FindService(ctx, "name2")
	assert.Nil(t, err2)
	assert.NotNil(t, itemFound2)
	assert.Equal(t, "name2", *itemFound2.Name)

	itemNotFound, err3 := d.FindService(ctx, "no-name")
	assert.True(t, IsNotFoundError(err3))
	assert.Nil(t, itemNotFound)
}

func Test_defaultLattice_FindService_errorsRaised(t *testing.T) {
	ctx := context.TODO()
	c := gomock.NewController(t)
	mockLattice := NewMockLattice(c)
	d := &defaultLattice{VPCLatticeAPI: mockLattice}

	mockLattice.EXPECT().ListServicesPagesWithContext(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("LIST_ERR")).Times(1)

	_, listErr := d.FindService(ctx, "foo")
	assert.NotNil(t, listErr)
	assert.False(t, IsNotFoundError(listErr))
}
