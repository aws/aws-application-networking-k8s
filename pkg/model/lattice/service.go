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
	CustomerDomainName string    `json:"customerdomainname"`
	IsDeleted          bool
}

type ServiceStatus struct {
	ServiceARN string `json:"latticeServiceARN"`
	ServiceID  string `json:"latticeServiceID"`
	ServiceDNS string `json:"latticeServiceDNS"`
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
