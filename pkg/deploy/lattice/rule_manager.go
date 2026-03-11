package lattice

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

//go:generate mockgen -destination rule_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice RuleManager

type RuleManager interface {
	Upsert(ctx context.Context, modelRule *model.Rule, modelListener *model.Listener, modelSvc *model.Service) (model.RuleStatus, error)
	Delete(ctx context.Context, ruleId string, serviceId string, listenerId string) error
	UpdatePriorities(ctx context.Context, svcId string, listenerId string, rules []*model.Rule) error
	List(ctx context.Context, serviceId string, listenerId string) ([]*vpclattice.RuleSummary, error)
	Get(ctx context.Context, serviceId string, listenerId string, ruleId string) (*vpclattice.GetRuleOutput, error)
}

type defaultRuleManager struct {
	log   gwlog.Logger
	cloud pkg_aws.Cloud
}

func NewRuleManager(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
) *defaultRuleManager {
	return &defaultRuleManager{
		log:   log,
		cloud: cloud,
	}
}

func (r *defaultRuleManager) Get(ctx context.Context, serviceId string, listenerId string, ruleId string) (*vpclattice.GetRuleOutput, error) {
	getRuleInput := vpclattice.GetRuleInput{
		ListenerIdentifier: aws.String(listenerId),
		ServiceIdentifier:  aws.String(serviceId),
		RuleIdentifier:     aws.String(ruleId),
	}

	resp, err := r.cloud.Lattice().GetRuleWithContext(ctx, &getRuleInput)
	return resp, err
}

func (r *defaultRuleManager) List(ctx context.Context, svcId string, listenerId string) ([]*vpclattice.RuleSummary, error) {
	ruleListInput := vpclattice.ListRulesInput{
		ServiceIdentifier:  aws.String(svcId),
		ListenerIdentifier: aws.String(listenerId),
	}

	return r.cloud.Lattice().ListRulesAsList(ctx, &ruleListInput)
}

func (r *defaultRuleManager) UpdatePriorities(ctx context.Context, svcId string, listenerId string, rules []*model.Rule) error {
	var ruleUpdateList []*vpclattice.RuleUpdate

	for _, rule := range rules {
		ruleUpdate := vpclattice.RuleUpdate{
			RuleIdentifier: aws.String(rule.Status.Id),
			Priority:       aws.Int64(rule.Spec.Priority),
		}

		ruleUpdateList = append(ruleUpdateList, &ruleUpdate)
	}

	// BatchUpdate rules using right priority
	batchRuleInput := vpclattice.BatchUpdateRuleInput{
		ServiceIdentifier:  aws.String(svcId),
		ListenerIdentifier: aws.String(listenerId),
		Rules:              ruleUpdateList,
	}

	_, err := r.cloud.Lattice().BatchUpdateRuleWithContext(ctx, &batchRuleInput)
	if err != nil {
		return fmt.Errorf("failed BatchUpdateRule %s, %s, due to %s", svcId, listenerId, err)
	}

	r.log.Infof(ctx, "Success BatchUpdateRule %s, %s", svcId, listenerId)
	return nil
}

func (r *defaultRuleManager) buildLatticeRule(modelRule *model.Rule) (*vpclattice.GetRuleOutput, error) {
	gro := vpclattice.GetRuleOutput{
		IsDefault: aws.Bool(false),
		Priority:  aws.Int64(modelRule.Spec.Priority),
	}

	httpMatch := vpclattice.HttpMatch{}
	updateMatchFromRule(&httpMatch, modelRule)
	gro.Match = &vpclattice.RuleMatch{HttpMatch: &httpMatch}

	// check if we have at least one valid target group
	var hasValidTargetGroup bool
	for _, tg := range modelRule.Spec.Action.TargetGroups {
		if tg.LatticeTgId != model.InvalidBackendRefTgId {
			hasValidTargetGroup = true
			break
		}
	}

	if hasValidTargetGroup {
		var latticeTGs []*vpclattice.WeightedTargetGroup
		for _, ruleTg := range modelRule.Spec.Action.TargetGroups {
			// skip any invalid TGs - eventually VPC Lattice may support weighted fixed response
			// and this logic can be more in line with the spec
			if ruleTg.LatticeTgId == model.InvalidBackendRefTgId {
				continue
			}

			latticeTG := vpclattice.WeightedTargetGroup{
				TargetGroupIdentifier: aws.String(ruleTg.LatticeTgId),
				Weight:                aws.Int64(ruleTg.Weight),
			}

			latticeTGs = append(latticeTGs, &latticeTG)
		}

		gro.Action = &vpclattice.RuleAction{
			Forward: &vpclattice.ForwardAction{
				TargetGroups: latticeTGs,
			},
		}
	} else {
		r.log.Debugf(context.TODO(), "There are no valid target groups, defaulting to 500 Fixed response")
		gro.Action = &vpclattice.RuleAction{
			FixedResponse: &vpclattice.FixedResponseAction{
				StatusCode: aws.Int64(model.InvalidBackendRefFixedResponseStatusCode),
			},
		}
	}

	gro.Name = aws.String(fmt.Sprintf("k8s-%d-rule-%d", modelRule.Spec.CreateTime.Unix(), modelRule.Spec.Priority))
	return &gro, nil
}

func (r *defaultRuleManager) Upsert(
	ctx context.Context,
	modelRule *model.Rule,
	modelListener *model.Listener,
	modelSvc *model.Service,
) (model.RuleStatus, error) {
	if modelListener.Status == nil || modelListener.Status.Id == "" {
		return model.RuleStatus{}, errors.New("listener is missing id")
	}
	if modelSvc.Status == nil || modelSvc.Status.Id == "" {
		return model.RuleStatus{}, errors.New("model service is missing id")
	}
	for i, mtg := range modelRule.Spec.Action.TargetGroups {
		if mtg.LatticeTgId == "" {
			return model.RuleStatus{}, fmt.Errorf("rule %d action %d is missing lattice target group id", modelRule.Spec.Priority, i)
		}
	}
	latticeServiceId := modelSvc.Status.Id
	latticeListenerId := modelListener.Status.Id

	// this allows us to make apples to apples comparisons with what's in Lattice already
	latticeRuleFromModel, err := r.buildLatticeRule(modelRule)
	if err != nil {
		return model.RuleStatus{}, err
	}

	r.log.Debugf(ctx, "Upsert rule %s for service %s-%s and listener port %d and protocol %s",
		aws.StringValue(latticeRuleFromModel.Name), latticeServiceId, latticeListenerId,
		modelListener.Spec.Port, modelListener.Spec.Protocol)

	lri := vpclattice.ListRulesInput{
		ServiceIdentifier:  aws.String(modelSvc.Status.Id),
		ListenerIdentifier: aws.String(modelListener.Status.Id),
	}
	// TODO: fetching all rules every time is not efficient - maybe have a separate public method to prepopulate?
	currentLatticeRules, err := r.cloud.Lattice().GetRulesAsList(ctx, &lri)
	if err != nil {
		return model.RuleStatus{}, err
	}

	var matchingRule *vpclattice.GetRuleOutput
	for _, clr := range currentLatticeRules {
		if isMatchEqual(latticeRuleFromModel, clr) {
			matchingRule = clr
			break
		}
	}

	if matchingRule == nil {
		return r.create(ctx, currentLatticeRules, latticeRuleFromModel, latticeServiceId, latticeListenerId, modelRule)
	} else {
		return r.updateIfNeeded(ctx, latticeRuleFromModel, matchingRule, latticeServiceId, latticeListenerId, modelRule, modelSvc)
	}
}

func (r *defaultRuleManager) updateIfNeeded(
	ctx context.Context,
	ruleToUpdate *vpclattice.GetRuleOutput,
	matchingRule *vpclattice.GetRuleOutput,
	latticeSvcId string,
	latticeListenerId string,
	modelRule *model.Rule,
	modelSvc *model.Service,
) (model.RuleStatus, error) {
	updatedRuleStatus := model.RuleStatus{
		Name:       aws.StringValue(matchingRule.Name),
		Arn:        aws.StringValue(matchingRule.Arn),
		Id:         aws.StringValue(matchingRule.Id),
		ListenerId: latticeListenerId,
		ServiceId:  latticeSvcId,
		Priority:   aws.Int64Value(matchingRule.Priority),
	}

	var awsManagedTags services.Tags
	if modelSvc.Spec.AllowTakeoverFrom != "" {
		awsManagedTags = services.Tags{
			pkg_aws.TagManagedBy: r.cloud.DefaultTags()[pkg_aws.TagManagedBy],
		}
	}

	err := r.cloud.Tagging().UpdateTags(ctx, aws.StringValue(matchingRule.Arn), modelRule.Spec.AdditionalTags, awsManagedTags)
	if err != nil {
		return model.RuleStatus{}, fmt.Errorf("failed to update tags for rule %s: %w", aws.StringValue(matchingRule.Id), err)
	}

	// we already validated Match, if Action is also the same then no updates required
	updateNeeded := !reflect.DeepEqual(ruleToUpdate.Action, matchingRule.Action)
	if !updateNeeded {
		r.log.Debugf(ctx, "rule unchanged, no updates required")
		return updatedRuleStatus, nil
	}

	// when we update a rule, we use the priority of the existing rule to avoid conflicts
	ruleToUpdate.Priority = matchingRule.Priority
	ruleToUpdate.Id = matchingRule.Id

	uri := vpclattice.UpdateRuleInput{
		Action:             ruleToUpdate.Action,
		ServiceIdentifier:  aws.String(latticeSvcId),
		ListenerIdentifier: aws.String(latticeListenerId),
		RuleIdentifier:     ruleToUpdate.Id,
		Match:              ruleToUpdate.Match,
		Priority:           ruleToUpdate.Priority,
	}

	_, err = r.cloud.Lattice().UpdateRuleWithContext(ctx, &uri)
	if err != nil {
		return model.RuleStatus{}, fmt.Errorf("failed UpdateRule %d for %s, %s due to %s",
			ruleToUpdate.Priority, latticeListenerId, latticeSvcId, err)
	}

	r.log.Infof(ctx, "Success UpdateRule %d for %s, %s", ruleToUpdate.Priority, latticeListenerId, latticeSvcId)
	return updatedRuleStatus, nil
}

func (r *defaultRuleManager) create(
	ctx context.Context,
	currentLatticeRules []*vpclattice.GetRuleOutput,
	ruleToCreate *vpclattice.GetRuleOutput,
	latticeSvcId string,
	latticeListenerId string,
	modelRule *model.Rule,
) (model.RuleStatus, error) {
	// when we create a rule, we just pick an available priority so we can
	// successfully create the rule. After all rules are created, we update
	// priorities based on the order they appear in the Route. Note, this
	// approach is not fully compliant with the gw spec
	priority, err := r.nextAvailablePriority(currentLatticeRules)
	if err != nil {
		return model.RuleStatus{}, err
	}
	ruleToCreate.Priority = aws.Int64(priority)

	tags := r.cloud.MergeTags(r.cloud.DefaultTags(), modelRule.Spec.AdditionalTags)

	cri := vpclattice.CreateRuleInput{
		Action:             ruleToCreate.Action,
		ServiceIdentifier:  aws.String(latticeSvcId),
		ListenerIdentifier: aws.String(latticeListenerId),
		Match:              ruleToCreate.Match,
		Name:               ruleToCreate.Name,
		Priority:           ruleToCreate.Priority,
		Tags:               tags,
	}

	res, err := r.cloud.Lattice().CreateRuleWithContext(ctx, &cri)
	if err != nil {
		return model.RuleStatus{}, fmt.Errorf("failed CreateRule %s, %s due to %s", latticeListenerId, latticeSvcId, err)
	}

	r.log.Infof(ctx, "Success CreateRule %s, %s", aws.StringValue(res.Name), aws.StringValue(res.Id))

	return model.RuleStatus{
		Name:       aws.StringValue(res.Name),
		Arn:        aws.StringValue(res.Arn),
		Id:         aws.StringValue(res.Id),
		ServiceId:  latticeSvcId,
		ListenerId: latticeListenerId,
		Priority:   aws.Int64Value(res.Priority),
	}, nil
}

func updateMatchFromRule(httpMatch *vpclattice.HttpMatch, modelRule *model.Rule) {
	// setup path based
	if modelRule.Spec.PathMatchExact || modelRule.Spec.PathMatchPrefix {
		matchType := vpclattice.PathMatchType{}
		if modelRule.Spec.PathMatchExact {
			matchType.Exact = aws.String(modelRule.Spec.PathMatchValue)
		}
		if modelRule.Spec.PathMatchPrefix {
			matchType.Prefix = aws.String(modelRule.Spec.PathMatchValue)
		}

		httpMatch.PathMatch = &vpclattice.PathMatch{
			Match:         &matchType,
			CaseSensitive: aws.Bool(true), // see PathMatchType.PathPrefix in gw spec
		}
	}

	if modelRule.Spec.Method != "" {
		httpMatch.Method = &modelRule.Spec.Method
	}

	for i := 0; i < len(modelRule.Spec.MatchedHeaders); i++ {
		headerMatch := vpclattice.HeaderMatch{
			Match:         modelRule.Spec.MatchedHeaders[i].Match,
			Name:          modelRule.Spec.MatchedHeaders[i].Name,
			CaseSensitive: aws.Bool(false), // see HTTPHeaderMatch.HTTPHeaderName in gw spec
		}
		httpMatch.HeaderMatches = append(httpMatch.HeaderMatches, &headerMatch)
	}
}

func isMatchEqual(localRule, latticeRule *vpclattice.GetRuleOutput) bool {
	// currently lattice API converts nil HeaderMatches to empty list on create
	// if we're currently nil, test both just in case it gets fixed later
	if localRule.Match != nil && localRule.Match.HttpMatch != nil &&
		localRule.Match.HttpMatch.HeaderMatches == nil {
		firstTry := reflect.DeepEqual(localRule.Match, latticeRule.Match)
		if firstTry {
			return true
		}
		// test with empty, then reset to original value
		localRule.Match.HttpMatch.HeaderMatches = make([]*vpclattice.HeaderMatch, 0)
		secondTry := reflect.DeepEqual(localRule.Match, latticeRule.Match)
		localRule.Match.HttpMatch.HeaderMatches = nil

		return secondTry
	}

	// otherwise we can rely on normal equality
	return reflect.DeepEqual(localRule.Match, latticeRule.Match)
}

func (r *defaultRuleManager) nextAvailablePriority(latticeRules []*vpclattice.GetRuleOutput) (int64, error) {
	var priorities [model.MaxRulePriority]bool
	for i := 0; i < model.MaxRulePriority; i++ {
		priorities[i] = false
	}

	for _, lr := range latticeRules {
		if lr.IsDefault != nil && aws.BoolValue(lr.IsDefault) {
			continue
		}
		// priority range is 1 -> 100
		priorities[aws.Int64Value(lr.Priority)-1] = true
	}

	for i := 0; i < model.MaxRulePriority; i++ {
		if !priorities[i] {
			return int64(i + 1), nil
		}
	}

	return 0, errors.New("no available priorities")
}

func (r *defaultRuleManager) Delete(ctx context.Context, ruleId string, serviceId string, listenerId string) error {
	r.log.Debugf(ctx, "Deleting rule %s for listener %s and service %s", ruleId, listenerId, serviceId)

	deleteInput := vpclattice.DeleteRuleInput{
		ServiceIdentifier:  aws.String(serviceId),
		ListenerIdentifier: aws.String(listenerId),
		RuleIdentifier:     aws.String(ruleId),
	}

	_, err := r.cloud.Lattice().DeleteRuleWithContext(ctx, &deleteInput)
	if err != nil {
		return fmt.Errorf("failed DeleteRule %s/%s/%s due to %s", serviceId, listenerId, ruleId, err)
	}

	r.log.Infof(ctx, "Success DeleteRule %s/%s/%s", serviceId, listenerId, ruleId)
	return nil
}
