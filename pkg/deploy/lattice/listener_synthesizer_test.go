package lattice

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_SynthesizeListenerCreate(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockListenerMgr := NewMockListenerManager(c)

	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})

	svc := &model.Service{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Service", "stack-svc-id"),
		Status:       &model.ServiceStatus{Id: "svc-id"},
	}
	assert.NoError(t, stack.AddResource(svc))

	l := &model.Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "l-id"),
		Spec: model.ListenerSpec{
			StackServiceId: "stack-svc-id",
		},
	}
	assert.NoError(t, stack.AddResource(l))

	mockListenerMgr.EXPECT().Upsert(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(
		model.ListenerStatus{Id: "new-listener-id"}, nil)

	mockListenerMgr.EXPECT().List(ctx, gomock.Any()).Return([]*vpclattice.ListenerSummary{}, nil)

	ls := NewListenerSynthesizer(gwlog.FallbackLogger, mockListenerMgr, stack)
	err := ls.Synthesize(ctx)
	assert.Nil(t, err)
}

func Test_SynthesizeListenerCreateWithReconcile(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockListenerMgr := NewMockListenerManager(c)

	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})

	svc := &model.Service{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Service", "stack-svc-id"),
		Status:       &model.ServiceStatus{Id: "svc-id"},
	}
	assert.NoError(t, stack.AddResource(svc))

	l := &model.Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "l-id"),
		Spec: model.ListenerSpec{
			StackServiceId: "stack-svc-id",
			Port:           80,
		},
	}
	assert.NoError(t, stack.AddResource(l))

	mockListenerMgr.EXPECT().Upsert(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(
		model.ListenerStatus{Id: "new-listener-id"}, nil)

	mockListenerMgr.EXPECT().List(ctx, gomock.Any()).Return([]*vpclattice.ListenerSummary{
		{
			Id:   aws.String("to-delete-id"),
			Port: aws.Int64(443), // <-- makes this listener unique
		},
	}, nil)

	mockListenerMgr.EXPECT().Delete(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, ml *model.Listener) error {
			assert.Equal(t, "to-delete-id", ml.Status.Id)
			assert.Equal(t, "svc-id", ml.Status.ServiceId)
			return nil
		})

	ls := NewListenerSynthesizer(gwlog.FallbackLogger, mockListenerMgr, stack)
	err := ls.Synthesize(ctx)
	assert.Nil(t, err)
}
