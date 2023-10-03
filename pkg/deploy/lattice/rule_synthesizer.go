package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type ruleSynthesizer struct {
	log          gwlog.Logger
	rule         RuleManager
	stack        core.Stack
	latticestore *latticestore.LatticeDataStore
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
		latticestore: store,
	}
}

func (r *ruleSynthesizer) Synthesize(ctx context.Context) error {
	var resRule []*model.Rule

	err := r.stack.ListResources(&resRule)
	if err != nil {
		r.log.Debugf("Error while listing rules %s", err)
	}

	updatePriority := false

	for _, rule := range resRule {
		ruleResp, err := r.rule.Create(ctx, rule)
		if err != nil {
			return err
		}

		if ruleResp.UpdatePriorityNeeded {
			updatePriority = true
		}

		r.log.Debugf("Synthesise rule %s, ruleResp: %+v", rule.Spec.RuleID, ruleResp)
		rule.Status = &ruleResp
	}

	// handle delete
	sdkRules, err := r.getSDKRules(ctx)
	if err != nil {
		r.log.Debugf("Error while getting rules due to %s", err)
	}

	for _, sdkRule := range sdkRules {
		_, err := r.findMatchedRule(ctx, sdkRule.RuleID, sdkRule.ListenerID, sdkRule.ServiceID, resRule)
		if err != nil {
			r.log.Debugf("Error while finding matching rule for service %s, listener %s, rule %s. %s",
				sdkRule.ServiceID, sdkRule.ListenerID, sdkRule.RuleID, err)
			err := r.rule.Delete(ctx, sdkRule.RuleID, sdkRule.ListenerID, sdkRule.ServiceID)
			if err != nil {
				r.log.Debugf("Error while deleting rule for service %s, listener %s, rule %s. %s",
					sdkRule.ServiceID, sdkRule.ListenerID, sdkRule.RuleID, err)
			}
		}
	}

	if updatePriority {
		err := r.rule.Update(ctx, resRule)
		if err != nil {
			r.log.Debugf("Error while updating rule priority for rules %+v. %s", resRule, err)
		}
	}

	return nil
}

func (r *ruleSynthesizer) findMatchedRule(
	ctx context.Context,
	sdkRuleId string,
	listener string,
	service string,
	resRule []*model.Rule,
) (*model.Rule, error) {
	var modelRule *model.Rule = nil
	sdkRuleDetail, err := r.rule.Get(ctx, service, listener, sdkRuleId)
	if err != nil {
		return modelRule, err
	}

	if sdkRuleDetail.Match == nil ||
		sdkRuleDetail.Match.HttpMatch == nil {
		return modelRule, errors.New("rule not found, no HTTPMatch")
	}

	for _, modelRule := range resRule {
		sameRule := isRulesSame(r.log, modelRule, sdkRuleDetail)
		if sameRule {
			return modelRule, nil
		}
	}

	return modelRule, fmt.Errorf("failed to find matching rule in model for rule %s", sdkRuleId)
}

func (r *ruleSynthesizer) getSDKRules(ctx context.Context) ([]*model.RuleStatus, error) {
	var sdkRules []*model.RuleStatus
	var resService []*model.Service
	var resListener []*model.Listener
	var resRule []*model.Rule

	err := r.stack.ListResources(&resService)
	if err != nil {
		r.log.Errorf("Error listing services: %s", err)
	}

	err = r.stack.ListResources(&resListener)
	if err != nil {
		r.log.Errorf("Error listing listeners: %s", err)
	}

	err = r.stack.ListResources(&resRule)
	if err != nil {
		r.log.Errorf("Error listing rules: %s", err)
	}

	for _, service := range resService {
		latticeService, err := r.rule.Cloud().Lattice().FindService(ctx, service)
		if err != nil {
			return sdkRules, fmt.Errorf("failed to find service %s-%s, %s",
				service.Spec.Name, service.Spec.Namespace, err)
		}

		listeners, err := r.latticestore.GetAllListeners(service.Spec.Name, service.Spec.Namespace)
		if err != nil {
			return sdkRules, err
		}

		if len(listeners) == 0 {
			return sdkRules, errors.New("failed to find listener in store")
		}

		for _, listener := range listeners {
			rules, _ := r.rule.List(ctx, aws.StringValue(latticeService.Id), listener.ID)
			sdkRules = append(sdkRules, rules...)
		}
	}

	return sdkRules, nil
}

func (r *ruleSynthesizer) PostSynthesize(ctx context.Context) error {
	return nil
}
