package lattice

import (
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

type Service struct {
	core.ResourceMeta `json:"-"`

	Spec ServiceSpec `json:"spec"`

	Status *ServiceStatus `json:"status,omitempty"`
}

type ServiceSpec struct {
	Name               string    `json:"name"`
	Namespace          string    `json:"namespace"`
	Protocols          []*string `json:"protocols"`
	ServiceNetworkName string    `json:"servicenetworkhname"`
	IsDeleted          bool
}

type ServiceStatus struct {
	ServiceARN string `json:"mercuryServiceARN"`
	ServiceID  string `json:"mercuryServiceID"`
	ServiceDNS string `json:"mercuryServiceDNS"`
}

func NewLatticeService(stack core.Stack, id string, spec ServiceSpec) *Service {
	service := &Service{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::Service", id),
		Spec:         spec,
		Status:       nil,
	}

	stack.AddResource(service)

	return service
}
