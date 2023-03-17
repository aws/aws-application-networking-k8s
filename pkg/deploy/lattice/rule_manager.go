package lattice

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/glog"
	"strings"

	"github.com/aws/aws-sdk-go/aws"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

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
	cloud            lattice_aws.Cloud
	latticeDataStore *latticestore.LatticeDataStore
}

func NewRuleManager(cloud lattice_aws.Cloud, store *latticestore.LatticeDataStore) *defaultRuleManager {
	return &defaultRuleManager{
		cloud:            cloud,
		latticeDataStore: store,
	}
}

func (r *defaultRuleManager) Get(ctx context.Context, serviceID string, listernID string, ruleID string) (*vpclattice.GetRuleOutput, error) {
	getruleInput := vpclattice.GetRuleInput{
		ListenerIdentifier: aws.String(listernID),
		ServiceIdentifier:  aws.String(serviceID),
		RuleIdentifier:     aws.String(ruleID),
	}

	resp, err := r.cloud.Lattice().GetRule(&getruleInput)

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

	glog.V(6).Infoln("############list rules req############")
	glog.V(6).Infof("rule: %v , serviceID: %v, listenerID %v \n", resp, service, listener)

	glog.V(6).Infoln("############list rules resp############")
	glog.V(6).Infof("resp: %v, err: %v\n", resp, err)

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

	glog.V(6).Infof("Rule --- update >>>>>>>>.%v\n", rules)

	latticeService, err := r.latticeDataStore.GetLatticeService(rules[0].Spec.ServiceName, rules[0].Spec.ServiceNamespace)

	if err != nil {
		errmsg := fmt.Sprintf("Service %v not found during rule creation", rules[0].Spec)
		glog.V(6).Infof("Error during update rule %s \n", errmsg)
		return errors.New(errmsg)
	}

	listener, err := r.latticeDataStore.GetlListener(rules[0].Spec.ServiceName, rules[0].Spec.ServiceNamespace,
		rules[0].Spec.ListenerPort, rules[0].Spec.ListenerProtocol)

	if err != nil {
		errmsg := fmt.Sprintf("Listener %v not found during rule creation", rules[0].Spec)
		glog.V(6).Infof("Error during update rule %s \n", errmsg)
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

	glog.V(2).Infoln("############req updating rule ###########")
	glog.V(2).Infoln(batchRuleInput)
	glog.V(2).Infof("############resp updateing rule ###########, err: %v \n", err)
	glog.V(2).Infoln(resp)

	return err
}

func (r *defaultRuleManager) Create(ctx context.Context, rule *latticemodel.Rule) (latticemodel.RuleStatus, error) {
	glog.V(6).Infof("Rule --- Create >>>>>>>>.%v\n", *rule)

	latticeService, err := r.latticeDataStore.GetLatticeService(rule.Spec.ServiceName, rule.Spec.ServiceNamespace)

	if err != nil {
		errmsg := fmt.Sprintf("Service %v not found during rule creation", rule.Spec)
		glog.V(6).Infof("Error during create rule %s \n", errmsg)
		return latticemodel.RuleStatus{}, errors.New(errmsg)
	}

	listener, err := r.latticeDataStore.GetlListener(rule.Spec.ServiceName, rule.Spec.ServiceNamespace,
		rule.Spec.ListenerPort, rule.Spec.ListenerProtocol)

	if err != nil {
		errmsg := fmt.Sprintf("Listener %v not found during rule creation", rule.Spec)
		glog.V(6).Infof("Error during create rule %s \n", errmsg)
		return latticemodel.RuleStatus{}, errors.New(errmsg)
	}

	priority, err := ruleID2Priority(rule.Spec.RuleID)
	glog.V(6).Infof("Convert rule id %s to priority %d error: %v \n", rule.Spec.RuleID, priority, err)

	if err != nil {
		glog.V(6).Infof("Error create rule, failed to convert RuleID %v to priority err :%v\n", rule.Spec.RuleID, err)
		return latticemodel.RuleStatus{}, errors.New("failed to create rule, due to invalid ruleID")
	}

	ruleStatus, err := r.findMatchingRule(ctx, rule, latticeService.ID, listener.ID)

	if err == nil && !ruleStatus.UpdateTGsNeeded {

		if ruleStatus.Priority != priority {
			glog.V(6).Infof("Rule-Create: need to BatchUpdate priority")
			ruleStatus.UpdatePriorityNeeded = true
		}
		glog.V(6).Infof("Rule--Create, found existing matching rule %v rulsStatus %v\n", rule, ruleStatus)
		return ruleStatus, nil
	}

	// if not found, ruleStatus contains the next available priority

	latticeTGs := []*vpclattice.WeightedTargetGroup{}

	for _, tgRule := range rule.Spec.Action.TargetGroups {

		tgName := latticestore.TargetGroupName(tgRule.Name, tgRule.Namespace)
		tg, err := r.latticeDataStore.GetTargetGroup(tgName, tgRule.IsServiceImport)

		if err != nil {
			glog.V(6).Infof("Faild to create rule due to unknown tg %v, err %v\n", tgName, err)
			return latticemodel.RuleStatus{}, err
		}

		latticeTG := vpclattice.WeightedTargetGroup{
			TargetGroupIdentifier: aws.String(tg.ID),
			// TODO weighted target
			Weight: aws.Int64(tgRule.Weight),
		}

		latticeTGs = append(latticeTGs, &latticeTG)

	}

	ruleName := fmt.Sprintf("k8s-%d-%s", rule.Spec.CreateTime.Unix(), rule.Spec.RuleID)

	if ruleStatus.UpdateTGsNeeded {
		updateRuleInput := vpclattice.UpdateRuleInput{
			Action: &vpclattice.RuleAction{
				Forward: &vpclattice.ForwardAction{
					TargetGroups: latticeTGs,
				},
			},
			ListenerIdentifier: aws.String(listener.ID),
			Match: &vpclattice.RuleMatch{
				HttpMatch: &vpclattice.HttpMatch{
					PathMatch: &vpclattice.PathMatch{
						CaseSensitive: nil,
						Match: &vpclattice.PathMatchType{
							Exact:  nil,
							Prefix: aws.String(rule.Spec.RuleValue),
						},
					},
				},
			},
			Priority:          aws.Int64(ruleStatus.Priority),
			ServiceIdentifier: aws.String(latticeService.ID),
			RuleIdentifier:    aws.String(ruleStatus.RuleID),
		}

		resp, err := r.cloud.Lattice().UpdateRule(&updateRuleInput)

		glog.V(2).Infoln("############req updating rule TGs###########")
		glog.V(2).Infoln(updateRuleInput)
		glog.V(2).Infof("############resp updating  rule TGs ###########, err: %v \n", err)
		glog.V(2).Infoln(resp)
		return latticemodel.RuleStatus{
			RuleID:               aws.StringValue(resp.Id),
			UpdatePriorityNeeded: ruleStatus.UpdatePriorityNeeded,
			ServiceID:            latticeService.ID,
			ListenerID:           listener.ID,
		}, nil

	} else {

		httpMatch := vpclattice.HttpMatch{}

		glog.V(2).Infof("liwwu>> rule.Spec %v", rule.Spec)

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

		// setup header based
		if rule.Spec.NumOfHeaderMatches > 0 {

			for i := 0; i < rule.Spec.NumOfHeaderMatches; i++ {
				headerMatch := vpclattice.HeaderMatch{
					Match: rule.Spec.MatchedHeaders[i].Match,
					Name: rule.Spec.MatchedHeaders[i].Name,
				}
				httpMatch.HeaderMatches = append(httpMatch.HeaderMatches, &headerMatch)

			}

		}

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

		glog.V(2).Infoln("############req creating rule ###########")
		glog.V(2).Infoln(ruleInput)
		glog.V(2).Infof("############resp creating rule ###########, err: %v \n", err)
		glog.V(2).Infoln(resp)
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

	glog.V(6).Infoln("############list rules req############")
	glog.V(6).Infof("rule: %v , serviceID: %v, listenerID %v \n", rule, serviceID, listenerID)

	glog.V(6).Infoln("############list rules resp############")
	glog.V(6).Infof("resp: %v, err: %v\n", resp, err)

	if err != nil {
		return latticemodel.RuleStatus{}, err
	}

	var matchRule *vpclattice.GetRuleOutput = nil
	var updateTGsNeeded bool = false
	for _, ruleSum := range resp.Items {

		if aws.BoolValue(ruleSum.IsDefault) {
			// Ignore the default
			glog.V(6).Infof("findMatchingRule: ingnore the default rule %v\n", ruleSum)
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
			glog.V(6).Infof("findMatchingRule, rule %v not found err:%v\n", ruleInput, err)
			continue
		}

		priorityMap[aws.Int64Value(ruleResp.Priority)] = true

		// path based comparasion

		if ruleResp.Match != nil && ruleResp.Match.HttpMatch.PathMatch != nil && aws.StringValue(ruleResp.Match.HttpMatch.PathMatch.Match.Prefix) != rule.Spec.RuleValue {
			glog.V(6).Infof("findMatchingRule, rule prefix %v does not match rule value %v\n",
				aws.StringValue(ruleResp.Match.HttpMatch.PathMatch.Match.Prefix), rule.Spec.RuleValue)
			continue
		}

		// header based comparasion
		if !isHeaderMatchSame(ruleResp.Match, rule.Spec) {
			continue

		}

		matchRule = ruleResp

		if len(ruleResp.Action.Forward.TargetGroups) != len(rule.Spec.Action.TargetGroups) {
			glog.V(6).Infof("findMatchingRule, mismatch TGs lattice %v, k8s %v\n",
				ruleResp.Action.Forward.TargetGroups, rule.Spec.Action.TargetGroups)
			updateTGsNeeded = true
			continue
		}

		if len(ruleResp.Action.Forward.TargetGroups) == 0 {
			glog.V(6).Infof("findMatchingRule, 0 targetGroups \n")
			continue
		}

		for _, tg := range ruleResp.Action.Forward.TargetGroups {

			for _, k8sTG := range rule.Spec.Action.TargetGroups {
				// get k8sTG id
				tgName := latticestore.TargetGroupName(k8sTG.Name, k8sTG.Namespace)
				k8sTGinStore, err := r.latticeDataStore.GetTargetGroup(tgName, k8sTG.IsServiceImport)

				if err != nil {
					glog.V(6).Infof("findMatchingRule, failed to find tg %v in store \n", k8sTG)
					updateTGsNeeded = true
					continue
				}

				if aws.StringValue(tg.TargetGroupIdentifier) != k8sTGinStore.ID {
					glog.V(6).Infof("findMatchingRule, failed to TGID mismatch lattice %v, k8s %v\n",
						aws.StringValue(tg.TargetGroupIdentifier), k8sTGinStore.ID)
					updateTGsNeeded = true
					continue

				}

				if k8sTG.Weight != aws.Int64Value(tg.Weight) {
					glog.V(6).Infof("findMatchRule, weight has changed for tg %v old %v new %v\n",
						tg, aws.Int64Value(tg.Weight), k8sTG.Weight)
					updateTGsNeeded = true
					continue
				}

				break

			}

			if updateTGsNeeded {
				glog.V(6).Infof("findMatchingRule: can not find matching TG tg %v in matching K8S tg\n", tg)
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

func isHeaderMatchSame(awsRuleMatch *vpclattice.RuleMatch, ruleSpec latticemodel.RuleSpec) bool {
	if awsRuleMatch == nil {
		fmt.Printf("liwwu>>> no rule match \n")
		return false
	}
	if awsRuleMatch.HttpMatch.HeaderMatches == nil {
		fmt.Printf("liwwu >>> no header Match %v\n", *awsRuleMatch)
		return false

	}

	if len(awsRuleMatch.HttpMatch.HeaderMatches) != ruleSpec.NumOfHeaderMatches {
		fmt.Printf("liwwu >>> number of headers are different \n")
		return false
	}

	for _, match := range awsRuleMatch.HttpMatch.HeaderMatches {
		found := false
		for _, matchSpec := range ruleSpec.MatchedHeaders {
			if aws.StringValue(match.Match.Exact) == aws.StringValue(matchSpec.Match.Exact) &&
				aws.StringValue(match.Name) == aws.StringValue(matchSpec.Name) {
				found = true
				break
			}
		}
		if !found {
			return false
		}

	}

	return true
}

func ruleID2Priority(ruleID string) (int64, error) {

	var priority int
	ruleIDName := strings.NewReader(ruleID)
	_, err := fmt.Fscanf(ruleIDName, "rule-%d", &priority)

	return int64(priority), err
}

func (r *defaultRuleManager) Delete(ctx context.Context, ruleID string, listenerID string, serviceID string) error {
	glog.V(6).Infof("Rule --- Delete >>>>> rule %v, listener %v service %v \n", ruleID, listenerID, serviceID)

	deleteInput := vpclattice.DeleteRuleInput{
		RuleIdentifier:     aws.String(ruleID),
		ListenerIdentifier: aws.String(listenerID),
		ServiceIdentifier:  aws.String(serviceID),
	}

	resp, err := r.cloud.Lattice().DeleteRule(&deleteInput)

	glog.V(2).Infof("Delete Rule >>>> input %v, output %v, err %v\n", deleteInput, resp, err)

	return err
}
