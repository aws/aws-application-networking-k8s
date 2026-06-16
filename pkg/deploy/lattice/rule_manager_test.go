package lattice

import (
	"context"
	"testing"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
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

		mockLattice.EXPECT().CreateRule(ctx, gomock.Any()).Return(
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
		mockTagging := mocks.NewMockTagging(c)
		cloudWithTagging := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

		mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
			[]*vpclattice.GetRuleOutput{
				{
					Id:  aws.String("existing-id"),
					Arn: aws.String("existing-arn"),
					Match: &types.RuleMatchMemberHttpMatch{
						Value: types.HttpMatch{
							Method: aws.String("POST"),
						},
					},
					Action: &types.RuleActionMemberFixedResponse{
						Value: types.FixedResponseAction{}, // <-- this will trigger update
					},
					Name:     aws.String("existing-name"),
					Priority: aws.Int32(1),
				},
			}, nil)

		mockTagging.EXPECT().UpdateTags(ctx, "existing-arn", gomock.Any(), nil).Return(nil)

		mockLattice.EXPECT().UpdateRule(ctx, gomock.Any()).Return(
			&vpclattice.UpdateRuleOutput{
				Arn:  aws.String("existing-arn"),
				Id:   aws.String("existing-id"),
				Name: aws.String("existing-name"),
			}, nil)

		rm := NewRuleManager(gwlog.FallbackLogger, cloudWithTagging)
		ruleStatus, err := rm.Upsert(ctx, r, l, svc)
		assert.Nil(t, err)
		assert.Equal(t, "existing-arn", ruleStatus.Arn)
	})

	t.Run("test update path match", func(t *testing.T) {
		mockTagging := mocks.NewMockTagging(c)
		cloudWithTagging := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

		mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
			[]*vpclattice.GetRuleOutput{
				{
					Id:  aws.String("existing-id"),
					Arn: aws.String("existing-arn"),
					Match: &types.RuleMatchMemberHttpMatch{
						Value: types.HttpMatch{
							HeaderMatches: make([]types.HeaderMatch, 0), // this is what's returned in the Lattice API, not nil
							PathMatch: &types.PathMatch{
								CaseSensitive: aws.Bool(true), // default value
								Match: &types.PathMatchTypeMemberPrefix{
									Value: "/foo",
								},
							},
						},
					},
					Action: &types.RuleActionMemberFixedResponse{
						Value: types.FixedResponseAction{}, // <-- this will trigger update
					},
					Name:     aws.String("existing-name"),
					Priority: aws.Int32(1),
				},
			}, nil)

		mockTagging.EXPECT().UpdateTags(ctx, "existing-arn", gomock.Any(), nil).Return(nil)

		mockLattice.EXPECT().UpdateRule(ctx, gomock.Any()).Return(
			&vpclattice.UpdateRuleOutput{
				Arn:  aws.String("existing-arn"),
				Id:   aws.String("existing-id"),
				Name: aws.String("existing-name"),
			}, nil)

		rm := NewRuleManager(gwlog.FallbackLogger, cloudWithTagging)
		ruleStatus, err := rm.Upsert(ctx, r2, l, svc)
		assert.Nil(t, err)
		assert.Equal(t, "existing-arn", ruleStatus.Arn)
	})

	t.Run("test update - nothing to do", func(t *testing.T) {
		mockTagging := mocks.NewMockTagging(c)
		cloudWithTagging := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

		mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
			[]*vpclattice.GetRuleOutput{
				{
					Id:  aws.String("existing-id"),
					Arn: aws.String("existing-arn"),
					Match: &types.RuleMatchMemberHttpMatch{
						Value: types.HttpMatch{
							Method: aws.String("POST"),
						},
					},
					Action: &types.RuleActionMemberForward{
						Value: types.ForwardAction{
							TargetGroups: []types.WeightedTargetGroup{
								{
									TargetGroupIdentifier: aws.String("tg-id"),
									Weight:                aws.Int32(1),
								},
							},
						},
					},
					Name:     aws.String("existing-name"),
					Priority: aws.Int32(1),
				},
			}, nil) // <-- should be an exact match, no update required

		mockTagging.EXPECT().UpdateTags(ctx, "existing-arn", gomock.Any(), nil).Return(nil)

		rm := NewRuleManager(gwlog.FallbackLogger, cloudWithTagging)
		ruleStatus, err := rm.Upsert(ctx, r, l, svc)
		assert.Nil(t, err)
		assert.Equal(t, "existing-arn", ruleStatus.Arn)
	})

	t.Run("test create - invalid backendRefs", func(t *testing.T) {
		mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
			[]*vpclattice.GetRuleOutput{}, nil).Times(2)

		mockLattice.EXPECT().CreateRule(ctx, gomock.Any()).DoAndReturn(
			func(ctx context.Context, input *vpclattice.CreateRuleInput, i ...interface{}) (*vpclattice.CreateRuleOutput, error) {
				assert.Equal(t, int32(500), *input.Action.(*types.RuleActionMemberFixedResponse).Value.StatusCode)

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

		mockLattice.EXPECT().CreateRule(ctx, gomock.Any()).DoAndReturn(
			func(ctx context.Context, input *vpclattice.CreateRuleInput, i ...interface{}) (*vpclattice.CreateRuleOutput, error) {
				assert.Equal(t, "POST", aws.ToString(input.Match.(*types.RuleMatchMemberHttpMatch).Value.Method))
				fwd := input.Action.(*types.RuleActionMemberForward).Value
				assert.Equal(t, 1, len(fwd.TargetGroups))
				assert.Equal(t, "tg-id", aws.ToString(fwd.TargetGroups[0].TargetGroupIdentifier))

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
				Match: &types.RuleMatchMemberHttpMatch{
					Value: types.HttpMatch{
						Method: aws.String("GET"), // <-- will be considered a different rule
					},
				},
				Name:     aws.String("existing-name"),
				Priority: aws.Int32(1), // <-- we have the same priority
			},
		}, nil)

	expectedPriority := int32(2)

	mockLattice.EXPECT().CreateRule(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, input *vpclattice.CreateRuleInput, i ...interface{}) (*vpclattice.CreateRuleOutput, error) {
			// 2 is the "next" available priority
			assert.Equal(t, expectedPriority, aws.ToInt32(input.Priority))

			return &vpclattice.CreateRuleOutput{
				Arn:      aws.String("new-arn"),
				Id:       aws.String("new-id"),
				Name:     aws.String("new-name"),
				Priority: aws.Int32(expectedPriority),
			}, nil
		})

	rm := NewRuleManager(gwlog.FallbackLogger, cloud)
	ruleStatus, err := rm.Upsert(ctx, r, l, svc)
	assert.Nil(t, err)
	assert.Equal(t, "new-arn", ruleStatus.Arn)
	assert.Equal(t, int64(expectedPriority), ruleStatus.Priority)
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

	mockLattice.EXPECT().BatchUpdateRule(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, input *vpclattice.BatchUpdateRuleInput, i ...interface{}) (*vpclattice.BatchUpdateRuleOutput, error) {
			for _, rule := range input.Rules {
				if *rule.RuleIdentifier == "rule-0" {
					assert.Equal(t, int32(2), *rule.Priority)
					continue
				}
				if *rule.RuleIdentifier == "rule-1" {
					assert.Equal(t, int32(1), *rule.Priority)
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

func Test_RuleManager_WithAdditionalTags_Create(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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
			AdditionalTags: mocks.Tags{
				"Environment": "Test",
				"Project":     "RuleManager",
			},
		},
	}

	mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return([]*vpclattice.GetRuleOutput{}, nil)

	expectedTags := cloud.MergeTags(cloud.DefaultTags(), r.Spec.AdditionalTags)

	mockLattice.EXPECT().CreateRule(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, input *vpclattice.CreateRuleInput, i ...interface{}) (*vpclattice.CreateRuleOutput, error) {
			assert.Equal(t, expectedTags, input.Tags, "Rule tags should include additional tags")

			return &vpclattice.CreateRuleOutput{
				Arn:  aws.String("arn"),
				Id:   aws.String("id"),
				Name: aws.String("name"),
			}, nil
		})

	rm := NewRuleManager(gwlog.FallbackLogger, cloud)
	ruleStatus, err := rm.Upsert(ctx, r, l, svc)
	assert.Nil(t, err)
	assert.Equal(t, "arn", ruleStatus.Arn)
}

func Test_RuleManager_WithAdditionalTags_Update(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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
			AdditionalTags: mocks.Tags{
				"Environment": "Prod",
				"Project":     "RuleUpdate",
			},
		},
	}

	mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
		[]*vpclattice.GetRuleOutput{
			{
				Id:  aws.String("existing-id"),
				Arn: aws.String("existing-arn"),
				Match: &types.RuleMatchMemberHttpMatch{
					Value: types.HttpMatch{
						Method: aws.String("POST"),
					},
				},
				Action: &types.RuleActionMemberFixedResponse{
					Value: types.FixedResponseAction{}, // Different action will trigger update
				},
				Name:     aws.String("existing-name"),
				Priority: aws.Int32(1),
			},
		}, nil)

	mockTagging.EXPECT().UpdateTags(ctx, "existing-arn", r.Spec.AdditionalTags, nil).Return(nil)

	mockLattice.EXPECT().UpdateRule(ctx, gomock.Any()).Return(
		&vpclattice.UpdateRuleOutput{
			Arn:  aws.String("existing-arn"),
			Id:   aws.String("existing-id"),
			Name: aws.String("existing-name"),
		}, nil)

	rm := NewRuleManager(gwlog.FallbackLogger, cloud)
	ruleStatus, err := rm.Upsert(ctx, r, l, svc)
	assert.Nil(t, err)
	assert.Equal(t, "existing-arn", ruleStatus.Arn)
}

func Test_RuleManager_WithAdditionalTags_UpdateNoActionChange(t *testing.T) {
	// Test case: update existing rule with additional tags but no action change
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

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
			AdditionalTags: mocks.Tags{
				"Environment": "Staging",
				"Project":     "RuleNoUpdate",
			},
		},
	}

	// Existing rule with exact match (no action update needed)
	mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
		[]*vpclattice.GetRuleOutput{
			{
				Id:  aws.String("existing-id"),
				Arn: aws.String("existing-arn"),
				Match: &types.RuleMatchMemberHttpMatch{
					Value: types.HttpMatch{
						Method: aws.String("POST"),
					},
				},
				Action: &types.RuleActionMemberForward{
					Value: types.ForwardAction{
						TargetGroups: []types.WeightedTargetGroup{
							{
								TargetGroupIdentifier: aws.String("tg-id"),
								Weight:                aws.Int32(1),
							},
						},
					},
				},
				Name:     aws.String("existing-name"),
				Priority: aws.Int32(1),
			},
		}, nil)

	// Mock UpdateTags call for additional tags (should still be called even if no action update)
	mockTagging.EXPECT().UpdateTags(ctx, "existing-arn", r.Spec.AdditionalTags, nil).Return(nil)

	// No UpdateRule call expected since action matches
	mockLattice.EXPECT().UpdateRule(ctx, gomock.Any()).Times(0)

	rm := NewRuleManager(gwlog.FallbackLogger, cloud)
	ruleStatus, err := rm.Upsert(ctx, r, l, svc)
	assert.Nil(t, err)
	assert.Equal(t, "existing-arn", ruleStatus.Arn)
}

func Test_RuleManager_WithTakeoverAnnotation_UpdateTags(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	svc := &model.Service{
		Spec: model.ServiceSpec{
			AllowTakeoverFrom: "other-account/other-cluster/other-vpc",
		},
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
			AdditionalTags: mocks.Tags{
				"Environment": "Takeover",
			},
		},
	}

	mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
		[]*vpclattice.GetRuleOutput{
			{
				Id:  aws.String("existing-id"),
				Arn: aws.String("existing-arn"),
				Match: &types.RuleMatchMemberHttpMatch{
					Value: types.HttpMatch{
						Method: aws.String("POST"),
					},
				},
				Action: &types.RuleActionMemberForward{
					Value: types.ForwardAction{
						TargetGroups: []types.WeightedTargetGroup{
							{
								TargetGroupIdentifier: aws.String("tg-id"),
								Weight:                aws.Int32(1),
							},
						},
					},
				},
				Name:     aws.String("existing-name"),
				Priority: aws.Int32(1),
			},
		}, nil)

	expectedAwsManagedTags := mocks.Tags{
		pkg_aws.TagManagedBy: cloud.DefaultTags()[pkg_aws.TagManagedBy],
	}
	mockTagging.EXPECT().UpdateTags(ctx, "existing-arn", r.Spec.AdditionalTags, expectedAwsManagedTags).Return(nil)

	mockLattice.EXPECT().UpdateRule(ctx, gomock.Any()).Times(0)

	rm := NewRuleManager(gwlog.FallbackLogger, cloud)
	ruleStatus, err := rm.Upsert(ctx, r, l, svc)
	assert.Nil(t, err)
	assert.Equal(t, "existing-arn", ruleStatus.Arn)
}
