package lattice

import (
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

type Targets struct {
	core.ResourceMeta `json:"-"`
	Spec              TargetsSpec `json:"spec"`
}

type TargetsSpec struct {
	Name          string   `json:"name"`
	Namespace     string   `json:"namespace"`
	TargetGroupID string   `json:"targetgroupID"`
	TargetIPList  []Target `json:"targetIPlist"`
}

type Target struct {
	TargetIP string `json:"targetID"`
	Port     int64  `json:"port"`
}

func NewTargets(stack core.Stack, id string, spec TargetsSpec) *Targets {

	targets := &Targets{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Targets", id),
		Spec:         spec,
	}

	stack.AddResource(targets)

	return targets
}
