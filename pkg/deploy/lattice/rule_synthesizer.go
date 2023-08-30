package lattice

import (
	"context"
	"errors"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type ruleSynthesizer struct {
	log          gwlog.Logger
	rule         RuleManager
	stack        core.Stack
	latticeStore *latticestore.LatticeDataStore
}

func NewRuleSynthesizer(
	log gwlog.Logger,
	ruleManager RuleManager,
	stack core.Stack,
	store *latticestore.LatticeDataStore,
) *ruleSynthesizer {
	return &ruleSynthesizer{
		log:          log,
		rule:         ruleManager,
		stack:        stack,
		latticeStore: store,
	}
}

func (r *ruleSynthesizer) Synthesize(ctx context.Context) error {
	var resRule []*latticemodel.Rule

	err := r.stack.ListResources(&resRule)

	r.log.Infof("Synthesize rule = %v, err :%v", resRule, err)
	updatePriority := false

	for _, rule := range resRule {
		ruleResp, err := r.rule.Create(ctx, rule)

		if err != nil {
			r.log.Infof("Failed to create rule %v, err :%v", rule, err)
			return err
		}

		if ruleResp.UpdatePriorityNeeded {
			updatePriority = true
		}

		r.log.Infof("Synthesise rule %v, ruleResp:%v", rule, ruleResp)
		rule.Status = &ruleResp
	}

	// handle delete
	sdkRules, err := r.getSDKRules(ctx)
	r.log.Infof("rule>>> synthesize,  sdkRules :%v err: %v", sdkRules, err)

	for _, sdkrule := range sdkRules {
		_, err := r.findMatchedRule(ctx, sdkrule.RuleID, sdkrule.ListenerID, sdkrule.ServiceID, resRule)

		if err == nil {
			continue
		}

		r.log.Debugf("rule-synthersize >>> deleting rule %v", *sdkrule)
		r.rule.Delete(ctx, sdkrule.RuleID, sdkrule.ListenerID, sdkrule.ServiceID)
	}

	if updatePriority {
		//r.rule.
		err := r.rule.Update(ctx, resRule)
		r.log.Infof("rule --synthesie update rule priority err: %v", err)
	}

	return nil
}

func (r *ruleSynthesizer) findMatchedRule(ctx context.Context, sdkRuleID string, listern string, service string,
	resRule []*latticemodel.Rule) (*latticemodel.Rule, error) {
	var modelRule *latticemodel.Rule = nil

	r.log.Infof("findMatchedRule: skdRuleID %v, listener %v, service %v", sdkRuleID, listern, service)
	sdkRuleDetail, err := r.rule.Get(ctx, service, listern, sdkRuleID)

	if err != nil {
		r.log.Infof("findMatchRule, rule not found err:%v", err)
		return modelRule, errors.New("rule not found")
	}

	if sdkRuleDetail.Match == nil ||
		sdkRuleDetail.Match.HttpMatch == nil {
		r.log.Infof("no HTTPMatch ")
		return modelRule, errors.New("rule not found")
	}

	for _, modelRule := range resRule {
		sameRule := isRulesSame(r.log, modelRule, sdkRuleDetail)

		if !sameRule {
			continue
		}

		r.log.Infof("findMatchedRule: found matched modelRule %v", modelRule)
		return modelRule, nil
	}

	r.log.Infof("findMatchedRule, sdk rule %v not found in model rules %v", sdkRuleID, resRule)
	return modelRule, errors.New("failed to find matching rule in model")
}

func (r *ruleSynthesizer) getSDKRules(ctx context.Context) ([]*latticemodel.RuleStatus, error) {
	var sdkRules []*latticemodel.RuleStatus
	var resService []*latticemodel.Service
	var resListener []*latticemodel.Listener
	var resRule []*latticemodel.Rule

	err := r.stack.ListResources(&resService)

	r.log.Infof("getSDKRules service: %v err: %v", resService, err)

	err = r.stack.ListResources(&resListener)

	r.log.Infof("getSDKRules, listener: %v err: %v ", resListener, err)

	err = r.stack.ListResources(&resRule)
	r.log.Infof("getSDKRules, rule %v err %v", resRule, err)

	for _, service := range resService {
		latticeService, err := r.latticeStore.GetLatticeService(service.Spec.Name, service.Spec.Namespace)

		if err != nil {
			r.log.Infof("getSDKRules: failed to find service in store service %v, err %v", service, err)
			return sdkRules, errors.New("getSDKRules: failed to find service in store")
		}

		listeners, err := r.latticeStore.GetAllListeners(service.Spec.Name, service.Spec.Namespace)

		if len(listeners) == 0 {
			r.log.Infof("getSDKRules, no listeners in store service %v", service)
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
