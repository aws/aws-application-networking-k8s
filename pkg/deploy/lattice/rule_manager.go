package lattice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type RuleManager interface {
	Create(ctx context.Context, rule *latticemodel.Rule) (latticemodel.RuleStatus, error)
	Delete(ctx context.Context, ruleID string, listenerID string, serviceID string) error
	Update(ctx context.Context, rules []*latticemodel.Rule) error
	List(ctx context.Context, serviceID string, listenerID string) ([]*latticemodel.RuleStatus, error)
	Get(ctx context.Context, serviceID string, listernID string, ruleID string) (*vpclattice.GetRuleOutput, error)
}

type defaultRuleManager struct {
	log              gwlog.Logger
	cloud            lattice_aws.Cloud
	latticeDataStore *latticestore.LatticeDataStore
}

func NewRuleManager(log gwlog.Logger, cloud lattice_aws.Cloud, store *latticestore.LatticeDataStore) *defaultRuleManager {
	return &defaultRuleManager{
		log:              log,
		cloud:            cloud,
		latticeDataStore: store,
	}
}

func (r *defaultRuleManager) Get(ctx context.Context, serviceID string, listernID string, ruleID string) (*vpclattice.GetRuleOutput, error) {
	getRuleInput := vpclattice.GetRuleInput{
		ListenerIdentifier: aws.String(listernID),
		ServiceIdentifier:  aws.String(serviceID),
		RuleIdentifier:     aws.String(ruleID),
	}

	resp, err := r.cloud.Lattice().GetRule(&getRuleInput)

	return resp, err
}

// find out all rules in SDK lattice under a single service
func (r *defaultRuleManager) List(ctx context.Context, service string, listener string) ([]*latticemodel.RuleStatus, error) {
	var sdkRules []*latticemodel.RuleStatus = nil

	ruleListInput := vpclattice.ListRulesInput{
		ListenerIdentifier: aws.String(listener),
		ServiceIdentifier:  aws.String(service),
	}

	var resp *vpclattice.ListRulesOutput
	resp, err := r.cloud.Lattice().ListRules(&ruleListInput)

	r.log.Debugln("############list rules req############")
	r.log.Debugf("rule: %v , serviceID: %v, listenerID %v", resp, service, listener)

	r.log.Debugln("############list rules resp############")
	r.log.Debugf("resp: %v, err: %v", resp, err)

	if err != nil {
		return sdkRules, err
	}

	for _, ruleSum := range resp.Items {
		if aws.BoolValue(ruleSum.IsDefault) {
			continue
		}

		sdkRules = append(sdkRules,
			&latticemodel.RuleStatus{
				RuleID:     aws.StringValue(ruleSum.Id),
				ServiceID:  service,
				ListenerID: listener,
			})
	}
	return sdkRules, nil
}

// today, it only batch update the priority
func (r *defaultRuleManager) Update(ctx context.Context, rules []*latticemodel.Rule) error {

	var ruleUpdateList []*vpclattice.RuleUpdate

	r.log.Infof("Rule --- update >>>>>>>>.%v", rules)

	latticeService, err := r.latticeDataStore.GetLatticeService(rules[0].Spec.ServiceName, rules[0].Spec.ServiceNamespace)

	if err != nil {
		errmsg := fmt.Sprintf("Service %v not found during rule creation", rules[0].Spec)
		r.log.Debugf("Error during update rule %s", errmsg)
		return errors.New(errmsg)
	}

	listener, err := r.latticeDataStore.GetlListener(rules[0].Spec.ServiceName, rules[0].Spec.ServiceNamespace,
		rules[0].Spec.ListenerPort, rules[0].Spec.ListenerProtocol)

	if err != nil {
		errmsg := fmt.Sprintf("Listener %v not found during rule creation", rules[0].Spec)
		r.log.Debugf("Error during update rule %s", errmsg)
		return errors.New(errmsg)
	}

	for _, rule := range rules {
		priority, _ := ruleID2Priority(rule.Spec.RuleID)
		ruleupdate := vpclattice.RuleUpdate{
			RuleIdentifier: aws.String(rule.Status.RuleID),
			Priority:       aws.Int64(priority),
		}

		ruleUpdateList = append(ruleUpdateList, &ruleupdate)

	}
	// batchupdate rules using right priority
	batchRuleInput := vpclattice.BatchUpdateRuleInput{
		ListenerIdentifier: aws.String(listener.ID),
		ServiceIdentifier:  aws.String(latticeService.ID),
		Rules:              ruleUpdateList,
	}

	resp, err := r.cloud.Lattice().BatchUpdateRule(&batchRuleInput)

	r.log.Debugln("############req updating rule ###########")
	r.log.Debugln(batchRuleInput)
	r.log.Debugf("############resp updateing rule ###########, err: %v", err)
	r.log.Debugln(resp)

	return err
}

func (r *defaultRuleManager) Create(ctx context.Context, rule *latticemodel.Rule) (latticemodel.RuleStatus, error) {
	r.log.Infof("Rule --- Create >>>>>>>>.%v", *rule)

	latticeService, err := r.latticeDataStore.GetLatticeService(rule.Spec.ServiceName, rule.Spec.ServiceNamespace)

	if err != nil {
		errmsg := fmt.Sprintf("Service %v not found during rule creation", rule.Spec)
		r.log.Debugf("Error during create rule %s", errmsg)
		return latticemodel.RuleStatus{}, errors.New(errmsg)
	}

	listener, err := r.latticeDataStore.GetlListener(rule.Spec.ServiceName, rule.Spec.ServiceNamespace,
		rule.Spec.ListenerPort, rule.Spec.ListenerProtocol)

	if err != nil {
		errmsg := fmt.Sprintf("Listener %v not found during rule creation", rule.Spec)
		r.log.Debugf("Error during create rule %s", errmsg)
		return latticemodel.RuleStatus{}, errors.New(errmsg)
	}

	priority, err := ruleID2Priority(rule.Spec.RuleID)
	r.log.Infof("Convert rule id %s to priority %d error: %v", rule.Spec.RuleID, priority, err)

	if err != nil {
		r.log.Debugf("Error create rule, failed to convert RuleID %v to priority err :%v", rule.Spec.RuleID, err)
		return latticemodel.RuleStatus{}, errors.New("failed to create rule, due to invalid ruleID")
	}

	ruleStatus, err := r.findMatchingRule(ctx, rule, latticeService.ID, listener.ID)

	if err == nil && !ruleStatus.UpdateTGsNeeded {

		if ruleStatus.Priority != priority {
			r.log.Infof("Rule-Create: need to BatchUpdate priority")
			ruleStatus.UpdatePriorityNeeded = true
		}
		r.log.Infof("Rule--Create, found existing matching rule %v rulsStatus %v", rule, ruleStatus)
		return ruleStatus, nil
	}

	// if not found, ruleStatus contains the next available priority

	latticeTGs := []*vpclattice.WeightedTargetGroup{}

	for _, tgRule := range rule.Spec.Action.TargetGroups {

		tgName := latticestore.TargetGroupName(tgRule.Name, tgRule.Namespace)
		tg, err := r.latticeDataStore.GetTargetGroup(tgName, tgRule.RouteName, tgRule.IsServiceImport)

		if err != nil {
			r.log.Debugf("Faild to create rule due to unknown tg %v, err %v", tgName, err)
			return latticemodel.RuleStatus{}, err
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
			ServiceIdentifier: aws.String(latticeService.ID),
			RuleIdentifier:    aws.String(ruleStatus.RuleID),
		}

		resp, err := r.cloud.Lattice().UpdateRule(&updateRuleInput)

		r.log.Debugln("############req updating rule TGs###########")
		r.log.Debugln(updateRuleInput)
		r.log.Debugf("############resp updating  rule TGs ###########, err: %v", err)
		r.log.Debugln(resp)
		return latticemodel.RuleStatus{
			RuleID:               aws.StringValue(resp.Id),
			UpdatePriorityNeeded: ruleStatus.UpdatePriorityNeeded,
			ServiceID:            latticeService.ID,
			ListenerID:           listener.ID,
		}, nil

	} else {

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
			ServiceIdentifier: aws.String(latticeService.ID),
		}

		resp, err := r.cloud.Lattice().CreateRule(&ruleInput)

		r.log.Debugln("############req creating rule ###########")
		r.log.Debugln(ruleInput)
		r.log.Debugf("############resp creating rule ###########, err: %v", err)
		r.log.Debugln(resp)
		if err != nil {
			return latticemodel.RuleStatus{}, err
		} else {
			return latticemodel.RuleStatus{
				RuleID:               *resp.Id,
				ListenerID:           listener.ID,
				ServiceID:            latticeService.ID,
				UpdatePriorityNeeded: ruleStatus.UpdatePriorityNeeded,
				UpdateTGsNeeded:      ruleStatus.UpdatePriorityNeeded,
			}, nil
		}
	}

}

func updateSDKhttpMatch(httpMatch *vpclattice.HttpMatch, rule *latticemodel.Rule) {
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

func isRulesSame(log gwlog.Logger, modelRule *latticemodel.Rule, sdkRuleDetail *vpclattice.GetRuleOutput) bool {
	// Exact Path Match
	if modelRule.Spec.PathMatchExact {
		log.Infoln("Checking PathMatchExact")

		if sdkRuleDetail.Match.HttpMatch.PathMatch == nil ||
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match == nil ||
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Exact == nil {
			log.Infoln("no sdk PathMatchExact match")
			return false
		}

		if aws.StringValue(sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Exact) != modelRule.Spec.PathMatchValue {
			log.Infoln("Match.Exact mismatch")
			return false
		}

	} else {
		if sdkRuleDetail.Match.HttpMatch.PathMatch != nil &&
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match != nil &&
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Exact != nil {
			log.Infoln("no sdk PathMatchExact match")
			return false
		}
	}

	// Path Prefix
	if modelRule.Spec.PathMatchPrefix {
		log.Infoln("Checking PathMatchPrefix")

		if sdkRuleDetail.Match.HttpMatch.PathMatch == nil ||
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match == nil ||
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Prefix == nil {
			log.Infoln("no sdk HTTP PathPrefix")
			return false
		}

		if aws.StringValue(sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Prefix) != modelRule.Spec.PathMatchValue {
			log.Infoln("PathMatchPrefix mismatch ")
			return false
		}
	} else {
		if sdkRuleDetail.Match.HttpMatch.PathMatch != nil &&
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match != nil &&
			sdkRuleDetail.Match.HttpMatch.PathMatch.Match.Prefix != nil {
			log.Infoln("no sdk HTTP PathPrefix")
			return false
		}
	}

	// Method match
	if aws.StringValue(sdkRuleDetail.Match.HttpMatch.Method) != modelRule.Spec.Method {
		log.Infof("Method mismatch '%v' != '%v'", modelRule.Spec.Method, sdkRuleDetail.Match.HttpMatch.Method)
		return false
	}

	// Header Match
	if modelRule.Spec.NumOfHeaderMatches > 0 {
		log.Infof("Checking Header Match, numofheader matches %v", modelRule.Spec.NumOfHeaderMatches)
		if len(sdkRuleDetail.Match.HttpMatch.HeaderMatches) != modelRule.Spec.NumOfHeaderMatches {
			log.Infoln("header match number mismatch")
			return false
		}

		misMatch := false

		// compare 2 array
		for _, sdkHeader := range sdkRuleDetail.Match.HttpMatch.HeaderMatches {
			log.Infof("sdkHeader >> %v", sdkHeader)
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
				log.Infof("header not found sdkHeader %v", *sdkHeader)
				break
			}
		}

		if misMatch {
			log.Infof("mismatch header")
			return false
		}
	}

	return true
}

// Determine if rule spec is same
// If rule spec is same, then determine if there is any changes on target groups
func (r *defaultRuleManager) findMatchingRule(ctx context.Context, rule *latticemodel.Rule,
	serviceID string, listenerID string) (latticemodel.RuleStatus, error) {

	var priorityMap [100]bool

	for i := 1; i < 100; i++ {
		priorityMap[i] = false

	}

	ruleListInput := vpclattice.ListRulesInput{
		ListenerIdentifier: aws.String(listenerID),
		ServiceIdentifier:  aws.String(serviceID),
	}

	var resp *vpclattice.ListRulesOutput
	resp, err := r.cloud.Lattice().ListRules(&ruleListInput)

	r.log.Infoln("############list rules req############")
	r.log.Infof("rule: %v , serviceID: %v, listenerID %v", rule, serviceID, listenerID)

	r.log.Infoln("############list rules resp############")
	r.log.Infof("resp: %v, err: %v", resp, err)

	if err != nil {
		return latticemodel.RuleStatus{}, err
	}

	var matchRule *vpclattice.GetRuleOutput = nil
	var updateTGsNeeded bool = false
	for _, ruleSum := range resp.Items {

		if aws.BoolValue(ruleSum.IsDefault) {
			// Ignore the default
			r.log.Infof("findMatchingRule: ingnore the default rule %v", ruleSum)
			continue
		}

		// retrieve action
		ruleInput := vpclattice.GetRuleInput{
			ListenerIdentifier: &listenerID,
			ServiceIdentifier:  &serviceID,
			RuleIdentifier:     ruleSum.Id,
		}

		var ruleResp *vpclattice.GetRuleOutput

		ruleResp, err := r.cloud.Lattice().GetRule(&ruleInput)

		if err != nil {
			r.log.Debugf("findMatchingRule, rule %v not found err:%v", ruleInput, err)
			continue
		}

		priorityMap[aws.Int64Value(ruleResp.Priority)] = true

		samerule := isRulesSame(r.log, rule, ruleResp)

		if !samerule {
			continue
		}

		matchRule = ruleResp

		if len(ruleResp.Action.Forward.TargetGroups) != len(rule.Spec.Action.TargetGroups) {
			r.log.Infof("Mismatched TGs lattice %v, k8s %v",
				ruleResp.Action.Forward.TargetGroups, rule.Spec.Action.TargetGroups)
			updateTGsNeeded = true
			continue
		}

		if len(ruleResp.Action.Forward.TargetGroups) == 0 {
			r.log.Infof("0 targetGroups")
			continue
		}

		for _, tg := range ruleResp.Action.Forward.TargetGroups {

			for _, k8sTG := range rule.Spec.Action.TargetGroups {
				// get k8sTG id
				tgName := latticestore.TargetGroupName(k8sTG.Name, k8sTG.Namespace)
				k8sTGinStore, err := r.latticeDataStore.GetTargetGroup(tgName, rule.Spec.ServiceName, k8sTG.IsServiceImport)

				if err != nil {
					r.log.Infof("Failed to find k8s tg %v in store", k8sTG)
					updateTGsNeeded = true
					continue
				}

				if aws.StringValue(tg.TargetGroupIdentifier) != k8sTGinStore.ID {
					r.log.Infof("TGID mismatch lattice %v, k8s %v",
						aws.StringValue(tg.TargetGroupIdentifier), k8sTGinStore.ID)
					updateTGsNeeded = true
					continue

				}

				if k8sTG.Weight != aws.Int64Value(tg.Weight) {
					r.log.Infof("Weight has changed for tg %v old %v new %v",
						tg, aws.Int64Value(tg.Weight), k8sTG.Weight)
					updateTGsNeeded = true
					continue
				}

				break

			}

			if updateTGsNeeded {
				r.log.Infof("update TGs Needed for tg %v", tg)
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

		return latticemodel.RuleStatus{
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
		return latticemodel.RuleStatus{Priority: nextPriority}, errors.New("rule not found")
	}

}

func ruleID2Priority(ruleID string) (int64, error) {

	var priority int
	ruleIDName := strings.NewReader(ruleID)
	_, err := fmt.Fscanf(ruleIDName, "rule-%d", &priority)

	return int64(priority), err
}

func (r *defaultRuleManager) Delete(ctx context.Context, ruleID string, listenerID string, serviceID string) error {
	r.log.Infof("Rule --- Delete >>>>> rule %v, listener %v service %v", ruleID, listenerID, serviceID)

	deleteInput := vpclattice.DeleteRuleInput{
		RuleIdentifier:     aws.String(ruleID),
		ListenerIdentifier: aws.String(listenerID),
		ServiceIdentifier:  aws.String(serviceID),
	}

	resp, err := r.cloud.Lattice().DeleteRule(&deleteInput)

	r.log.Debugf("Delete Rule >>>> input %v, output %v, err %v", deleteInput, resp, err)

	return err
}
