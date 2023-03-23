package lattice

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/glog"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	//"github.com/aws/aws-sdk-go/aws"
)

type ruleSynthesizer struct {
	rule         RuleManager
	stack        core.Stack
	latticestore *latticestore.LatticeDataStore
}

func NewRuleSynthesizer(ruleManager RuleManager, stack core.Stack, store *latticestore.LatticeDataStore) *ruleSynthesizer {
	return &ruleSynthesizer{
		rule:         ruleManager,
		stack:        stack,
		latticestore: store,
	}
}

func (r *ruleSynthesizer) Synthesize(ctx context.Context) error {
	var resRule []*latticemodel.Rule

	err := r.stack.ListResources(&resRule)

	glog.V(6).Infof("Synthesize rule = %v, err :%v \n", resRule, err)
	updatePriority := false

	for _, rule := range resRule {
		ruleResp, err := r.rule.Create(ctx, rule)

		if err != nil {
			glog.V(6).Infof("Failed to create rule %v, err :%v \n", rule, err)
			return err
		}

		if ruleResp.UpdatePriorityNeeded {
			updatePriority = true
		}

		glog.V(6).Infof("Synthesise rule %v, ruleResp:%v \n", rule, ruleResp)
		rule.Status = &ruleResp
	}

	// handle delete
	sdkRules, err := r.getSDKRules(ctx)
	glog.V(2).Infof("rule>>> synthesize,  sdkRules :%v err: %v \n", sdkRules, err)

	for _, sdkrule := range sdkRules {
		_, err := r.findMatchedRule(ctx, sdkrule.RuleID, sdkrule.ListenerID, sdkrule.ServiceID, resRule)

		if err == nil {
			continue
		}

		glog.V(2).Infof("rule-synthersize >>> deleting rule %v\n", *sdkrule)
		r.rule.Delete(ctx, sdkrule.RuleID, sdkrule.ListenerID, sdkrule.ServiceID)
	}

	if updatePriority {
		//r.rule.
		err := r.rule.Update(ctx, resRule)
		glog.V(6).Infof("rule --synthesie update rule priority err: %v\n", err)
	}

	return nil
}

func (r *ruleSynthesizer) findMatchedRule(ctx context.Context, sdkRuleID string, listern string, service string,
	resRule []*latticemodel.Rule) (*latticemodel.Rule, error) {
	var modelRule *latticemodel.Rule = nil

	glog.V(6).Infof("findMatchedRule: skdRuleID %v, listener %v, service %v \n", sdkRuleID, listern, service)
	sdkRuleDetail, err := r.rule.Get(ctx, service, listern, sdkRuleID)

	if err != nil {
		glog.V(6).Infof("findMatchRule, rule not found err:%v\n", err)
		return modelRule, errors.New("rule not found")
	}

	if sdkRuleDetail.Match == nil ||
		sdkRuleDetail.Match.HttpMatch == nil {
		fmt.Println("liwwu >>> no HTTPMatch ")
		return modelRule, errors.New("rule not found")
	}

	for _, modelRule := range resRule {
		sameRule := isRulesSame(modelRule, sdkRuleDetail)

		if !sameRule {
			continue
		}
		/* TODO -- delete
		// Exact Path Match
		if modelRule.Spec.PathMatchExact {
			fmt.Println("liwwu>>> sdk, findMatchedRule PathMatchExact")

			if sdkRuleDetail.Match.HttpMatch.PathMatch == nil ||
				sdkRuleDetail.Match.HttpMatch.PathMatch.Match == nil ||
				sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Exact == nil {
				fmt.Printf("liwwu >> no sdk HTTP PathExact match")
				continue
			}

			if aws.StringValue(sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Exact) != modelRule.Spec.PathMatchValue {
				fmt.Printf("liwwu>>> findMatchedRule, ignore exact path miss ")
				continue
			}

		}

		// Path Prefix
		if modelRule.Spec.PathMatchPrefix {
			fmt.Println("liwwu >>> sdk findMatchRule, PathMatchPrefix")

			if sdkRuleDetail.Match.HttpMatch.PathMatch == nil ||
				sdkRuleDetail.Match.HttpMatch.PathMatch.Match == nil ||
				sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Prefix == nil {
				fmt.Println("liwwu >> no sdk HTTP PathPrefix")
				continue
			}

			if aws.StringValue(sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Prefix) != modelRule.Spec.PathMatchValue {
				fmt.Printf("liwwu >>PathMatchPrefix ignore prefix path ")
				continue
			}
		}

		// Header Match

		if modelRule.Spec.NumOfHeaderMatches > 0 {
			fmt.Printf("liwwu >>> numofheader matches %v \n", modelRule.Spec.NumOfHeaderMatches)
			if len(sdkRuleDetail.Match.HttpMatch.HeaderMatches) != modelRule.Spec.NumOfHeaderMatches {
				fmt.Printf("liwwu>> header match number mismatch")
				continue
			}

			misMatch := false

			// compare 2 array
			for _, sdkHeader := range sdkRuleDetail.Match.HttpMatch.HeaderMatches {
				fmt.Printf("sdkHeader >> %v\n", sdkHeader)
				matchFound := false
				// check if this is in module
				for i := 0; i < modelRule.Spec.NumOfHeaderMatches; i++ {
					// compare header
					if aws.StringValue(modelRule.Spec.MatchedHeaders[i].Name) ==
						aws.StringValue(sdkHeader.Name) &&
						aws.StringValue(modelRule.Spec.MatchedHeaders[i].Match.Exact) ==
							aws.StringValue(sdkHeader.Match.Exact) {
						matchFound = true
						break
					}

				}

				if !matchFound {
					misMatch = true
					fmt.Printf("liwwu >> header not found sdkHeader %v\n", *sdkHeader)
					break
				}
			}

			if misMatch {
				fmt.Println("mismatch header")
				continue
			}
		}
		*/

		glog.V(6).Infof("findMatchedRule: found matched modelRule %v \n", modelRule)
		return modelRule, nil
	}

	glog.V(6).Infof("findMatchedRule, sdk rule %v not found in model rules %v \n", sdkRuleID, resRule)
	return modelRule, errors.New("failed to find matching rule in model")
}

func (r *ruleSynthesizer) getSDKRules(ctx context.Context) ([]*latticemodel.RuleStatus, error) {
	var sdkRules []*latticemodel.RuleStatus
	var resService []*latticemodel.Service
	var resListener []*latticemodel.Listener
	var resRule []*latticemodel.Rule

	err := r.stack.ListResources(&resService)

	glog.V(6).Infof("getSDKRules service: %v err: %v \n", resService, err)

	err = r.stack.ListResources(&resListener)

	glog.V(6).Infof("getSDKRules, listener: %v err: %v \n ", resListener, err)

	err = r.stack.ListResources(&resRule)
	glog.V(6).Infof("getSDKRules, rule %v err %v \n", resRule, err)

	for _, service := range resService {
		latticeService, err := r.latticestore.GetLatticeService(service.Spec.Name, service.Spec.Namespace)

		if err != nil {
			glog.V(6).Infof("getSDKRules: failed to find service in store service %v, err %v \n", service, err)
			return sdkRules, errors.New("getSDKRules: failed to find service in store")
		}

		listeners, err := r.latticestore.GetAllListeners(service.Spec.Name, service.Spec.Namespace)

		if len(listeners) == 0 {
			glog.V(6).Infof("getSDKRules, no listeners in store service %v \n", service)
			return sdkRules, errors.New("failed to find listener in store")

		}

		for _, listener := range listeners {
			rules, _ := r.rule.List(ctx, latticeService.ID, listener.ID)

			sdkRules = append(sdkRules, rules...)

		}
	}

	return sdkRules, nil

}

func (r *ruleSynthesizer) PostSynthesize(ctx context.Context) error {
	return nil
}
