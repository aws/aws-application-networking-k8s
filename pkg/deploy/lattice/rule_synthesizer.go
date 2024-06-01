package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type ruleSynthesizer struct {
	log         gwlog.Logger
	ruleManager RuleManager
	tgManager   TargetGroupManager
	stack       core.Stack
}

func NewRuleSynthesizer(
	log gwlog.Logger,
	ruleManager RuleManager,
	tgManager TargetGroupManager,
	stack core.Stack,
) *ruleSynthesizer {
	return &ruleSynthesizer{
		log:         log,
		ruleManager: ruleManager,
		tgManager:   tgManager,
		stack:       stack,
	}
}

// helper types for checking which leftover rules are no longer referenced
// and need to be deleted
type ruleIdMap map[string]*model.Rule
type snlKey struct {
	SvcId      string
	ListenerId string
}

func (r *ruleSynthesizer) Synthesize(ctx context.Context) error {
	var resRule []*model.Rule

	err := r.stack.ListResources(&resRule)
	if err != nil {
		return err
	}

	// svc id -> listener id -> rule id
	snlStackRules := make(map[snlKey]ruleIdMap)

	for _, rule := range resRule {
		// this will also populate our map with rules for each service+listener
		err = r.createOrUpdateRules(ctx, rule, snlStackRules)
		if err != nil {
			return err
		}
	}

	// for each service/listener, remove any lingering lattice rules
	err = r.deleteStaleLatticeRules(ctx, snlStackRules)
	if err != nil {
		return err
	}

	// now we have a clean set of rules, update priorities accordingly
	err = r.adjustPriorities(ctx, snlStackRules, resRule)
	if err != nil {
		return err
	}

	return nil
}

func (r *ruleSynthesizer) createOrUpdateRules(ctx context.Context, rule *model.Rule, snlRules map[snlKey]ruleIdMap) error {
	stackListener, stackSvc, err := r.getStackObjects(rule)
	if err != nil {
		return err
	}

	if stackListener.Spec.Protocol == vpclattice.ListenerProtocolTlsPassthrough {
		r.log.Debugf("Skip updating rule=%v, since TLS_PASSTHROUGH listener can only have one default action and without any other additional rule", *rule)
		return nil
	}

	err = r.tgManager.ResolveRuleTgIds(ctx, rule, r.stack)
	if err != nil {
		return err
	}
	status, err := r.ruleManager.Upsert(ctx, rule, stackListener, stackSvc)
	if err != nil {
		return fmt.Errorf("Failed RuleManager.Upsert due to %s", err)
	}
	rule.Status = &status

	// build a map svc + listener -> all current rules
	key := snlKey{
		SvcId:      stackSvc.Status.Id,
		ListenerId: stackListener.Status.Id,
	}
	var ok bool
	var ruleMap ruleIdMap
	if ruleMap, ok = snlRules[key]; !ok {
		// create and add a map if there isn't one already
		ruleMap = make(ruleIdMap)
		snlRules[key] = ruleMap
	}

	ruleMap[rule.Status.Id] = rule
	return nil
}

func (r *ruleSynthesizer) deleteStaleLatticeRules(ctx context.Context, snlRules map[snlKey]ruleIdMap) error {
	var delErr error
	for snl := range snlRules {
		allLatticeRules, err := r.ruleManager.List(ctx, snl.SvcId, snl.ListenerId)
		if err != nil {
			return fmt.Errorf("failed RuleManager.List %s/%s, due to %s", snl.SvcId, snl.ListenerId, err)
		}

		activeRules := snlRules[snl]
		for _, lr := range allLatticeRules {
			if aws.BoolValue(lr.IsDefault) {
				continue
			}

			// if the rule is not in our list of ids, we need to remove it
			// make sure to skip the default
			ruleId := aws.StringValue(lr.Id)
			if _, ok := activeRules[ruleId]; !ok {
				err := r.ruleManager.Delete(ctx, ruleId, snl.SvcId, snl.ListenerId)
				if err != nil {
					delErr = errors.Join(delErr,
						fmt.Errorf("failed RuleManager.Delete %s/%s/%s, due to %s", snl.SvcId, snl.ListenerId, ruleId, err))
				}
			}
		}
	}
	return delErr
}

func (r *ruleSynthesizer) adjustPriorities(ctx context.Context, snlStackRules map[snlKey]ruleIdMap, resRule []*model.Rule) error {
	var updateErr error
	for snl := range snlStackRules {
		activeRules := snlStackRules[snl]
		for _, rule := range activeRules {
			if rule.Spec.Priority != rule.Status.Priority {
				// *any* mismatch in priority prompts a batch update of ALL priorities
				r.log.Debugf("Found rule priority mismatch, update required")

				var rulesToUpdate []*model.Rule
				for _, snlRule := range activeRules {
					rulesToUpdate = append(rulesToUpdate, snlRule)
				}

				err := r.ruleManager.UpdatePriorities(ctx, snl.SvcId, snl.ListenerId, rulesToUpdate)
				if err != nil {
					updateErr = errors.Join(updateErr,
						fmt.Errorf("failed RuleManager.UpdatePriorities for rules %+v due to %s", resRule, err))
				}
				break
			}
		}
	}

	return updateErr
}

func (r *ruleSynthesizer) getStackObjects(rule *model.Rule) (*model.Listener, *model.Service, error) {
	listener := &model.Listener{}
	err := r.stack.GetResource(rule.Spec.StackListenerId, listener)
	if err != nil {
		return nil, nil, err
	}

	svc := &model.Service{}
	err = r.stack.GetResource(listener.Spec.StackServiceId, svc)
	if err != nil {
		return nil, nil, err
	}

	return listener, svc, nil
}

func (r *ruleSynthesizer) PostSynthesize(ctx context.Context) error {
	return nil
}
