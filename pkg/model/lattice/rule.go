package lattice

import (
	"time"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

type Rule struct {
	core.ResourceMeta `json:"-"`
	Spec              RuleSpec    `json:"spec"`
	Status            *RuleStatus `json:"status,omitempty"`
}

const (
	// K8S HTTPRouteMatch
	MatchByPath = "HTTPRouteMatch"
	// K8S HTTPRouteFilter
	MatchByFilter = "HTTPRouteFilter"

	MAX_NUM_OF_MATCHED_HEADERS = 5
)

type RuleSpec struct {
	ServiceName      string `json:"name"`
	ServiceNamespace string `json:"namespace"`
	ListenerPort     int64  `json:"port"`
	ListenerProtocol string `json:"protocol"`
	// PathMatch
	PathMatchValue  string `json:"pathmatchvalue"`
	PathMatchExact  bool   `json:"pathmatchexact"`
	PathMatchPrefix bool   `json:"pathmatchprefix"`
	// Header
	NumOfHeaderMatches int `json:"numofheadermatches"`
	MatchedHeaders     [MAX_NUM_OF_MATCHED_HEADERS]vpclattice.HeaderMatch

	RuleID     string     `json:"id"`
	Action     RuleAction `json:"action"`
	CreateTime time.Time  `json:"time"`
}

type RuleAction struct {
	TargetGroups []*RuleTargetGroup `json:"ruletarget"`
}

type RuleTargetGroup struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	IsServiceImport bool   `json:"isServiceImport"`
	Weight          int64  `json:"weight"`
}

type RuleStatus struct {
	RuleARN              string `json:"ARN"`
	RuleID               string `json:"ID"`
	Priority             int64  `json:"priority"`
	ListenerID           string `json:"Listner"`
	ServiceID            string `json:"Service"`
	UpdatePriorityNeeded bool   `json:"updatepriorityneeded"`
	UpdateTGsNeeded      bool   `json:"updateTGneeded"`
}

func NewRule(stack core.Stack, id string, name string, namespace string, port int64,
	protocol string, action RuleAction, ruleSpec RuleSpec) *Rule {

	ruleSpec.ServiceName = name
	ruleSpec.ServiceNamespace = namespace
	ruleSpec.ListenerPort = port
	ruleSpec.ListenerProtocol = protocol
	ruleSpec.RuleID = id
	ruleSpec.Action = action
	ruleSpec.CreateTime = time.Now()
	rule := &Rule{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::Rule", id),
		Spec:         ruleSpec,
		Status:       nil,
	}

	stack.AddResource(rule)
	return rule
}
