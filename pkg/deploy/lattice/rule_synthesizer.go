package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"

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

	dropped, err := r.upsertRules(ctx, resRule, snlStackRules)
	if err != nil {
		return err
	}

	// for each service/listener, remove any lingering lattice rules
	deleted, err := r.deleteStaleLatticeRules(ctx, snlStackRules)
	if err != nil {
		return err
	}

	// Deleting stale rules frees quota, so the rules that hit it get one more try in this pass. Without a
	// deletion the retry would fail the same way on every requeue; quota freed any other way is picked up by
	// the next requeue.
	if len(dropped) > 0 && deleted > 0 {
		dropped, err = r.upsertRules(ctx, rulesOf(dropped), snlStackRules)
		if err != nil {
			return err
		}
	}

	// now we have a clean set of rules, update priorities accordingly
	err = r.adjustPriorities(ctx, snlStackRules, resRule)
	if err != nil {
		return err
	}

	if len(dropped) > 0 {
		infos := make([]DroppedRule, len(dropped))
		for i, d := range dropped {
			infos[i] = d.info
		}
		return &RuleQuotaExceededError{Dropped: infos, Applied: len(resRule) - len(dropped), Err: dropped[0].err}
	}

	return nil
}

// DroppedRule identifies a route rule that VPC Lattice could not create or update.
type DroppedRule struct {
	RuleIndex        int // position in the Route spec
	RuleName         string
	Priority         int64
	ListenerPort     int64
	ListenerProtocol string
	// NotUpdated is true when the rule already exists and keeps its previous backends, and false when it
	// could not be created.
	NotUpdated bool
}

// RuleQuotaExceededError marks VPC Lattice quota failures while creating or updating listener rules, as
// opposed to a quota failure in some other deploy step. Err is the first quota error returned by VPC Lattice.
type RuleQuotaExceededError struct {
	Dropped []DroppedRule
	// Applied counts the other rules, one per rule and listener like Dropped, that were created or updated.
	Applied int
	Err     error
}

func (e *RuleQuotaExceededError) Error() string {
	return fmt.Sprintf("%d rule(s) could not be created or updated: %s", len(e.Dropped), e.Err)
}

func (e *RuleQuotaExceededError) Unwrap() error { return e.Err }

func AsRuleQuotaExceeded(err error) (*RuleQuotaExceededError, bool) {
	var ruleErr *RuleQuotaExceededError
	if errors.As(err, &ruleErr) {
		return ruleErr, true
	}
	return nil, false
}

// quotaFailedRule pairs a model rule with its quota error, so it can be retried and then reported.
type quotaFailedRule struct {
	rule *model.Rule
	info DroppedRule
	err  error
}

func rulesOf(dropped []quotaFailedRule) []*model.Rule {
	rules := make([]*model.Rule, len(dropped))
	for i, d := range dropped {
		rules[i] = d.rule
	}
	return rules
}

// upsertRules creates or updates each rule and records it in snlRules. Rules that hit a quota are returned
// instead of failing, so every other rule is still applied; any other error aborts.
func (r *ruleSynthesizer) upsertRules(ctx context.Context, rules []*model.Rule, snlRules map[snlKey]ruleIdMap) ([]quotaFailedRule, error) {
	var dropped []quotaFailedRule
	for _, rule := range rules {
		d, err := r.upsertRule(ctx, rule, snlRules)
		if err != nil {
			return nil, err
		}
		if d != nil {
			dropped = append(dropped, *d)
		}
	}
	return dropped, nil
}

func (r *ruleSynthesizer) upsertRule(ctx context.Context, rule *model.Rule, snlRules map[snlKey]ruleIdMap) (*quotaFailedRule, error) {
	stackListener, stackSvc, err := r.getStackObjects(rule)
	if err != nil {
		return nil, err
	}

	err = r.tgManager.ResolveRuleTgIds(ctx, &rule.Spec.Action, r.stack)
	if err != nil {
		return nil, err
	}
	status, err := r.ruleManager.Upsert(ctx, rule, stackListener, stackSvc)

	var quotaErr *ruleQuotaError
	if errors.As(err, &quotaErr) {
		// Create the listener's entry even if nothing on it was applied, so its stale rules are still deleted
		// and stop holding the quota.
		ruleMap := r.ruleMapFor(snlRules, stackSvc, stackListener)
		if quotaErr.notUpdated {
			// keep the live rule out of stale cleanup
			rule.Status = &status
			ruleMap[status.Id] = rule
		}
		return &quotaFailedRule{
			rule: rule,
			err:  quotaErr,
			info: DroppedRule{
				RuleIndex:        rule.Spec.RuleIndex,
				RuleName:         rule.Spec.RuleName,
				Priority:         rule.Spec.Priority,
				ListenerPort:     stackListener.Spec.Port,
				ListenerProtocol: stackListener.Spec.Protocol,
				NotUpdated:       quotaErr.notUpdated,
			},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed RuleManager.Upsert due to %w", err)
	}
	rule.Status = &status

	// build a map svc + listener -> all current rules
	r.ruleMapFor(snlRules, stackSvc, stackListener)[rule.Status.Id] = rule
	return nil, nil
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

// deleteStaleLatticeRules deletes Lattice rules that are no longer in the stack and returns how many it deleted.
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
