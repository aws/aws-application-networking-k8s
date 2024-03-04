package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"

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

// populates all target group ids in the rule's actions
func (r *ruleSynthesizer) resolveRuleTgIds(ctx context.Context, modelRule *model.Rule) error {
	if len(modelRule.Spec.Action.TargetGroups) == 0 {
		r.log.Debugf("no target groups to resolve for rule %d", modelRule.Spec.Priority)
		return nil
	}

	for i, rtg := range modelRule.Spec.Action.TargetGroups {
		if rtg.StackTargetGroupId == "" && rtg.SvcImportTG == nil && rtg.LatticeTgId == "" {
			return errors.New("rule TG is missing a required target group identifier")
		}

		if rtg.LatticeTgId != "" {
			fmt.Printf("liwwu Rule TG %d already resolved %s\n", i, rtg.LatticeTgId)
			r.log.Debugf("Rule TG %d already resolved %s", i, rtg.LatticeTgId)
			continue
		}

		if rtg.StackTargetGroupId != "" {
			if rtg.StackTargetGroupId == model.InvalidBackendRefTgId {
				r.log.Debugf("Rule TG has an invalid backendref, setting TG id to invalid")
				rtg.LatticeTgId = model.InvalidBackendRefTgId
				continue
			}

			r.log.Debugf("Fetching TG %d from the stack (ID %s)", i, rtg.StackTargetGroupId)

			stackTg := &model.TargetGroup{}
			err := r.stack.GetResource(rtg.StackTargetGroupId, stackTg)
			if err != nil {
				return err
			}

			if stackTg.Status == nil {
				return errors.New("stack target group is missing Status field")
			}
			fmt.Printf("liwwu >>> lattice ID %v \n", stackTg.Status.Id)
			rtg.LatticeTgId = stackTg.Status.Id
		}

		if rtg.SvcImportTG != nil {
			r.log.Debugf("Getting target group for service import %s %s (%s, %s)",
				rtg.SvcImportTG.K8SServiceName, rtg.SvcImportTG.K8SServiceNamespace,
				rtg.SvcImportTG.K8SClusterName, rtg.SvcImportTG.VpcId)
			tgId, err := r.findSvcExportTG(ctx, *rtg.SvcImportTG)

			if err != nil {
				return err
			}
			rtg.LatticeTgId = tgId
		}
	}

	return nil
}

func (r *ruleSynthesizer) findSvcExportTG(ctx context.Context, svcImportTg model.SvcImportTargetGroup) (string, error) {
	tgs, err := r.tgManager.List(ctx)
	if err != nil {
		return "", err
	}

	for _, tg := range tgs {
		tgTags := model.TGTagFieldsFromTags(tg.tags)

		svcMatch := tgTags.IsSourceTypeServiceExport() && (tgTags.K8SServiceName == svcImportTg.K8SServiceName) &&
			(tgTags.K8SServiceNamespace == svcImportTg.K8SServiceNamespace)

		clusterMatch := (svcImportTg.K8SClusterName == "") || (tgTags.K8SClusterName == svcImportTg.K8SClusterName)

		vpcMatch := (svcImportTg.VpcId == "") || (svcImportTg.VpcId == aws.StringValue(tg.tgSummary.VpcIdentifier))

		if svcMatch && clusterMatch && vpcMatch {
			return *tg.tgSummary.Id, nil
		}
	}

	return "", errors.New("target group for service import could not be found")
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

	err = r.resolveRuleTgIds(ctx, rule)
	if err != nil {
		return err
	}

	if stackListener.Spec.Protocol == "TLS_PASSTHROUGH" {
		fmt.Printf("liwwu >>> skip update rule, since it is TLS_PASSTHROUGH \n")
		return nil
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
