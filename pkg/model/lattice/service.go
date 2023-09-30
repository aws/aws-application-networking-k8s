package lattice

import (
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

type Service struct {
	core.ResourceMeta `json:"-"`
	Spec              ServiceSpec    `json:"spec"`
	Status            *ServiceStatus `json:"status,omitempty"`
}

type ServiceSpec struct {
	Name                string `json:"name"`
	Namespace           string `json:"namespace"`
	RouteType           core.RouteType
	Protocols           []*string `json:"protocols"`
	ServiceNetworkNames []string  `json:"servicenetworkhname"`
	CustomerDomainName  string    `json:"customerdomainname"`
	CustomerCertARN     string    `json:"customercertarn"`
	IsDeleted           bool
}

type ServiceStatus struct {
	Arn string `json:"latticeServiceARN"`
	Id  string `json:"latticeServiceID"`
	Dns string `json:"latticeServiceDNS"`
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

func (s *Service) LatticeServiceName() string {
	return utils.LatticeServiceName(s.Spec.Name, s.Spec.Namespace)
}
