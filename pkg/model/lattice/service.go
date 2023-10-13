package lattice

import (
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

type Service struct {
	core.ResourceMeta `json:"-"`
	Spec              ServiceSpec    `json:"spec"`
	Status            *ServiceStatus `json:"status,omitempty"`
	IsDeleted         bool           `json:"isdeleted"`
}

type ServiceSpec struct {
	Name                string         `json:"name"`
	Namespace           string         `json:"namespace"`
	RouteType           core.RouteType `json:"routetype"`
	ServiceNetworkNames []string       `json:"servicenetworkhnames"`
	CustomerDomainName  string         `json:"customerdomainname"`
	CustomerCertARN     string         `json:"customercertarn"`
}

type ServiceStatus struct {
	Arn string `json:"arn"`
	Id  string `json:"id"`
	Dns string `json:"dns"`
}

func NewLatticeService(stack core.Stack, spec ServiceSpec) (*Service, error) {
	id := spec.LatticeServiceName()

	service := &Service{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::Service", id),
		Spec:         spec,
		Status:       nil,
	}

	err := stack.AddResource(service)
	if err != nil {
		return nil, err
	}

	return service, nil
}

func (s *Service) LatticeServiceName() string {
	return s.Spec.LatticeServiceName()
}

func (s *ServiceSpec) LatticeServiceName() string {
	return utils.LatticeServiceName(s.Name, s.Namespace)
}
