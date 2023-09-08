package services

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/request"
	"testing"

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
	}
}

func getTestArn(accountId string) string {
	//arn:<partition>:vpc-lattice:<region>:<account id>:<resource-type>/<resource-id>
	return fmt.Sprintf("arn:aws:vpc-lattice:region:%s:resource/id", accountId)
}

func Test_defaultLattice_FindServiceNetwork_happyPath(t *testing.T) {
	ctx := context.TODO()
	c := gomock.NewController(t)
	mockLattice := NewMockLattice(c)
	d := &defaultLattice{VPCLatticeAPI: mockLattice}

	snName := "sn-name"
	acctId := "123456"
	testArn := getTestArn(acctId)

	item := &vpclattice.ServiceNetworkSummary{
		Name: &snName,
		Arn:  &testArn,
		Id:   aws.String("id"),
	}

	listOutput1 := &vpclattice.ListServiceNetworksOutput{
		Items:     []*vpclattice.ServiceNetworkSummary{item},
		NextToken: nil,
	}

	mockLattice.EXPECT().ListServiceNetworksPagesWithContext(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx aws.Context, input *vpclattice.ListServiceNetworksInput, f func(*vpclattice.ListServiceNetworksOutput, bool) bool, opts ...request.Option) error {
			f(listOutput1, true)
			return nil
		}).AnyTimes()
	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListTagsForResourceOutput{}, nil).AnyTimes()

	itemFound, err1 := d.FindServiceNetwork(ctx, snName, acctId)
	assert.Nil(t, err1)
	assert.NotNil(t, itemFound)
	assert.Equal(t, snName, *itemFound.SvcNetwork.Name)

	emptyAccountItemFound, err2 := d.FindServiceNetwork(ctx, snName, "")
	assert.Nil(t, err2)
	assert.NotNil(t, emptyAccountItemFound)
	assert.Equal(t, snName, *emptyAccountItemFound.SvcNetwork.Name)

	mismatchedAccountId := "555555"
	itemNotFound, err3 := d.FindServiceNetwork(ctx, snName, mismatchedAccountId)
	assert.True(t, IsNotFoundError(err3))
	assert.Nil(t, itemNotFound)
}

func Test_defaultLattice_FindServiceNetwork_disambiguateByAccount(t *testing.T) {
	ctx := context.TODO()
	c := gomock.NewController(t)
	mockLattice := NewMockLattice(c)
	d := &defaultLattice{VPCLatticeAPI: mockLattice}

	acct1 := "12345"
	acct2 := "88888"

	listOutput1 := &vpclattice.ListServiceNetworksOutput{
		Items: []*vpclattice.ServiceNetworkSummary{
			{
				Arn:  aws.String(getTestArn(acct1)),
				Name: aws.String("duplicated-name"),
				Id:   aws.String("id"),
			},
			{
				Arn:  aws.String(getTestArn(acct2)),
				Name: aws.String("duplicated-name"),
				Id:   aws.String("id"),
			},
		},
		NextToken: nil,
	}

	mockLattice.EXPECT().ListServiceNetworksPagesWithContext(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx aws.Context, input *vpclattice.ListServiceNetworksInput, f func(*vpclattice.ListServiceNetworksOutput, bool) bool, opts ...request.Option) error {
			f(listOutput1, true)
			return nil
		}).AnyTimes()

	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListTagsForResourceOutput{
			Tags: map[string]*string{
				"foo": aws.String("bar"),
			},
		}, nil).AnyTimes()

	item1, err1 := d.FindServiceNetwork(ctx, "duplicated-name", acct1)
	assert.Nil(t, err1)
	assert.NotNil(t, item1)
	arn1, _ := arn.Parse(*item1.SvcNetwork.Arn)
	assert.Equal(t, acct1, arn1.AccountID)
	// make sure tags come back too
	assert.Equal(t, "bar", *item1.Tags["foo"])

	item2, err2 := d.FindServiceNetwork(ctx, "duplicated-name", acct2)
	assert.Nil(t, err2)
	assert.NotNil(t, item2)
	arn2, _ := arn.Parse(*item2.SvcNetwork.Arn)
	assert.Equal(t, acct2, arn2.AccountID)

	// will just return the first item it finds - is NOT predictable but doesn't fail
	emptyAcctItem, err3 := d.FindServiceNetwork(ctx, "duplicated-name", "")
	assert.Nil(t, err3)
	assert.NotNil(t, emptyAcctItem)
}

func Test_defaultLattice_FindServiceNetwork_noResults(t *testing.T) {
	ctx := context.TODO()
	c := gomock.NewController(t)
	mockLattice := NewMockLattice(c)
	d := &defaultLattice{VPCLatticeAPI: mockLattice}

	mockLattice.EXPECT().ListServiceNetworksPagesWithContext(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx aws.Context, input *vpclattice.ListServiceNetworksInput, f func(*vpclattice.ListServiceNetworksOutput, bool) bool, opts ...request.Option) error {
			f(&vpclattice.ListServiceNetworksOutput{}, true)
			return nil
		}).AnyTimes()

	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListTagsForResourceOutput{}, nil).AnyTimes()

	item, err := d.FindServiceNetwork(ctx, "foo", "1234")
	assert.True(t, IsNotFoundError(err))
	assert.Nil(t, item)
}

func Test_defaultLattice_FindServiceNetwork_pagedResults(t *testing.T) {
	ctx := context.TODO()
	c := gomock.NewController(t)
	mockLattice := NewMockLattice(c)
	d := &defaultLattice{VPCLatticeAPI: mockLattice}

	one := "1"
	two := "2"
	tokens := []*string{&one, &two, nil}
	results := [][]*vpclattice.ServiceNetworkSummary{{}, {}, {}}

	for i, _ := range results {
		for j := 1; j <= 5; j++ {
			// ids will be 11 - 15, 21 - 21, 31 - 35
			id := fmt.Sprintf("%d%d", i+1, j)
			results[i] = append(results[i],
				&vpclattice.ServiceNetworkSummary{
					Arn:  aws.String(getTestArn("1111" + id)),
					Name: aws.String("name-" + id),
					Id:   aws.String("id-" + id),
				})
		}
	}

	mockLattice.EXPECT().ListServiceNetworksPagesWithContext(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx aws.Context, input *vpclattice.ListServiceNetworksInput, f func(*vpclattice.ListServiceNetworksOutput, bool) bool, opts ...request.Option) error {
			for i, result := range results {
				responsePage := &vpclattice.ListServiceNetworksOutput{
					Items:     result,
					NextToken: tokens[i],
				}
				cont := f(responsePage, i+1 == len(results))
				if !cont {
					break
				}
			}
			return nil
		}).AnyTimes()

	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListTagsForResourceOutput{}, nil).AnyTimes()

	page3item, err3 := d.FindServiceNetwork(ctx, "name-35", "111135")
	assert.Nil(t, err3)
	assert.NotNil(t, page3item)
	assert.Equal(t, "name-35", *page3item.SvcNetwork.Name)

	page1item, err1 := d.FindServiceNetwork(ctx, "name-13", "111113")
	assert.Nil(t, err1)
	assert.NotNil(t, page1item)
	assert.Equal(t, "name-13", *page1item.SvcNetwork.Name)

	page2item, err2 := d.FindServiceNetwork(ctx, "name-21", "111121")
	assert.Nil(t, err2)
	assert.NotNil(t, page2item)
	assert.Equal(t, "name-21", *page2item.SvcNetwork.Name)

	notPresentItem, err4 := d.FindServiceNetwork(ctx, "no-name", "")
	assert.True(t, IsNotFoundError(err4))
	assert.Nil(t, notPresentItem)
}

func Test_defaultLattice_FindServiceNetwork_errorsRaised(t *testing.T) {
	ctx := context.TODO()
	c := gomock.NewController(t)
	mockLattice := NewMockLattice(c)
	d := &defaultLattice{VPCLatticeAPI: mockLattice}

	mockLattice.EXPECT().ListServiceNetworksPagesWithContext(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("LIST_ERR")).Times(1)

	_, listErr := d.FindServiceNetwork(ctx, "foo", "1234")
	assert.NotNil(t, listErr)
	assert.False(t, IsNotFoundError(listErr))

	listOutput := vpclattice.ListServiceNetworksOutput{
		Items: []*vpclattice.ServiceNetworkSummary{
			{
				Arn:  aws.String(getTestArn("1234")),
				Name: aws.String("foo"),
				Id:   aws.String("id"),
			},
		},
		NextToken: nil,
	}
	mockLattice.EXPECT().ListServiceNetworksPagesWithContext(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx aws.Context, input *vpclattice.ListServiceNetworksInput, f func(*vpclattice.ListServiceNetworksOutput, bool) bool, opts ...request.Option) error {
			f(&listOutput, true)
			return nil
		}).Times(1)

	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(
		nil, errors.Errorf("TAG_ERR")).Times(1)

	_, tagErr := d.FindServiceNetwork(ctx, "foo", "1234")
	assert.NotNil(t, tagErr)
	assert.False(t, IsNotFoundError(tagErr))
}

type StringNameProvider struct {
	name string
}

func (p *StringNameProvider) LatticeName() string {
	return p.name
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

	itemFound, err1 := d.FindService(ctx, &StringNameProvider{name})
	assert.Nil(t, err1)
	assert.NotNil(t, itemFound)
	assert.Equal(t, name, *itemFound.Name)

	itemNotFound, err2 := d.FindService(ctx, &StringNameProvider{"no-name"})
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

	itemFound1, err1 := d.FindService(ctx, &StringNameProvider{"name1"})
	assert.Nil(t, err1)
	assert.NotNil(t, itemFound1)
	assert.Equal(t, "name1", *itemFound1.Name)

	itemFound2, err2 := d.FindService(ctx, &StringNameProvider{"name2"})
	assert.Nil(t, err2)
	assert.NotNil(t, itemFound2)
	assert.Equal(t, "name2", *itemFound2.Name)

	itemNotFound, err3 := d.FindService(ctx, &StringNameProvider{"no-name"})
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

	_, listErr := d.FindService(ctx, &StringNameProvider{"foo"})
	assert.NotNil(t, listErr)
	assert.False(t, IsNotFoundError(listErr))
}
