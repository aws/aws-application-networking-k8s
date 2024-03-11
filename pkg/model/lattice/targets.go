package lattice

import (
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"k8s.io/apimachinery/pkg/types"
)

type Targets struct {
	core.ResourceMeta `json:"-"`
	Spec              TargetsSpec `json:"spec"`
}

// unlike target groups, which can reference a service export, targets
// are always sourced from the local cluster. When we update targets,
// we find all the target groups linked to the specific service
type TargetsSpec struct {
	StackTargetGroupId string   `json:"stacktargetgroupid"`
	TargetList         []Target `json:"targetlist"`
}

type Target struct {
	TargetIP  string `json:"targetip"`
	Port      int64  `json:"port"`
	Ready     bool   `json:"ready"`
	TargetRef types.NamespacedName
}

func NewTargets(stack core.Stack, spec TargetsSpec) (*Targets, error) {
	id, err := core.IdFromHash(spec)
	if err != nil {
		return nil, err
	}
	targets := &Targets{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Targets", id),
		Spec:         spec,
	}

	stack.AddResource(targets)

	return targets, nil
}
