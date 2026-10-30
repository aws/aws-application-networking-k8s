package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
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

	// quota failures are collected so that every rule that can be programmed still is, and the caller can
	// report all rules that were dropped; any other error aborts immediately
	var dropped []droppedRule
	for _, rule := range resRule {
		// this will also populate our map with rules for each service+listener
		err = r.createOrUpdateRules(ctx, rule, snlStackRules)
		if err != nil {
			if dre, ok := asDroppedRuleError(err); ok {
				dropped = append(dropped, droppedRule{rule: rule, err: dre})
				continue
			}
			return err
		}
	}

	// for each service/listener, remove any lingering lattice rules. This also frees quota held by rules
	// that are no longer in the Route.
	deleted, err := r.deleteStaleLatticeRules(ctx, snlStackRules)
	if err != nil {
		return err
	}

	// if cleanup freed quota, give the dropped rules one more chance within this pass
	if len(dropped) > 0 && deleted > 0 {
		var stillDropped []droppedRule
		for _, d := range dropped {
			err = r.createOrUpdateRules(ctx, d.rule, snlStackRules)
			if err == nil {
				continue
			}
			if dre, ok := asDroppedRuleError(err); ok {
				stillDropped = append(stillDropped, droppedRule{rule: d.rule, err: dre})
				continue
			}
			return err
		}
		dropped = stillDropped
	}

	// now we have a clean set of rules, update priorities accordingly
	err = r.adjustPriorities(ctx, snlStackRules, resRule)
	if err != nil {
		return err
	}

	if len(dropped) > 0 {
		infos := make([]services.DroppedRule, len(dropped))
		for i, d := range dropped {
			infos[i] = d.err.rule
		}
		return &services.RuleQuotaExceededError{Dropped: infos, Err: dropped[0].err.err}
	}

	return nil
}

type droppedRule struct {
	rule *model.Rule
	err  *droppedRuleError
}

// droppedRuleError carries a rule that hit a VPC Lattice quota between createOrUpdateRules and Synthesize.
type droppedRuleError struct {
	err  error
	rule services.DroppedRule
}

func (e *droppedRuleError) Error() string {
	return fmt.Sprintf("failed RuleManager.Upsert due to %s", e.err)
}

func (e *droppedRuleError) Unwrap() error { return e.err }

func asDroppedRuleError(err error) (*droppedRuleError, bool) {
	var dre *droppedRuleError
	if errors.As(err, &dre) {
		return dre, true
	}
	return nil, false
}

func (r *ruleSynthesizer) createOrUpdateRules(ctx context.Context, rule *model.Rule, snlRules map[snlKey]ruleIdMap) error {
	stackListener, stackSvc, err := r.getStackObjects(rule)
	if err != nil {
		return err
	}

	err = r.tgManager.ResolveRuleTgIds(ctx, &rule.Spec.Action, r.stack)
	if err != nil {
		return err
	}
	status, err := r.ruleManager.Upsert(ctx, rule, stackListener, stackSvc)
	if err != nil {
		// Only CreateRule errors keep their type (rule_manager.create wraps with %w), so a rule that already exists
		// in Lattice is never classified as dropped and then deleted as stale.
		if services.IsServiceQuotaExceededError(err) {
			// make sure stale rules are cleaned up even if every rule on this listener was dropped,
			// otherwise they would keep holding the quota
			r.ruleMapFor(snlRules, stackSvc, stackListener)
			return &droppedRuleError{
				err: err,
				rule: services.DroppedRule{
					RuleIndex:        rule.Spec.RuleIndex,
					RuleName:         rule.Spec.RuleName,
					Priority:         rule.Spec.Priority,
					ListenerPort:     stackListener.Spec.Port,
					ListenerProtocol: stackListener.Spec.Protocol,
				},
			}
		}
		return fmt.Errorf("failed RuleManager.Upsert due to %w", err)
	}
	rule.Status = &status

	// build a map svc + listener -> all current rules
	r.ruleMapFor(snlRules, stackSvc, stackListener)[rule.Status.Id] = rule
	return nil
}

// ruleMapFor returns the map of current rules for a service+listener, creating it if there isn't one already.
func (r *ruleSynthesizer) ruleMapFor(snlRules map[snlKey]ruleIdMap, svc *model.Service, listener *model.Listener) ruleIdMap {
	key := snlKey{
		SvcId:      svc.Status.Id,
		ListenerId: listener.Status.Id,
	}
	ruleMap, ok := snlRules[key]
	if !ok {
		ruleMap = make(ruleIdMap)
		snlRules[key] = ruleMap
	}
	return ruleMap
}

func (r *ruleSynthesizer) deleteStaleLatticeRules(ctx context.Context, snlRules map[snlKey]ruleIdMap) (int, error) {
	var delErr error
	deleted := 0
	for snl := range snlRules {
		allLatticeRules, err := r.ruleManager.List(ctx, snl.SvcId, snl.ListenerId)
		if err != nil {
			return deleted, fmt.Errorf("failed RuleManager.List %s/%s, due to %s", snl.SvcId, snl.ListenerId, err)
		}

		activeRules := snlRules[snl]
		for _, lr := range allLatticeRules {
			if aws.ToBool(lr.IsDefault) {
				continue
			}

			// if the rule is not in our list of ids, we need to remove it
			// make sure to skip the default
			ruleId := aws.ToString(lr.Id)
			if _, ok := activeRules[ruleId]; !ok {
				err := r.ruleManager.Delete(ctx, ruleId, snl.SvcId, snl.ListenerId)
				if err != nil {
					delErr = errors.Join(delErr,
						fmt.Errorf("failed RuleManager.Delete %s/%s/%s, due to %s", snl.SvcId, snl.ListenerId, ruleId, err))
				} else {
					deleted++
				}
			}
		}
	}
	return deleted, delErr
}

func (r *ruleSynthesizer) adjustPriorities(ctx context.Context, snlStackRules map[snlKey]ruleIdMap, resRule []*model.Rule) error {
	var updateErr error
	for snl := range snlStackRules {
		activeRules := snlStackRules[snl]
		for _, rule := range activeRules {
			if rule.Spec.Priority != rule.Status.Priority {
				// *any* mismatch in priority prompts a batch update of ALL priorities
				r.log.Debugf(ctx, "Found rule priority mismatch, update required")

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
