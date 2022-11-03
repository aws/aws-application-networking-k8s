package lattice

import (
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

type TargetGroup struct {
	core.ResourceMeta `json:"-"`
	Spec              TargetGroupSpec    `json:"spec"`
	Status            *TargetGroupStatus `json:"status,omitempty"`
}

type TargetGroupSpec struct {
	Name      string
	Config    TargetGroupConfig `json:"config"`
	Type      TargetGroupType
	IsDeleted bool
	LatticeID string
}

type TargetGroupConfig struct {
	Port            int32  `json:"port"`
	Protocol        string `json:"protocol"`
	ProtocolVersion string `json:"protocolversion"`
	VpcID           string `json:"vpcid"`
	EKSClusterName  string `json:"eksclustername"`
	IsServiceImport bool   `json:"serviceimport"`
}

type TargetGroupStatus struct {
	TargetGroupARN string `json:"latticeServiceARN"`
	TargetGroupID  string `json:"latticeServiceID"`
}

type TargetGroupType string

const (
	TargetGroupTypeIP TargetGroupType = "IP"
)

func NewTargetGroup(stack core.Stack, id string, spec TargetGroupSpec) *TargetGroup {
	tg := &TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", id),
		Spec:         spec,
		Status:       nil,
	}

	stack.AddResource(tg)

	return tg
}
