package lattice

import (
	"time"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

type Rule struct {
	core.ResourceMeta `json:"-"`
	Spec              RuleSpec    `json:"spec"`
	Status            *RuleStatus `json:"status,omitempty"`
}

const (
	MaxRulePriority                          = 100
	InvalidBackendRefTgId                    = "INVALID"
	DefaultActionFixedResponseStatusCode     = 404
	InvalidBackendRefFixedResponseStatusCode = 500
)

type RuleSpec struct {
	StackListenerId string                   `json:"stacklistenerid"`
	PathMatchValue  string                   `json:"pathmatchvalue"`
	PathMatchExact  bool                     `json:"pathmatchexact"`
	PathMatchPrefix bool                     `json:"pathmatchprefix"`
	MatchedHeaders  []vpclattice.HeaderMatch `json:"matchedheaders"`
	Method          string                   `json:"method"`
	Priority        int64                    `json:"priority"`
	Action          RuleAction               `json:"action"`
	CreateTime      time.Time                `json:"createtime"`
	AdditionalTags  services.Tags            `json:"additionaltags,omitempty"`
}

type RuleAction struct {
	TargetGroups []*RuleTargetGroup `json:"ruletarget"`
}

type RuleTargetGroup struct {
	StackTargetGroupId string                `json:"stacktargetgroupid"`
	SvcImportTG        *SvcImportTargetGroup `json:"svcimporttg"`
	LatticeTgId        string                `json:"latticetgid"`
	Weight             int64                 `json:"weight"`
}

type SvcImportTargetGroup struct {
	K8SClusterName      string `json:"k8sclustername"`
	K8SServiceName      string `json:"k8sservicename"`
	K8SServiceNamespace string `json:"k8sservicenamespace"`
	VpcId               string `json:"vpcid"`
}

type RuleStatus struct {
	Name       string `json:"name"`
	Arn        string `json:"arn"`
	Id         string `json:"id"`
	ServiceId  string `json:"serviceid"`
	ListenerId string `json:"listenerid"`
	// we submit priority updates as a batch after all rules have been created/modified
	// this ensures we do not set the same priority on two rules at the same time
	// we have the Priority field here for convenience in these scenarios,
	// so we can check for differences and update as a batch when needed
	Priority int64 `json:"priority"`
}

func NewRule(stack core.Stack, spec RuleSpec) (*Rule, error) {
	id, err := core.IdFromHash(spec)
	if err != nil {
		return nil, err
	}

	if spec.CreateTime.IsZero() {
		spec.CreateTime = time.Now()
	}

	rule := &Rule{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::Rule", id),
		Spec:         spec,
		Status:       nil,
	}

	stack.AddResource(rule)
	return rule, nil
}
