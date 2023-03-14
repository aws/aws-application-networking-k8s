package lattice

import (
	"context"
	//"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_SynthesizeRule(t *testing.T) {

	//var httpSectionName gateway_api.SectionName = "http"
	var serviceKind gateway_api.Kind = "Service"
	var serviceimportKind gateway_api.Kind = "ServiceImport"
	var weight1 = int32(10)
	var weight2 = int32(90)
	var namespace = gateway_api.Namespace("default")
	var path1 = string("/ver1")
	var path2 = string("/ver2")
	var backendRef1 = gateway_api.BackendRef{
		BackendObjectReference: gateway_api.BackendObjectReference{
			Name:      "targetgroup1",
			Namespace: &namespace,
			Kind:      &serviceKind,
		},
		Weight: &weight1,
	}
	var backendRef2 = gateway_api.BackendRef{
		BackendObjectReference: gateway_api.BackendObjectReference{
			Name:      "targetgroup2",
			Namespace: &namespace,
			Kind:      &serviceimportKind,
		},
		Weight: &weight2,
	}
	/*
		var backendServiceImportRef = gateway_api.BackendRef{
			BackendObjectReference: gateway_api.BackendObjectReference{
				Name: "targetgroup1",
				Kind: &serviceimportKind,
			},
		}
	*/

	tests := []struct {
		name           string
		gwListenerPort gateway_api.PortNumber
		httpRoute      *gateway_api.HTTPRoute
		listenerARN    string
		listenerID     string
		serviceARN     string
		serviceID      string
		updatedTGs     bool
		mgrErr         error
		wantErrIsNil   bool
		wantIsDeleted  bool
	}{
		{
			name:           "Add Rule",
			gwListenerPort: *PortNumberPtr(80),
			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service1",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{

										Value: &path1,
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{

										Value: &path2,
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef2,
								},
							},
						},
					},
				},
			},

			listenerARN:   "arn1234",
			listenerID:    "1234",
			serviceARN:    "arn56789",
			serviceID:     "56789",
			updatedTGs:    false,
			mgrErr:        nil,
			wantIsDeleted: false,
			wantErrIsNil:  true,
		},
		{
			name:           "Add Rule",
			gwListenerPort: *PortNumberPtr(80),
			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service1",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{

										Value: &path1,
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{

										Value: &path2,
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef2,
								},
							},
						},
					},
				},
			},

			listenerARN:   "arn1234",
			listenerID:    "1234",
			serviceARN:    "arn56789",
			serviceID:     "56789",
			updatedTGs:    true,
			mgrErr:        nil,
			wantIsDeleted: false,
			wantErrIsNil:  true,
		},
	}

	var protocol = "HTTP"

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		ds := latticestore.NewLatticeDataStore()

		stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.httpRoute)))

		mockRuleManager := NewMockRuleManager(c)

		var ruleID = 1

		for _, httpRule := range tt.httpRoute.Spec.Rules {
			var ruleValue string
			tgList := []*latticemodel.RuleTargetGroup{}

			if len(httpRule.Matches) != 0 {
				// only handle 1 match for now using  path
				if httpRule.Matches[0].Path.Value != nil {
					ruleValue = *httpRule.Matches[0].Path.Value
				}
			}

			for _, httpBackendRef := range httpRule.BackendRefs {
				ruleTG := latticemodel.RuleTargetGroup{}

				ruleTG.Name = string(httpBackendRef.Name)
				ruleTG.Namespace = string(*httpBackendRef.Namespace)

				if httpBackendRef.Weight != nil {
					ruleTG.Weight = int64(*httpBackendRef.Weight)
				}

				tgList = append(tgList, &ruleTG)
			}

			ruleIDName := fmt.Sprintf("rule-%d", ruleID)
			ruleAction := latticemodel.RuleAction{
				TargetGroups: tgList,
			}
			rule := latticemodel.NewRule(stack, ruleIDName, tt.httpRoute.Name, tt.httpRoute.Namespace, int64(tt.gwListenerPort),
				protocol, latticemodel.MatchByPath, ruleValue, ruleAction)

			var ruleResp latticemodel.RuleStatus

			if tt.updatedTGs {
				ruleResp.UpdatePriorityNeeded = true
			} else {
				ruleResp.UpdatePriorityNeeded = false
			}
			mockRuleManager.EXPECT().Create(ctx, rule).Return(ruleResp, nil)

			ruleID++

		}

		var resRule []*latticemodel.Rule
		stack.ListResources(&resRule)

		if tt.updatedTGs {
			// TODO, resRule return from stack.ListResources is not consistent with the ordering
			// so we use gomock.Any() instead of resRule below
			mockRuleManager.EXPECT().Update(ctx, gomock.Any())
		}

		synthesizer := NewRuleSynthesizer(mockRuleManager, stack, ds)

		err := synthesizer.Synthesize(ctx)

		if tt.wantErrIsNil {
			assert.Nil(t, err)
		}

	}
}

func Test_SynthesizeDeleteRule(t *testing.T) {

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	ds := latticestore.NewLatticeDataStore()

	mockRuleManager := NewMockRuleManager(c)

	var serviceName = "service1"
	var serviceNamespace = "test"
	var serviceID = "service1-id"

	var httpRoute = gateway_api.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceName,
		},
	}

	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(&httpRoute.ObjectMeta)))
	pro := "HTTP"
	protocols := []*string{&pro}
	spec := latticemodel.ServiceSpec{
		Name:      serviceName,
		Namespace: serviceNamespace,
		Protocols: protocols,
	}
	latticemodel.NewLatticeService(stack, "", spec)

	ds.AddLatticeService(serviceName, serviceNamespace, "servicearn", serviceID, "test-dns")

	rule1 := latticemodel.RuleStatus{
		RuleARN:    "rule1-arn",
		RuleID:     "rule1-id",
		Priority:   1,
		ServiceID:  serviceID,
		ListenerID: "listener1-ID",
	}

	rule2 := latticemodel.RuleStatus{
		RuleARN:    "rule2-arn",
		RuleID:     "rule2-id",
		Priority:   2,
		ServiceID:  serviceID,
		ListenerID: "listener1-ID",
	}

	rule3 := latticemodel.RuleStatus{
		RuleARN:    "rule3-arn",
		RuleID:     "rule3-id",
		Priority:   1,
		ServiceID:  serviceID,
		ListenerID: "listener2-ID",
	}

	rule4 := latticemodel.RuleStatus{
		RuleARN:    "rule4-arn",
		RuleID:     "rule4-id",
		Priority:   2,
		ServiceID:  serviceID,
		ListenerID: "listener2-ID",
	}

	listeners := []struct {
		port        int64
		listenerARN string
		listenerID  string

		rulelist []*latticemodel.RuleStatus
	}{
		{
			port:        80,
			listenerARN: "listener1-ARN",
			listenerID:  "listener1-ID",
			rulelist: []*latticemodel.RuleStatus{
				&rule1,
				&rule2,
			},
		},
		{
			port:        443,
			listenerARN: "listener2-ARN",
			listenerID:  "listener2-ID",
			rulelist: []*latticemodel.RuleStatus{
				&rule3,
				&rule4,
			},
		},
	}

	for _, listener := range listeners {
		ds.AddListener(serviceName, serviceNamespace, listener.port, "HTTP",
			listener.listenerARN, listener.listenerID)

		mockRuleManager.EXPECT().List(ctx, serviceID, listener.listenerID).Return(listener.rulelist, nil)

		for _, rule := range listener.rulelist {
			sdkRuleDetail := vpclattice.GetRuleOutput{}

			mockRuleManager.EXPECT().Get(ctx, serviceID, listener.listenerID, rule.RuleID).Return(&sdkRuleDetail, nil)
			mockRuleManager.EXPECT().Delete(ctx, rule.RuleID, listener.listenerID, serviceID)
		}

	}

	synthesizer := NewRuleSynthesizer(mockRuleManager, stack, ds)

	synthesizer.Synthesize(ctx)

}
