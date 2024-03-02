package lattice

import (
	"context"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_CreateListenerNew(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	ml := &model.Listener{}
	ms := &model.Service{
		Status: &model.ServiceStatus{Id: "svc-id"},
	}

	mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{}}, nil)

	mockLattice.EXPECT().CreateListenerWithContext(ctx, gomock.Any()).DoAndReturn(
		func(ctx aws.Context, input *vpclattice.CreateListenerInput, opts ...request.Option) (*vpclattice.CreateListenerOutput, error) {
			// part of the gw spec is to default to 404, so assert that here
			assert.Equal(t, int64(404), *input.DefaultAction.FixedResponse.StatusCode)
			assert.Equal(t, "svc-id", *input.ServiceIdentifier)

			return &vpclattice.CreateListenerOutput{Id: aws.String("new-lid")}, nil
		},
	)

	lm := NewListenerManager(gwlog.FallbackLogger, cloud)
	status, err := lm.Upsert(ctx, ml, ms, nil)
	assert.Nil(t, err)
	assert.Equal(t, "new-lid", status.Id)
	assert.Equal(t, "svc-id", status.ServiceId)
}

func Test_CreateListenerExisting(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	ml := &model.Listener{
		Spec: model.ListenerSpec{
			Port: 8181,
		},
	}
	ms := &model.Service{
		Status: &model.ServiceStatus{Id: "svc-id"},
	}

	mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{
			{
				Arn:  aws.String("existing-arn"),
				Id:   aws.String("existing-lid"),
				Name: aws.String("existing-name"),
				Port: aws.Int64(8181),
			},
		}}, nil)

	lm := NewListenerManager(gwlog.FallbackLogger, cloud)
	status, err := lm.Upsert(ctx, ml, ms, nil)
	assert.Nil(t, err)
	assert.Equal(t, "existing-lid", status.Id)
	assert.Equal(t, "svc-id", status.ServiceId)
	assert.Equal(t, "existing-name", status.Name)
}

func Test_DeleteListener(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	t.Run("test precondition errors", func(t *testing.T) {
		ml := &model.Listener{}

		lm := NewListenerManager(gwlog.FallbackLogger, cloud)
		assert.Error(t, lm.Delete(ctx, nil))
		assert.Error(t, lm.Delete(ctx, ml))
	})

	t.Run("listener only success", func(t *testing.T) {
		ml := &model.Listener{Status: &model.ListenerStatus{Id: "lid", ServiceId: "sid"}}
		mockLattice.EXPECT().DeleteListenerWithContext(ctx, gomock.Any()).DoAndReturn(
			func(ctx aws.Context, input *vpclattice.DeleteListenerInput, opts ...request.Option) (*vpclattice.DeleteListenerOutput, error) {
				assert.Equal(t, "lid", *input.ListenerIdentifier)
				assert.Equal(t, "sid", *input.ServiceIdentifier)
				return &vpclattice.DeleteListenerOutput{}, nil
			},
		)
		lm := NewListenerManager(gwlog.FallbackLogger, cloud)
		assert.NoError(t, lm.Delete(ctx, ml))
	})
}
