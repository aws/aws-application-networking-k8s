package lattice

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"

	"testing"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"

	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

var rulelist = []struct {
	Arn       string
	Id        string
	IsDefault bool
	Name      string
}{
	{

		Arn:       "Rule-Arn-1",
		Id:        "Rule-Id-1",
		IsDefault: false,
		Name:      "Rule-1",
	},
	{

		Arn:       "Rule-Arn-2",
		Id:        "Rule-Id-2",
		IsDefault: false,
		Name:      "Rule-2",
	},
}

var rules = []*latticemodel.Rule{
	&latticemodel.Rule{
		Spec: latticemodel.RuleSpec{
			ServiceName:      "svc-1",
			ServiceNamespace: "default",
			ListenerPort:     int64(80),
			ListenerProtocol: "HTTP",
			RuleID:           "rule-1", //TODO, maybe rename this field to RuleName
		},
		Status: &latticemodel.RuleStatus{
			ServiceID:  "serviceID1",
			ListenerID: "listenerID1",
			RuleID:     "rule-ID-1",
		},
	},

	&latticemodel.Rule{
		Spec: latticemodel.RuleSpec{
			ServiceName:      "svc-1",
			ServiceNamespace: "default",
			ListenerPort:     int64(80),
			ListenerProtocol: "HTTP",
			RuleID:           "rule-2", //TODO, maybe rename this field to RuleName
		},
		Status: &latticemodel.RuleStatus{
			ServiceID:  "serviceID1",
			ListenerID: "listenerID1",
			RuleID:     "rule-ID-2",
		},
	},
}

func Test_CreateRule(t *testing.T) {
	ServiceName := "seviceName"
	ServiceNameSpace := "defaultService"
	ServiceID := "serviceID"
	ListenerPort := int64(80)
	ListenerProtocol := "HTTP"
	ListenerID := "listenerID"
	ruleID := "ruleID"

	var hdr1 = "env1"
	var hdr1Value = "test1"
	var hdr2 = "env2"
	var hdr2Value = "test2"

	var weight1 = int64(90)
	var weight2 = int64(10)
	weightRulePriority := 1
	weightRuleID := fmt.Sprintf("rule-%d", weightRulePriority)
	WeigthedAction_1 := latticemodel.RuleTargetGroup{
		Name:            "TestCreateWeighted1",
		Namespace:       "default",
		IsServiceImport: false,
		Weight:          weight1,
	}

	WeightedAction_11 := latticemodel.RuleTargetGroup{
		Name:            "TestCreateWeighted1",
		Namespace:       "default",
		IsServiceImport: false,
		Weight:          weight2,
	}

	WeigthedAction_2 := latticemodel.RuleTargetGroup{
		Name:            "TestCreateWeighte2",
		Namespace:       "default",
		IsServiceImport: false,
		Weight:          weight2,
	}

	WeigthedAction_22 := latticemodel.RuleTargetGroup{
		Name:            "TestCreateWeighte2",
		Namespace:       "default",
		IsServiceImport: false,
		Weight:          weight1,
	}

	WeightedRule_1 := latticemodel.Rule{
		Spec: latticemodel.RuleSpec{
			ServiceName:      ServiceName,
			ServiceNamespace: ServiceNameSpace,
			ListenerPort:     ListenerPort,
			ListenerProtocol: ListenerProtocol,
			PathMatchPrefix:  true,
			PathMatchValue:   "",
			RuleID:           weightRuleID,
			Action: latticemodel.RuleAction{
				TargetGroups: []*latticemodel.RuleTargetGroup{
					&WeigthedAction_1,
				},
			},
		},
		Status: &latticemodel.RuleStatus{
			RuleARN:    "ruleARn",
			RuleID:     "rule-id-1",
			ListenerID: ListenerID,
			ServiceID:  ServiceID,
		},
	}

	WeightedRule_1_2 := latticemodel.Rule{
		Spec: latticemodel.RuleSpec{
			ServiceName:      ServiceName,
			ServiceNamespace: ServiceNameSpace,
			ListenerPort:     ListenerPort,
			ListenerProtocol: ListenerProtocol,
			PathMatchValue:   "",
			PathMatchPrefix:  true,
			RuleID:           weightRuleID,
			Action: latticemodel.RuleAction{
				TargetGroups: []*latticemodel.RuleTargetGroup{
					&WeigthedAction_1,
					&WeigthedAction_2,
				},
			},
		},
		Status: &latticemodel.RuleStatus{
			RuleARN:    "ruleARn",
			RuleID:     "rule-id-1-2",
			ListenerID: ListenerID,
			ServiceID:  ServiceID,
		},
	}

	WeightedRule_2_1 := latticemodel.Rule{
		Spec: latticemodel.RuleSpec{
			ServiceName:      ServiceName,
			ServiceNamespace: ServiceNameSpace,
			ListenerPort:     ListenerPort,
			ListenerProtocol: ListenerProtocol,
			PathMatchValue:   "",
			PathMatchPrefix:  true,
			RuleID:           weightRuleID,
			Action: latticemodel.RuleAction{
				TargetGroups: []*latticemodel.RuleTargetGroup{
					&WeightedAction_11,
					&WeigthedAction_22,
				},
			},
		},
		Status: &latticemodel.RuleStatus{
			RuleARN:    "ruleARn",
			RuleID:     "rule-id-2-1",
			ListenerID: ListenerID,
			ServiceID:  ServiceID,
		},
	}

	pathRule_1 := latticemodel.Rule{
		Spec: latticemodel.RuleSpec{
			ServiceName:      ServiceName,
			ServiceNamespace: ServiceNameSpace,
			ListenerPort:     ListenerPort,
			ListenerProtocol: ListenerProtocol,
			RuleID:           weightRuleID,
			PathMatchPrefix:  true,
			PathMatchValue:   "/ver-1",
			Action: latticemodel.RuleAction{
				TargetGroups: []*latticemodel.RuleTargetGroup{
					&WeigthedAction_1,
				},
			},
		},
		Status: &latticemodel.RuleStatus{
			RuleARN:    "ruleARn",
			RuleID:     "rule-id-2-1",
			ListenerID: ListenerID,
			ServiceID:  ServiceID,
		},
	}

	pathRule_11 := latticemodel.Rule{
		Spec: latticemodel.RuleSpec{
			ServiceName:      ServiceName,
			ServiceNamespace: ServiceNameSpace,
			ListenerPort:     ListenerPort,
			ListenerProtocol: ListenerProtocol,
			RuleID:           weightRuleID,
			PathMatchPrefix:  true,
			PathMatchValue:   "/ver-1",
			Action: latticemodel.RuleAction{
				TargetGroups: []*latticemodel.RuleTargetGroup{
					&WeigthedAction_2,
				},
			},
		},
		Status: &latticemodel.RuleStatus{
			RuleARN:    "ruleARn",
			RuleID:     "rule-id-2-1",
			ListenerID: ListenerID,
			ServiceID:  ServiceID,
		},
	}

	pathRule_2 := latticemodel.Rule{
		Spec: latticemodel.RuleSpec{
			ServiceName:      ServiceName,
			ServiceNamespace: ServiceNameSpace,
			ListenerPort:     ListenerPort,
			ListenerProtocol: ListenerProtocol,
			RuleID:           weightRuleID,
			PathMatchPrefix:  true,
			PathMatchValue:   "/ver-2",
			Action: latticemodel.RuleAction{
				TargetGroups: []*latticemodel.RuleTargetGroup{
					&WeigthedAction_1,
				},
			},
		},
		Status: &latticemodel.RuleStatus{
			RuleARN:    "ruleARn",
			RuleID:     "rule-id-2-1",
			ListenerID: ListenerID,
			ServiceID:  ServiceID,
		},
	}

	headerRule_1 := latticemodel.Rule{
		Spec: latticemodel.RuleSpec{
			ServiceName:        ServiceName,
			ServiceNamespace:   ServiceNameSpace,
			ListenerPort:       ListenerPort,
			ListenerProtocol:   ListenerProtocol,
			RuleID:             weightRuleID,
			PathMatchPrefix:    true,
			PathMatchValue:     "/ver-2",
			NumOfHeaderMatches: 2,
			MatchedHeaders: [5]vpclattice.HeaderMatch{

				{
					Match: &vpclattice.HeaderMatchType{
						Exact: &hdr1Value},
					Name: &hdr1,
				},
				{
					Match: &vpclattice.HeaderMatchType{
						Exact: &hdr2Value},
					Name: &hdr2,
				},

				{},
				{},
				{},
			},
			Action: latticemodel.RuleAction{
				TargetGroups: []*latticemodel.RuleTargetGroup{
					&WeigthedAction_1,
				},
			},
		},
		Status: &latticemodel.RuleStatus{
			RuleARN:    "ruleARn",
			RuleID:     "rule-id-2-1",
			ListenerID: ListenerID,
			ServiceID:  ServiceID,
		},
	}

	tests := []struct {
		name                 string
		oldRule              *latticemodel.Rule
		newRule              *latticemodel.Rule
		listRuleOuput        []*latticemodel.Rule
		createRule           bool
		updateRule           bool
		noServiceID          bool
		noListenerID         bool
		noTargetGroupID      bool
		updatePriorityNeeded bool
	}{

		{
			name:                 "create header-based rule with 1 TG",
			oldRule:              nil,
			newRule:              &headerRule_1,
			createRule:           true,
			updateRule:           false,
			noServiceID:          false,
			noListenerID:         false,
			noTargetGroupID:      false,
			updatePriorityNeeded: false,
		},

		{
			name:                 "test1, create weighted rule with 1 TG",
			oldRule:              nil,
			newRule:              &WeightedRule_1,
			createRule:           true,
			updateRule:           false,
			noServiceID:          false,
			noListenerID:         false,
			noTargetGroupID:      false,
			updatePriorityNeeded: false,
		},

		{
			name:                 "create weighted rule with 2 TGs",
			oldRule:              &WeightedRule_1,
			newRule:              &WeightedRule_1_2,
			createRule:           false,
			updateRule:           true,
			noServiceID:          false,
			noListenerID:         false,
			noTargetGroupID:      false,
			updatePriorityNeeded: false,
		},

		{
			name:                 "update weighted rule with 2 TGs",
			oldRule:              &WeightedRule_1_2,
			newRule:              &WeightedRule_2_1,
			createRule:           false,
			updateRule:           true,
			noServiceID:          false,
			noListenerID:         false,
			noTargetGroupID:      false,
			updatePriorityNeeded: false,
		},

		{
			name:                 "create path-based rule, no need to update priority",
			oldRule:              nil,
			newRule:              &pathRule_1,
			createRule:           true,
			updateRule:           false,
			noServiceID:          false,
			noListenerID:         false,
			noTargetGroupID:      false,
			updatePriorityNeeded: false,
		},

		{

			name:                 "create path-based rule, need to update priority",
			oldRule:              &pathRule_1,
			newRule:              &pathRule_2,
			createRule:           true,
			updateRule:           false,
			noServiceID:          false,
			noListenerID:         false,
			noTargetGroupID:      false,
			updatePriorityNeeded: true,
		},

		{
			name:                 "update path-based rule with a different TG",
			oldRule:              &pathRule_1,
			newRule:              &pathRule_11,
			createRule:           false,
			updateRule:           true,
			noServiceID:          false,
			noListenerID:         false,
			noTargetGroupID:      false,
			updatePriorityNeeded: false,
		},

		{
			name:                 "no serviceID",
			oldRule:              nil,
			newRule:              &pathRule_1,
			createRule:           false,
			updateRule:           false,
			noServiceID:          true,
			noListenerID:         false,
			noTargetGroupID:      false,
			updatePriorityNeeded: false,
		},
		{
			name:                 "no listenerID",
			oldRule:              nil,
			newRule:              &pathRule_1,
			createRule:           false,
			updateRule:           false,
			noServiceID:          false,
			noListenerID:         true,
			noTargetGroupID:      false,
			updatePriorityNeeded: false,
		},

		{
			name:                 "no TG IDs",
			oldRule:              nil,
			newRule:              &pathRule_1,
			createRule:           false,
			updateRule:           false,
			noServiceID:          false,
			noListenerID:         false,
			noTargetGroupID:      true,
			updatePriorityNeeded: false,
		},
	}

	for _, tt := range tests {
		fmt.Printf("testing >>> tt.name: %v\n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		mockVpcLatticeSess := mocks.NewMockLattice(c)

		mockCloud := mocks_aws.NewMockCloud(c)

		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		latticeDataStore := latticestore.NewLatticeDataStore()

		ruleManager := NewRuleManager(mockCloud, latticeDataStore)

		if !tt.noServiceID {
			latticeDataStore.AddLatticeService(tt.newRule.Spec.ServiceName, tt.newRule.Spec.ServiceNamespace, "serviceARN",
				tt.newRule.Status.ServiceID, "test-dns")
		}

		if !tt.noListenerID {
			latticeDataStore.AddListener(tt.newRule.Spec.ServiceName, tt.newRule.Spec.ServiceNamespace,
				tt.newRule.Spec.ListenerPort, "HTTP",
				"listernerARN", tt.newRule.Status.ListenerID)
		}

		if !tt.noTargetGroupID {
			for _, tg := range tt.newRule.Spec.Action.TargetGroups {
				tgName := latticestore.TargetGroupName(tg.Name, tg.Namespace)
				latticeDataStore.AddTargetGroup(tgName, "vpc", "arn", "tg-id", tg.IsServiceImport)
			}

		}

		if !tt.noListenerID && !tt.noServiceID {
			ruleInput := vpclattice.ListRulesInput{
				ListenerIdentifier: aws.String(tt.newRule.Status.ListenerID),
				ServiceIdentifier:  aws.String(tt.newRule.Status.ServiceID),
			}

			ruleOutput := vpclattice.ListRulesOutput{}

			if tt.oldRule != nil {
				items := []*vpclattice.RuleSummary{}

				items = append(items, &vpclattice.RuleSummary{
					Id: aws.String(tt.oldRule.Spec.RuleID),
				})
				ruleOutput = vpclattice.ListRulesOutput{
					Items: items,
				}
			}
			mockVpcLatticeSess.EXPECT().ListRules(&ruleInput).Return(&ruleOutput, nil)

			if tt.oldRule != nil {
				ruleGetInput := vpclattice.GetRuleInput{
					ListenerIdentifier: aws.String(ListenerID),
					ServiceIdentifier:  aws.String(ServiceID),
					RuleIdentifier:     aws.String(tt.oldRule.Spec.RuleID),
				}

				//				listenerID := tt.oldRule.Status.ListenerID
				latticeTGs := []*vpclattice.WeightedTargetGroup{}
				//	ruleName := fmt.Sprintf("rule-%d-%s", tt.oldRule.Spec.CreateTime.Unix(), tt.oldRule.Spec.RuleID)
				priority, _ := ruleID2Priority(tt.oldRule.Spec.RuleID)

				for _, tg := range tt.oldRule.Spec.Action.TargetGroups {
					latticeTG := vpclattice.WeightedTargetGroup{
						TargetGroupIdentifier: aws.String("tg-id"),
						Weight:                aws.Int64(tg.Weight),
					}
					latticeTGs = append(latticeTGs, &latticeTG)
				}

				httpMatch := vpclattice.HttpMatch{}
				updateSDKhttpMatch(&httpMatch, tt.oldRule)
				ruleGetOutput := vpclattice.GetRuleOutput{
					Id:       aws.String(tt.oldRule.Spec.RuleID),
					Priority: aws.Int64(priority),
					Action: &vpclattice.RuleAction{
						Forward: &vpclattice.ForwardAction{
							TargetGroups: latticeTGs,
						},
					},
					Match: &vpclattice.RuleMatch{
						HttpMatch: &httpMatch,
					},
				}

				mockVpcLatticeSess.EXPECT().GetRule(&ruleGetInput).Return(&ruleGetOutput, nil)

			}
		}

		if tt.createRule || tt.updateRule {
			listenerID := tt.newRule.Status.ListenerID
			latticeTGs := []*vpclattice.WeightedTargetGroup{}
			ruleName := fmt.Sprintf("k8s-%d-%s", tt.newRule.Spec.CreateTime.Unix(), tt.newRule.Spec.RuleID)
			priority, _ := ruleID2Priority(tt.newRule.Spec.RuleID)

			if tt.updatePriorityNeeded {
				priority, _ = ruleID2Priority(tt.oldRule.Spec.RuleID)
				priority++
			}

			for _, tg := range tt.newRule.Spec.Action.TargetGroups {
				latticeTG := vpclattice.WeightedTargetGroup{
					TargetGroupIdentifier: aws.String("tg-id"),
					Weight:                aws.Int64(tg.Weight),
				}
				latticeTGs = append(latticeTGs, &latticeTG)
			}

			if tt.createRule {
				httpMatch := vpclattice.HttpMatch{}
				updateSDKhttpMatch(&httpMatch, tt.newRule)
				ruleInput := vpclattice.CreateRuleInput{
					Action: &vpclattice.RuleAction{
						Forward: &vpclattice.ForwardAction{
							TargetGroups: latticeTGs,
						},
					},

					ListenerIdentifier: aws.String(listenerID),
					Name:               aws.String(ruleName),
					Priority:           aws.Int64(priority),
					ServiceIdentifier:  aws.String(ServiceID),
					Match: &vpclattice.RuleMatch{
						HttpMatch: &httpMatch,
					},
				}
				ruleOutput := vpclattice.CreateRuleOutput{
					Id: aws.String(ruleID),
				}
				mockVpcLatticeSess.EXPECT().CreateRule(&ruleInput).Return(&ruleOutput, nil)
			}

			if tt.updateRule {
				ruleInput := vpclattice.UpdateRuleInput{
					Action: &vpclattice.RuleAction{
						Forward: &vpclattice.ForwardAction{
							TargetGroups: latticeTGs,
						},
					},

					ListenerIdentifier: aws.String(listenerID),
					//Name:               aws.String(ruleName),
					RuleIdentifier:    aws.String(tt.newRule.Spec.RuleID),
					Priority:          aws.Int64(priority),
					ServiceIdentifier: aws.String(ServiceID),
					Match: &vpclattice.RuleMatch{
						HttpMatch: &vpclattice.HttpMatch{
							// TODO, what if not specfied this
							//Method: aws.String(vpclattice.HttpMethodGet),
							PathMatch: &vpclattice.PathMatch{
								CaseSensitive: nil,
								Match: &vpclattice.PathMatchType{
									Exact:  nil,
									Prefix: aws.String(tt.newRule.Spec.PathMatchValue),
								},
							},
						},
					},
				}
				ruleOutput := vpclattice.UpdateRuleOutput{
					Id: aws.String(ruleID),
				}
				mockVpcLatticeSess.EXPECT().UpdateRule(&ruleInput).Return(&ruleOutput, nil)
			}
		}

		resp, err := ruleManager.Create(ctx, tt.newRule)

		if !tt.noListenerID && !tt.noServiceID && !tt.noTargetGroupID {
			assert.NoError(t, err)

			assert.Equal(t, resp.ListenerID, ListenerID)
			assert.Equal(t, resp.ServiceID, ServiceID)
			assert.Equal(t, resp.RuleID, ruleID)
		}

		fmt.Printf(" rulemanager.Create :%v, err %d\n", resp, err)

	}
}

func Test_UpdateRule(t *testing.T) {
	tests := []struct {
		name         string
		noServiceID  bool
		noListenerID bool
	}{
		{
			name:         "update",
			noServiceID:  false,
			noListenerID: false,
		},

		{
			name:         "update -- no service",
			noServiceID:  true,
			noListenerID: false,
		},
		{
			name:         "update -- no listenerID",
			noServiceID:  false,
			noListenerID: true,
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		mockVpcLatticeSess := mocks.NewMockLattice(c)

		mockCloud := mocks_aws.NewMockCloud(c)

		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		latticeDataStore := latticestore.NewLatticeDataStore()

		ruleManager := NewRuleManager(mockCloud, latticeDataStore)

		var i = 0
		if !tt.noServiceID {

			latticeDataStore.AddLatticeService(rules[i].Spec.ServiceName, rules[i].Spec.ServiceNamespace, "serviceARN",
				rules[i].Status.ServiceID, "test-dns")
		}

		if !tt.noListenerID {
			latticeDataStore.AddListener(rules[i].Spec.ServiceName, rules[i].Spec.ServiceNamespace,
				rules[i].Spec.ListenerPort, "HTTP",
				"listenerARN", rules[i].Status.ListenerID)
		}

		var ruleUpdateList []*vpclattice.RuleUpdate

		for _, rule := range rules {
			priority, _ := ruleID2Priority(rule.Spec.RuleID)
			ruleupdate := vpclattice.RuleUpdate{
				RuleIdentifier: aws.String(rule.Status.RuleID),
				Priority:       aws.Int64(priority),
			}

			ruleUpdateList = append(ruleUpdateList, &ruleupdate)

		}

		batchRuleInput := vpclattice.BatchUpdateRuleInput{
			ListenerIdentifier: aws.String(rules[0].Status.ListenerID),
			ServiceIdentifier:  aws.String(rules[0].Status.ServiceID),
			Rules:              ruleUpdateList,
		}

		if !tt.noListenerID && !tt.noServiceID {
			var batchRuleOutput vpclattice.BatchUpdateRuleOutput
			mockVpcLatticeSess.EXPECT().BatchUpdateRule(&batchRuleInput).Return(&batchRuleOutput, nil)
		}

		err := ruleManager.Update(ctx, rules)

		if !tt.noListenerID && !tt.noServiceID {
			assert.NoError(t, err)
		} else {
			assert.NotNil(t, err)
		}
	}

}

func Test_List(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockVpcLatticeSess := mocks.NewMockLattice(c)

	mockCloud := mocks_aws.NewMockCloud(c)

	serviceID := "service1-ID"
	listenerID := "listener-ID"

	ruleInput := vpclattice.ListRulesInput{
		ListenerIdentifier: aws.String(listenerID),
		ServiceIdentifier:  aws.String(serviceID),
	}
	ruleOutput := vpclattice.ListRulesOutput{
		Items: []*vpclattice.RuleSummary{
			&vpclattice.RuleSummary{
				Arn:       &rulelist[0].Arn,
				Id:        &rulelist[0].Id,
				IsDefault: &rulelist[0].IsDefault,
			},
			&vpclattice.RuleSummary{
				Arn:       &rulelist[1].Arn,
				Id:        &rulelist[1].Id,
				IsDefault: &rulelist[1].IsDefault,
			},
		},
	}

	latticeDataStore := latticestore.NewLatticeDataStore()

	mockVpcLatticeSess.EXPECT().ListRules(&ruleInput).Return(&ruleOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	ruleManager := NewRuleManager(mockCloud, latticeDataStore)

	resp, err := ruleManager.List(ctx, serviceID, listenerID)

	assert.NoError(t, err)

	for i := 0; i < 2; i++ {
		assert.Equal(t, resp[i].ListenerID, listenerID)
		assert.Equal(t, resp[i].RuleID, rulelist[i].Id)
		assert.Equal(t, resp[i].ServiceID, serviceID)
	}
	fmt.Printf("rule Manager List resp %v\n", resp)

}

func Test_GetRule(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockVpcLatticeSess := mocks.NewMockLattice(c)

	mockCloud := mocks_aws.NewMockCloud(c)

	serviceID := "service1-ID"
	listenerID := "listener-ID"
	ruleID := "rule-ID"
	ruleARN := "rule-ARN"
	rulePriority := int64(10)

	ruleGetInput := vpclattice.GetRuleInput{
		ListenerIdentifier: aws.String(listenerID),
		ServiceIdentifier:  aws.String(serviceID),
		RuleIdentifier:     aws.String(ruleID),
	}

	latticeDataStore := latticestore.NewLatticeDataStore()

	ruleGetOutput := vpclattice.GetRuleOutput{
		Arn:      aws.String(ruleARN),
		Id:       aws.String(ruleID),
		Priority: aws.Int64(int64(rulePriority)),
	}

	mockVpcLatticeSess.EXPECT().GetRule(&ruleGetInput).Return(&ruleGetOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	ruleManager := NewRuleManager(mockCloud, latticeDataStore)

	resp, err := ruleManager.Get(ctx, serviceID, listenerID, ruleID)

	fmt.Printf("resp :%v \n", resp)
	assert.NoError(t, err)
	assert.Equal(t, aws.StringValue(resp.Id), ruleID)
	assert.Equal(t, aws.Int64Value(resp.Priority), rulePriority)

}

func Test_DeleteRule(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockVpcLatticeSess := mocks.NewMockLattice(c)

	mockCloud := mocks_aws.NewMockCloud(c)

	serviceID := "service1-ID"
	listenerID := "listener-ID"
	ruleID := "rule-ID"

	ruleDeleteInput := vpclattice.DeleteRuleInput{
		ServiceIdentifier:  aws.String(serviceID),
		ListenerIdentifier: aws.String(listenerID),
		RuleIdentifier:     aws.String(ruleID),
	}

	latticeDataStore := latticestore.NewLatticeDataStore()

	ruleDeleteOuput := vpclattice.DeleteRuleOutput{}
	mockVpcLatticeSess.EXPECT().DeleteRule(&ruleDeleteInput).Return(&ruleDeleteOuput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	ruleManager := NewRuleManager(mockCloud, latticeDataStore)

	ruleManager.Delete(ctx, ruleID, listenerID, serviceID)

}
