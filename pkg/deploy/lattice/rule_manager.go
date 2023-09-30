package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"strings"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type RuleManager interface {
	Cloud() pkg_aws.Cloud
	Create(ctx context.Context, rule *model.Rule) (model.RuleStatus, error)
	Delete(ctx context.Context, ruleId string, listenerId string, serviceId string) error
	Update(ctx context.Context, rules []*model.Rule) error
	List(ctx context.Context, serviceId string, listenerId string) ([]*model.RuleStatus, error)
	Get(ctx context.Context, serviceId string, listenerId string, ruleId string) (*vpclattice.GetRuleOutput, error)
}

type defaultRuleManager struct {
	log              gwlog.Logger
	cloud            pkg_aws.Cloud
	latticeDataStore *latticestore.LatticeDataStore
}

func NewRuleManager(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
	store *latticestore.LatticeDataStore,
) *defaultRuleManager {
	return &defaultRuleManager{
		log:              log,
		cloud:            cloud,
		latticeDataStore: store,
	}
}

func (r *defaultRuleManager) Cloud() pkg_aws.Cloud {
	return r.cloud
}

type RuleLSNProvider struct {
	rule *model.Rule
}

func (r *RuleLSNProvider) LatticeServiceName() string {
	return utils.LatticeServiceName(r.rule.Spec.ServiceName, r.rule.Spec.ServiceNamespace)
}

func (r *defaultRuleManager) Get(ctx context.Context, serviceId string, listenerId string, ruleId string) (*vpclattice.GetRuleOutput, error) {
	getRuleInput := vpclattice.GetRuleInput{
		ListenerIdentifier: aws.String(listenerId),
		ServiceIdentifier:  aws.String(serviceId),
		RuleIdentifier:     aws.String(ruleId),
	}

	resp, err := r.cloud.Lattice().GetRule(&getRuleInput)
	return resp, err
}

// find out all rules in SDK lattice under a single service
func (r *defaultRuleManager) List(ctx context.Context, service string, listener string) ([]*model.RuleStatus, error) {
	var sdkRules []*model.RuleStatus = nil

	ruleListInput := vpclattice.ListRulesInput{
		ListenerIdentifier: aws.String(listener),
		ServiceIdentifier:  aws.String(service),
	}

	var resp *vpclattice.ListRulesOutput
	resp, err := r.cloud.Lattice().ListRules(&ruleListInput)
	if err != nil {
		return sdkRules, err
	}

	for _, ruleSum := range resp.Items {
		if !aws.BoolValue(ruleSum.IsDefault) {
			sdkRules = append(sdkRules, &model.RuleStatus{
				RuleID:     aws.StringValue(ruleSum.Id),
				ServiceID:  service,
				ListenerID: listener,
			})
		}
	}

	return sdkRules, nil
}

// today, it only batch update the priority
func (r *defaultRuleManager) Update(ctx context.Context, rules []*model.Rule) error {
	firstRuleSpec := rules[0].Spec
	var ruleUpdateList []*vpclattice.RuleUpdate

	latticeService, err := r.cloud.Lattice().FindService(ctx, &RuleLSNProvider{rules[0]})
	if err != nil {
		return fmt.Errorf("service %s-%s not found during rule creation",
			firstRuleSpec.ServiceName, firstRuleSpec.ServiceNamespace)
	}

	listener, err := r.latticeDataStore.GetlListener(firstRuleSpec.ServiceName, firstRuleSpec.ServiceNamespace,
		firstRuleSpec.ListenerPort, firstRuleSpec.ListenerProtocol)

	if err != nil {
		return fmt.Errorf("listener not found during rule creation for service %s-%s, port %d, protocol %s",
			firstRuleSpec.ServiceName, firstRuleSpec.ServiceNamespace, firstRuleSpec.ListenerPort, firstRuleSpec.ListenerProtocol)
	}

	for _, rule := range rules {
		priority, _ := ruleID2Priority(rule.Spec.RuleID)
		ruleUpdate := vpclattice.RuleUpdate{
			RuleIdentifier: aws.String(rule.Status.RuleID),
			Priority:       aws.Int64(priority),
		}

		ruleUpdateList = append(ruleUpdateList, &ruleUpdate)
	}

	// BatchUpdate rules using right priority
	batchRuleInput := vpclattice.BatchUpdateRuleInput{
		ListenerIdentifier: aws.String(listener.ID),
		ServiceIdentifier:  aws.String(*latticeService.Id),
		Rules:              ruleUpdateList,
	}

	_, err = r.cloud.Lattice().BatchUpdateRule(&batchRuleInput)
	return err
}

func (r *defaultRuleManager) Create(ctx context.Context, rule *model.Rule) (model.RuleStatus, error) {
	r.log.Debugf("Creating rule %s for service %s-%s and listener port %d and protocol %s",
		rule.Spec.RuleID, rule.Spec.ServiceName, rule.Spec.ServiceNamespace,
		rule.Spec.ListenerPort, rule.Spec.ListenerProtocol)

	latticeService, err := r.cloud.Lattice().FindService(ctx, &RuleLSNProvider{rule})
	if err != nil {
		return model.RuleStatus{}, err
	}

	listener, err := r.latticeDataStore.GetlListener(rule.Spec.ServiceName, rule.Spec.ServiceNamespace,
		rule.Spec.ListenerPort, rule.Spec.ListenerProtocol)
	if err != nil {
		return model.RuleStatus{}, err
	}

	priority, err := ruleID2Priority(rule.Spec.RuleID)
	if err != nil {
		return model.RuleStatus{}, fmt.Errorf("failed to create rule due to invalid ruleId, err: %s", err)
	}
	r.log.Debugf("Converted rule id %s to priority %d", rule.Spec.RuleID, priority)

	ruleStatus, err := r.findMatchingRule(ctx, rule, *latticeService.Id, listener.ID)
	if err == nil && !ruleStatus.UpdateTGsNeeded {
		if ruleStatus.Priority != priority {
			r.log.Debugf("Need to BatchUpdate priority for rule %s", rule.Spec.RuleID)
			ruleStatus.UpdatePriorityNeeded = true
		}
		return ruleStatus, nil
	}

	// if not found, ruleStatus contains the next available priority

	var latticeTGs []*vpclattice.WeightedTargetGroup

	for _, tgRule := range rule.Spec.Action.TargetGroups {
		tgName := latticestore.TargetGroupName(tgRule.Name, tgRule.Namespace)

		tg, err := r.latticeDataStore.GetTargetGroup(tgName, tgRule.RouteName, tgRule.IsServiceImport)
		if err != nil {
			return model.RuleStatus{}, err
		}

		latticeTG := vpclattice.WeightedTargetGroup{
			TargetGroupIdentifier: aws.String(tg.ID),
			Weight:                aws.Int64(tgRule.Weight),
		}

		latticeTGs = append(latticeTGs, &latticeTG)
	}

	ruleName := fmt.Sprintf("k8s-%d-%s", rule.Spec.CreateTime.Unix(), rule.Spec.RuleID)

	if ruleStatus.UpdateTGsNeeded {
		httpMatch := vpclattice.HttpMatch{}

		updateSDKhttpMatch(&httpMatch, rule)

		updateRuleInput := vpclattice.UpdateRuleInput{
			Action: &vpclattice.RuleAction{
				Forward: &vpclattice.ForwardAction{
					TargetGroups: latticeTGs,
				},
			},
			ListenerIdentifier: aws.String(listener.ID),
			Match: &vpclattice.RuleMatch{
				HttpMatch: &httpMatch,
			},
			Priority:          aws.Int64(ruleStatus.Priority),
			ServiceIdentifier: aws.String(*latticeService.Id),
			RuleIdentifier:    aws.String(ruleStatus.RuleID),
		}

		resp, err := r.cloud.Lattice().UpdateRule(&updateRuleInput)
		if err != nil {
			r.log.Errorf("Error updating rule, %s", err)
		}

		return model.RuleStatus{
			RuleID:               aws.StringValue(resp.Id),
			UpdatePriorityNeeded: ruleStatus.UpdatePriorityNeeded,
			ServiceID:            aws.StringValue(latticeService.Id),
			ListenerID:           listener.ID,
		}, nil
	}

	httpMatch := vpclattice.HttpMatch{}

	updateSDKhttpMatch(&httpMatch, rule)

	ruleInput := vpclattice.CreateRuleInput{
		Action: &vpclattice.RuleAction{
			Forward: &vpclattice.ForwardAction{
				TargetGroups: latticeTGs,
			},
		},
		ClientToken:        nil,
		ListenerIdentifier: aws.String(listener.ID),
		Match: &vpclattice.RuleMatch{
			HttpMatch: &httpMatch,
		},
		Name:              aws.String(ruleName),
		Priority:          aws.Int64(ruleStatus.Priority),
		ServiceIdentifier: aws.String(*latticeService.Id),
	}

	resp, err := r.cloud.Lattice().CreateRule(&ruleInput)
	if err != nil {
		return model.RuleStatus{}, err
	}

	return model.RuleStatus{
		RuleID:               *resp.Id,
		ListenerID:           listener.ID,
		ServiceID:            aws.StringValue(latticeService.Id),
		UpdatePriorityNeeded: ruleStatus.UpdatePriorityNeeded,
		UpdateTGsNeeded:      ruleStatus.UpdatePriorityNeeded,
	}, nil
}

func updateSDKhttpMatch(httpMatch *vpclattice.HttpMatch, rule *model.Rule) {
	// setup path based
	if rule.Spec.PathMatchExact || rule.Spec.PathMatchPrefix {
		matchType := vpclattice.PathMatchType{}
		if rule.Spec.PathMatchExact {
			matchType.Exact = aws.String(rule.Spec.PathMatchValue)
		}
		if rule.Spec.PathMatchPrefix {
			matchType.Prefix = aws.String(rule.Spec.PathMatchValue)
		}

		httpMatch.PathMatch = &vpclattice.PathMatch{
			Match: &matchType,
		}
	}

	httpMatch.Method = &rule.Spec.Method

	if rule.Spec.NumOfHeaderMatches > 0 {
		for i := 0; i < rule.Spec.NumOfHeaderMatches; i++ {
			headerMatch := vpclattice.HeaderMatch{
				Match: rule.Spec.MatchedHeaders[i].Match,
				Name:  rule.Spec.MatchedHeaders[i].Name,
			}
			httpMatch.HeaderMatches = append(httpMatch.HeaderMatches, &headerMatch)
		}
	}
}

func isRulesSame(log gwlog.Logger, modelRule *model.Rule, sdkRuleDetail *vpclattice.GetRuleOutput) bool {
	// Exact Path Match
	if modelRule.Spec.PathMatchExact {
		if sdkRuleDetail.Match.HttpMatch.PathMatch == nil ||
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match == nil ||
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Exact == nil {
			log.Debugf("no sdk PathMatchExact match")
			return false
		}

		if aws.StringValue(sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Exact) != modelRule.Spec.PathMatchValue {
			log.Debugf("Match.Exact mismatch")
			return false
		}

	} else {
		if sdkRuleDetail.Match.HttpMatch.PathMatch != nil &&
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match != nil &&
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Exact != nil {
			log.Debugf("no sdk PathMatchExact match")
			return false
		}
	}

	// Path Prefix
	if modelRule.Spec.PathMatchPrefix {
		if sdkRuleDetail.Match.HttpMatch.PathMatch == nil ||
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match == nil ||
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Prefix == nil {
			log.Debugf("no sdk HTTP PathPrefix")
			return false
		}

		if aws.StringValue(sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Prefix) != modelRule.Spec.PathMatchValue {
			log.Debugf("PathMatchPrefix mismatch ")
			return false
		}
	} else {
		if sdkRuleDetail.Match.HttpMatch.PathMatch != nil &&
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match != nil &&
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Prefix != nil {
			log.Debugf("no sdk HTTP PathPrefix")
			return false
		}
	}

	// Method match
	if aws.StringValue(sdkRuleDetail.Match.HttpMatch.Method) != modelRule.Spec.Method {
		log.Debugf("Method mismatch '%s' != '%s'", modelRule.Spec.Method, *sdkRuleDetail.Match.HttpMatch.Method)
		return false
	}

	// Header Match
	if modelRule.Spec.NumOfHeaderMatches > 0 {
		if len(sdkRuleDetail.Match.HttpMatch.HeaderMatches) != modelRule.Spec.NumOfHeaderMatches {
			log.Debugf("header match number mismatch")
			return false
		}

		misMatch := false

		// compare 2 array
		for _, sdkHeader := range sdkRuleDetail.Match.HttpMatch.HeaderMatches {
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
				log.Debugf("header %s not found", *sdkHeader)
				break
			}
		}

		if misMatch {
			log.Debugf("mismatch header")
			return false
		}
	}

	return true
}

// Determine if rule spec is same
// If rule spec is same, then determine if there is any changes on target groups
func (r *defaultRuleManager) findMatchingRule(
	ctx context.Context,
	rule *model.Rule,
	serviceId string,
	listenerId string,
) (model.RuleStatus, error) {
	var priorityMap [100]bool

	for i := 1; i < 100; i++ {
		priorityMap[i] = false

	}

	ruleListInput := vpclattice.ListRulesInput{
		ListenerIdentifier: aws.String(listenerId),
		ServiceIdentifier:  aws.String(serviceId),
	}

	var resp *vpclattice.ListRulesOutput
	resp, err := r.cloud.Lattice().ListRules(&ruleListInput)
	if err != nil {
		return model.RuleStatus{}, err
	}

	var matchRule *vpclattice.GetRuleOutput = nil
	var updateTGsNeeded = false
	for _, ruleSum := range resp.Items {
		if aws.BoolValue(ruleSum.IsDefault) {
			// Ignore the default
			continue
		}

		// retrieve action
		ruleInput := vpclattice.GetRuleInput{
			ListenerIdentifier: &listenerId,
			ServiceIdentifier:  &serviceId,
			RuleIdentifier:     ruleSum.Id,
		}

		var ruleResp *vpclattice.GetRuleOutput

		ruleResp, err := r.cloud.Lattice().GetRule(&ruleInput)
		if err != nil {
			r.log.Debugf("Matching rule not found, err %s", err)
			continue
		}

		priorityMap[aws.Int64Value(ruleResp.Priority)] = true

		ruleIsSame := isRulesSame(r.log, rule, ruleResp)
		if !ruleIsSame {
			continue
		}

		matchRule = ruleResp

		if len(ruleResp.Action.Forward.TargetGroups) != len(rule.Spec.Action.TargetGroups) {
			r.log.Debugf("Skipping rule due to mismatched number of target groups to forward to")
			updateTGsNeeded = true
			continue
		}

		if len(ruleResp.Action.Forward.TargetGroups) == 0 {
			r.log.Debugf("Skipping rule due to 0 targetGroups to forward to")
			continue
		}

		for _, tg := range ruleResp.Action.Forward.TargetGroups {
			for _, k8sTG := range rule.Spec.Action.TargetGroups {
				// get k8sTG id
				tgName := latticestore.TargetGroupName(k8sTG.Name, k8sTG.Namespace)
				k8sTGinStore, err := r.latticeDataStore.GetTargetGroup(tgName, rule.Spec.ServiceName, k8sTG.IsServiceImport)

				if err != nil {
					r.log.Debugf("Failed to find k8s tg %s-%s in datastore", k8sTG.Name, k8sTG.Namespace)
					updateTGsNeeded = true
					continue
				}

				if aws.StringValue(tg.TargetGroupIdentifier) != k8sTGinStore.ID {
					r.log.Debugf("target group id mismatch in datastore, %s vs. %s",
						aws.StringValue(tg.TargetGroupIdentifier), k8sTGinStore.ID)
					updateTGsNeeded = true
					continue

				}

				if k8sTG.Weight != aws.Int64Value(tg.Weight) {
					r.log.Debugf("Weight has changed for tg %s, old %d vs. new %d",
						aws.StringValue(tg.TargetGroupIdentifier), aws.Int64Value(tg.Weight), k8sTG.Weight)
					updateTGsNeeded = true
					continue
				}

				break
			}

			if updateTGsNeeded {
				break
			}
		}
	}

	if matchRule != nil {
		inputRulePriority, _ := ruleID2Priority(rule.Spec.RuleID)

		UpdatePriority := false
		if inputRulePriority != aws.Int64Value(matchRule.Priority) {
			UpdatePriority = true
		}

		return model.RuleStatus{
			RuleARN:              aws.StringValue(matchRule.Arn),
			RuleID:               aws.StringValue(matchRule.Id),
			Priority:             aws.Int64Value(matchRule.Priority),
			UpdateTGsNeeded:      updateTGsNeeded,
			UpdatePriorityNeeded: UpdatePriority,
		}, nil
	} else {
		var nextPriority int64 = 0
		// find available priority
		for i := 1; i < 100; i++ {
			if !priorityMap[i] {
				nextPriority = int64(i)
				break
			}

		}
		return model.RuleStatus{Priority: nextPriority}, errors.New("rule not found")
	}
}

func ruleID2Priority(ruleId string) (int64, error) {
	var priority int
	ruleIDName := strings.NewReader(ruleId)
	_, err := fmt.Fscanf(ruleIDName, "rule-%d", &priority)
	return int64(priority), err
}

func (r *defaultRuleManager) Delete(ctx context.Context, ruleId string, listenerId string, serviceId string) error {
	r.log.Debugf("Deleting rule %s for listener %s and service %s", ruleId, listenerId, serviceId)

	deleteInput := vpclattice.DeleteRuleInput{
		RuleIdentifier:     aws.String(ruleId),
		ListenerIdentifier: aws.String(listenerId),
		ServiceIdentifier:  aws.String(serviceId),
	}

	_, err := r.cloud.Lattice().DeleteRule(&deleteInput)
	return err
}
