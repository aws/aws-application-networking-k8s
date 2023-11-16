package lattice

import (
	"context"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_Create(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	// each rule references a stack and a service which need to be present in the stack
	// in order to proceed, these just need their status+id
	svc := &model.Service{
		Status: &model.ServiceStatus{Id: "svc-id"},
	}

	l := &model.Listener{
		Spec: model.ListenerSpec{
			Port:     80,
			Protocol: "HTTP",
		},
		Status: &model.ListenerStatus{Id: "listener-id"},
	}

	r := &model.Rule{
		Spec: model.RuleSpec{
			Priority: 1,
			Method:   "POST",
			Action: model.RuleAction{
				TargetGroups: []*model.RuleTargetGroup{
					{
						LatticeTgId: "tg-id",
						Weight:      1,
					},
				},
			},
		},
	}

	r2 := &model.Rule{
		Spec: model.RuleSpec{
			Priority: 1,
			Action: model.RuleAction{
				TargetGroups: []*model.RuleTargetGroup{
					{
						LatticeTgId: "tg-id",
						Weight:      1,
					},
				},
			},
			PathMatchPrefix: true,
			PathMatchValue:  "/foo",
		},
	}

	rInvalidBR := &model.Rule{
		Spec: model.RuleSpec{
			Priority: 1,
			Action: model.RuleAction{
				TargetGroups: []*model.RuleTargetGroup{
					{
						LatticeTgId: model.InvalidBackendRefTgId,
						Weight:      1,
					},
				},
			},
			PathMatchPrefix: true,
			PathMatchValue:  "/foo",
		},
	}

	rTwoInvalidBR := &model.Rule{
		Spec: model.RuleSpec{
			Priority: 1,
			Action: model.RuleAction{
				TargetGroups: []*model.RuleTargetGroup{
					{
						LatticeTgId: model.InvalidBackendRefTgId,
						Weight:      1,
					},
					{
						LatticeTgId: model.InvalidBackendRefTgId,
						Weight:      1,
					},
				},
			},
			PathMatchPrefix: true,
			PathMatchValue:  "/foo",
		},
	}

	rOneValidBR := &model.Rule{
		Spec: model.RuleSpec{
			Priority: 1,
			Method:   "POST",
			Action: model.RuleAction{
				TargetGroups: []*model.RuleTargetGroup{
					{
						LatticeTgId: model.InvalidBackendRefTgId,
						Weight:      1,
					},
					{
						LatticeTgId: model.InvalidBackendRefTgId,
						Weight:      1,
					},
					{
						LatticeTgId: "tg-id",
						Weight:      1,
					},
				},
			},
			PathMatchPrefix: true,
			PathMatchValue:  "/foo",
		},
	}

	t.Run("test create", func(t *testing.T) {
		mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
			[]*vpclattice.GetRuleOutput{}, nil)

		mockLattice.EXPECT().CreateRuleWithContext(ctx, gomock.Any()).Return(
			&vpclattice.CreateRuleOutput{
				Arn:  aws.String("arn"),
				Id:   aws.String("id"),
				Name: aws.String("name"),
			}, nil)

		rm := NewRuleManager(gwlog.FallbackLogger, cloud)
		ruleStatus, err := rm.Upsert(ctx, r, l, svc)
		assert.Nil(t, err)
		assert.Equal(t, "arn", ruleStatus.Arn)
	})

	t.Run("test update method match", func(t *testing.T) {
		mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
			[]*vpclattice.GetRuleOutput{
				{
					Id:  aws.String("existing-id"),
					Arn: aws.String("existing-arn"),
					Match: &vpclattice.RuleMatch{
						HttpMatch: &vpclattice.HttpMatch{
							Method: aws.String("POST"),
						},
					},
					Action: &vpclattice.RuleAction{
						FixedResponse: &vpclattice.FixedResponseAction{}, // <-- this will trigger update
					},
					Name:     aws.String("existing-name"),
					Priority: aws.Int64(1),
				},
			}, nil)

		mockLattice.EXPECT().UpdateRuleWithContext(ctx, gomock.Any()).Return(
			&vpclattice.UpdateRuleOutput{
				Arn:  aws.String("existing-arn"),
				Id:   aws.String("existing-id"),
				Name: aws.String("existing-name"),
			}, nil)

		rm := NewRuleManager(gwlog.FallbackLogger, cloud)
		ruleStatus, err := rm.Upsert(ctx, r, l, svc)
		assert.Nil(t, err)
		assert.Equal(t, "existing-arn", ruleStatus.Arn)
	})

	t.Run("test update path match", func(t *testing.T) {
		mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
			[]*vpclattice.GetRuleOutput{
				{
					Id:  aws.String("existing-id"),
					Arn: aws.String("existing-arn"),
					Match: &vpclattice.RuleMatch{
						HttpMatch: &vpclattice.HttpMatch{
							HeaderMatches: make([]*vpclattice.HeaderMatch, 0), // this is what's returned in the Lattice API, not nil
							PathMatch: &vpclattice.PathMatch{
								CaseSensitive: aws.Bool(true), // default value
								Match: &vpclattice.PathMatchType{
									Prefix: aws.String("/foo"),
								},
							},
						},
					},
					Action: &vpclattice.RuleAction{
						FixedResponse: &vpclattice.FixedResponseAction{}, // <-- this will trigger update
					},
					Name:     aws.String("existing-name"),
					Priority: aws.Int64(1),
				},
			}, nil)

		mockLattice.EXPECT().UpdateRuleWithContext(ctx, gomock.Any()).Return(
			&vpclattice.UpdateRuleOutput{
				Arn:  aws.String("existing-arn"),
				Id:   aws.String("existing-id"),
				Name: aws.String("existing-name"),
			}, nil)

		rm := NewRuleManager(gwlog.FallbackLogger, cloud)
		ruleStatus, err := rm.Upsert(ctx, r2, l, svc)
		assert.Nil(t, err)
		assert.Equal(t, "existing-arn", ruleStatus.Arn)
	})

	t.Run("test update - nothing to do", func(t *testing.T) {
		mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
			[]*vpclattice.GetRuleOutput{
				{
					Id:  aws.String("existing-id"),
					Arn: aws.String("existing-arn"),
					Match: &vpclattice.RuleMatch{
						HttpMatch: &vpclattice.HttpMatch{
							Method: aws.String("POST"),
						},
					},
					Action: &vpclattice.RuleAction{
						Forward: &vpclattice.ForwardAction{
							TargetGroups: []*vpclattice.WeightedTargetGroup{
								{
									TargetGroupIdentifier: aws.String("tg-id"),
									Weight:                aws.Int64(1),
								},
							},
						},
					},
					Name:     aws.String("existing-name"),
					Priority: aws.Int64(1),
				},
			}, nil) // <-- should be an exact match, no update required

		rm := NewRuleManager(gwlog.FallbackLogger, cloud)
		ruleStatus, err := rm.Upsert(ctx, r, l, svc)
		assert.Nil(t, err)
		assert.Equal(t, "existing-arn", ruleStatus.Arn)
	})

	t.Run("test create - invalid backendRefs", func(t *testing.T) {
		mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
			[]*vpclattice.GetRuleOutput{}, nil).Times(2)

		mockLattice.EXPECT().CreateRuleWithContext(ctx, gomock.Any()).DoAndReturn(
			func(ctx context.Context, input *vpclattice.CreateRuleInput, i ...interface{}) (*vpclattice.CreateRuleOutput, error) {
				assert.Equal(t, int64(404), aws.Int64Value(input.Action.FixedResponse.StatusCode))

				return &vpclattice.CreateRuleOutput{
					Arn:  aws.String("arn"),
					Id:   aws.String("id"),
					Name: aws.String("name"),
				}, nil
			}).Times(2)

		rm := NewRuleManager(gwlog.FallbackLogger, cloud)
		ruleStatus, err := rm.Upsert(ctx, rInvalidBR, l, svc)
		assert.Nil(t, err)
		assert.Equal(t, "arn", ruleStatus.Arn)

		// result should be the same so long as all backendRefs are invalid
		ruleStatus, err = rm.Upsert(ctx, rTwoInvalidBR, l, svc)
		assert.Nil(t, err)
		assert.Equal(t, "arn", ruleStatus.Arn)
	})

	t.Run("test create - one valid backendRef, two invalid", func(t *testing.T) {
		mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
			[]*vpclattice.GetRuleOutput{}, nil)

		mockLattice.EXPECT().CreateRuleWithContext(ctx, gomock.Any()).DoAndReturn(
			func(ctx context.Context, input *vpclattice.CreateRuleInput, i ...interface{}) (*vpclattice.CreateRuleOutput, error) {
				assert.Equal(t, "POST", aws.StringValue(input.Match.HttpMatch.Method))
				assert.Equal(t, 1, len(input.Action.Forward.TargetGroups))
				assert.Equal(t, "tg-id", aws.StringValue(input.Action.Forward.TargetGroups[0].TargetGroupIdentifier))

				return &vpclattice.CreateRuleOutput{
					Arn:  aws.String("arn"),
					Id:   aws.String("id"),
					Name: aws.String("name"),
				}, nil
			})

		rm := NewRuleManager(gwlog.FallbackLogger, cloud)
		ruleStatus, err := rm.Upsert(ctx, rOneValidBR, l, svc)
		assert.Nil(t, err)
		assert.Equal(t, "arn", ruleStatus.Arn)
	})
}

func Test_CreateWithTempPriority(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	svc := &model.Service{
		Status: &model.ServiceStatus{Id: "svc-id"},
	}

	l := &model.Listener{
		Spec: model.ListenerSpec{
			Port:     80,
			Protocol: "HTTP",
		},
		Status: &model.ListenerStatus{Id: "listener-id"},
	}

	r := &model.Rule{
		Spec: model.RuleSpec{
			Priority: 1,
			Method:   "POST",
		},
	}

	mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
		[]*vpclattice.GetRuleOutput{
			{
				Id:  aws.String("existing-id"),
				Arn: aws.String("existing-arn"),
				Match: &vpclattice.RuleMatch{
					HttpMatch: &vpclattice.HttpMatch{
						Method: aws.String("GET"), // <-- will be considered a different rule
					},
				},
				Name:     aws.String("existing-name"),
				Priority: aws.Int64(1), // <-- we have the same priority
			},
		}, nil)

	expectedPriority := int64(2)

	mockLattice.EXPECT().CreateRuleWithContext(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, input *vpclattice.CreateRuleInput, i ...interface{}) (*vpclattice.CreateRuleOutput, error) {
			// 2 is the "next" available priority
			assert.Equal(t, expectedPriority, aws.Int64Value(input.Priority))

			return &vpclattice.CreateRuleOutput{
				Arn:      aws.String("new-arn"),
				Id:       aws.String("new-id"),
				Name:     aws.String("new-name"),
				Priority: aws.Int64(expectedPriority),
			}, nil
		})

	rm := NewRuleManager(gwlog.FallbackLogger, cloud)
	ruleStatus, err := rm.Upsert(ctx, r, l, svc)
	assert.Nil(t, err)
	assert.Equal(t, "new-arn", ruleStatus.Arn)
	assert.Equal(t, expectedPriority, ruleStatus.Priority)
}

func Test_UpdatePriorities(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	// note that priorities are actually just assigned in order
	// so this example of descending priority is contrived
	rules := []*model.Rule{
		{
			Spec:   model.RuleSpec{Priority: 2},
			Status: &model.RuleStatus{Id: "rule-0"},
		},
		{
			Spec:   model.RuleSpec{Priority: 1},
			Status: &model.RuleStatus{Id: "rule-1"},
		},
	}

	mockLattice.EXPECT().BatchUpdateRuleWithContext(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, input *vpclattice.BatchUpdateRuleInput, i ...interface{}) (*vpclattice.BatchUpdateRuleOutput, error) {
			for _, rule := range input.Rules {
				if *rule.RuleIdentifier == "rule-0" {
					assert.Equal(t, int64(2), *rule.Priority)
					continue
				}
				if *rule.RuleIdentifier == "rule-1" {
					assert.Equal(t, int64(1), *rule.Priority)
					continue
				}
				assert.Fail(t, "should not reach this point")
			}

			return &vpclattice.BatchUpdateRuleOutput{}, nil
		})

	rm := NewRuleManager(gwlog.FallbackLogger, cloud)
	err := rm.UpdatePriorities(ctx, "svc-id", "l-id", rules)
	assert.Nil(t, err)
}
