package lattice

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func Test_UpsertListener_NewListener(t *testing.T) {
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
	fixedResponse404 := &vpclattice.RuleAction{
		FixedResponse: &vpclattice.FixedResponseAction{
			StatusCode: aws.Int64(404),
		},
	}
	lm := NewListenerManager(gwlog.FallbackLogger, cloud)
	status, err := lm.Upsert(ctx, ml, ms, fixedResponse404)
	assert.Nil(t, err)
	assert.Equal(t, "new-lid", status.Id)
	assert.Equal(t, "svc-id", status.ServiceId)
}

func Test_UpsertListener_DoNotNeedToUpdateExistingHTTPAndHTTPSListener(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	fixedResponse404 := &vpclattice.RuleAction{
		FixedResponse: &vpclattice.FixedResponseAction{
			StatusCode: aws.Int64(404),
		},
	}
	ms := &model.Service{
		Status: &model.ServiceStatus{Id: "svc-id"},
	}
	for _, listenerProtocol := range []string{vpclattice.ListenerProtocolHttp, vpclattice.ListenerProtocolHttps} {
		t.Run(fmt.Sprintf("Existing %s Listener, do not need to update", listenerProtocol), func(t *testing.T) {
			ml := &model.Listener{
				Spec: model.ListenerSpec{
					Protocol: listenerProtocol,
					Port:     8181,
				},
			}

			mockLattice := mocks.NewMockLattice(c)
			cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
			mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
				&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{
					{
						Arn:  aws.String("existing-arn"),
						Id:   aws.String("existing-listener-id"),
						Name: aws.String("existing-name"),
						Port: aws.Int64(8181),
					},
				}}, nil)

			mockLattice.EXPECT().GetListenerWithContext(ctx, gomock.Any()).Times(0)
			mockLattice.EXPECT().UpdateListenerWithContext(ctx, gomock.Any()).Times(0)

			lm := NewListenerManager(gwlog.FallbackLogger, cloud)
			status, err := lm.Upsert(ctx, ml, ms, fixedResponse404)
			assert.Nil(t, err)
			assert.Equal(t, "existing-listener-id", status.Id)
			assert.Equal(t, "svc-id", status.ServiceId)
			assert.Equal(t, "existing-name", status.Name)
			assert.Equal(t, "existing-arn", status.ListenerArn)

		})
	}

}

func Test_UpsertListener_NeedToUpdateExistingTLS_PASSTHROUGHListenerDefaultAction(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	ml := &model.Listener{
		Spec: model.ListenerSpec{
			Protocol: vpclattice.ListenerProtocolTlsPassthrough,
			Port:     8181,
		},
	}
	ms := &model.Service{
		Status: &model.ServiceStatus{Id: "svc-id"},
	}

	mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{
			{
				Arn:      aws.String("existing-arn"),
				Id:       aws.String("existing-listener-id"),
				Name:     aws.String("existing-name"),
				Protocol: aws.String(vpclattice.ListenerProtocolTlsPassthrough),
				Port:     aws.Int64(8181),
			},
		}}, nil)
	oldDefaultAction := &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: []*vpclattice.WeightedTargetGroup{
				{
					TargetGroupIdentifier: aws.String("tg-id-1"),
					Weight:                aws.Int64(15),
				},
				{
					TargetGroupIdentifier: aws.String("tg-id-2"),
					Weight:                aws.Int64(85),
				},
			},
		},
	}

	newDefaultAction := &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: []*vpclattice.WeightedTargetGroup{
				{
					TargetGroupIdentifier: aws.String("tg-id-1"),
					Weight:                aws.Int64(20),
				},
				{
					TargetGroupIdentifier: aws.String("tg-id-3"),
					Weight:                aws.Int64(80),
				},
			},
		},
	}

	mockLattice.EXPECT().GetListenerWithContext(ctx, &vpclattice.GetListenerInput{
		ServiceIdentifier:  aws.String("svc-id"),
		ListenerIdentifier: aws.String("existing-listener-id"),
	}).Return(
		&vpclattice.GetListenerOutput{
			DefaultAction: oldDefaultAction,
			Id:            aws.String("existing-listener-id"),
			Port:          aws.Int64(8181),
			Protocol:      aws.String(vpclattice.ListenerProtocolTlsPassthrough),
		}, nil).Times(1)

	mockLattice.EXPECT().UpdateListenerWithContext(ctx, &vpclattice.UpdateListenerInput{
		ListenerIdentifier: aws.String("existing-listener-id"),
		ServiceIdentifier:  aws.String("svc-id"),
		DefaultAction:      newDefaultAction,
	}).Return(&vpclattice.UpdateListenerOutput{
		Id:            aws.String("existing-listener-id"),
		DefaultAction: newDefaultAction,
		Port:          aws.Int64(8181),
		Protocol:      aws.String("HTTP"),
	}, nil)

	lm := NewListenerManager(gwlog.FallbackLogger, cloud)
	status, err := lm.Upsert(ctx, ml, ms, newDefaultAction)
	assert.Nil(t, err)
	assert.Equal(t, "existing-listener-id", status.Id)
	assert.Equal(t, "svc-id", status.ServiceId)
	assert.Equal(t, "existing-name", status.Name)
	assert.Equal(t, "existing-arn", status.ListenerArn)
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

func Test_defaultListenerManager_needToUpdateDefaultAction(t *testing.T) {
	latticeSvcId := "svc-id"
	latticeListenerId := "listener-id"
	forwardAction1 := &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: []*vpclattice.WeightedTargetGroup{
				{
					TargetGroupIdentifier: aws.String("tg-id-1"),
					Weight:                aws.Int64(15),
				}, {
					TargetGroupIdentifier: aws.String("tg-id-2"),
					Weight:                aws.Int64(85),
				},
			},
		},
	}
	forwardAction1Copy := &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: []*vpclattice.WeightedTargetGroup{
				{
					TargetGroupIdentifier: aws.String("tg-id-1"),
					Weight:                aws.Int64(15),
				}, {
					TargetGroupIdentifier: aws.String("tg-id-2"),
					Weight:                aws.Int64(85),
				},
			},
		},
	}

	assert.False(t, forwardAction1 == forwardAction1Copy) // addresses are different
	assert.Equal(t, forwardAction1, forwardAction1Copy)   // contents are the same
	forwardAction2DifferentWeight := &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: []*vpclattice.WeightedTargetGroup{
				{
					TargetGroupIdentifier: aws.String("tg-id-1"),
					Weight:                aws.Int64(10),
				}, {
					TargetGroupIdentifier: aws.String("tg-id-2"),
					Weight:                aws.Int64(90),
				},
			},
		},
	}
	forwardAction3DifferentTgId := &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: []*vpclattice.WeightedTargetGroup{
				{
					TargetGroupIdentifier: aws.String("tg-id-1"),
					Weight:                aws.Int64(15),
				}, {
					TargetGroupIdentifier: aws.String("tg-id-3"),
					Weight:                aws.Int64(85),
				},
			},
		},
	}
	tests := []struct {
		name                             string
		listenerDefaultActionFromStack   *vpclattice.RuleAction
		listenerDefaultActionFromLattice *vpclattice.RuleAction
		expectNeedToUpdateDefaultAction  bool
		wantErr                          error
	}{
		{
			name: "Same 404 FixedResponseAction from stack and lattice, do not need to update",
			listenerDefaultActionFromStack: &vpclattice.RuleAction{
				FixedResponse: &vpclattice.FixedResponseAction{StatusCode: aws.Int64(404)},
			},
			listenerDefaultActionFromLattice: &vpclattice.RuleAction{
				FixedResponse: &vpclattice.FixedResponseAction{StatusCode: aws.Int64(404)},
			},
			expectNeedToUpdateDefaultAction: false,
			wantErr:                         nil,
		},
		{
			name:                             "Same ForwardAction from stack and lattice, do not need to update",
			listenerDefaultActionFromStack:   forwardAction1,
			listenerDefaultActionFromLattice: forwardAction1Copy,
			expectNeedToUpdateDefaultAction:  false,
			wantErr:                          nil,
		},
		{
			name:                             "Different weight in ForwardAction from stack and lattice, need to update",
			listenerDefaultActionFromStack:   forwardAction1,
			listenerDefaultActionFromLattice: forwardAction2DifferentWeight,
			expectNeedToUpdateDefaultAction:  true,
			wantErr:                          nil,
		}, {
			name:                             "Different tg id in ForwardAction from stack and lattice, need to update",
			listenerDefaultActionFromStack:   forwardAction1,
			listenerDefaultActionFromLattice: forwardAction3DifferentTgId,
			expectNeedToUpdateDefaultAction:  true,
			wantErr:                          nil,
		},
	}
	c := gomock.NewController(t)
	defer c.Finish()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLattice.EXPECT().GetListenerWithContext(context.TODO(), &vpclattice.GetListenerInput{
				ServiceIdentifier:  &latticeSvcId,
				ListenerIdentifier: &latticeListenerId,
			}).Return(&vpclattice.GetListenerOutput{
				DefaultAction: tt.listenerDefaultActionFromLattice,
			}, nil)

			d := &defaultListenerManager{
				log:   gwlog.FallbackLogger,
				cloud: cloud,
			}
			got, err := d.needToUpdateDefaultAction(context.TODO(), latticeSvcId, latticeListenerId, tt.listenerDefaultActionFromStack)
			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.expectNeedToUpdateDefaultAction, got)
		})
	}
}
