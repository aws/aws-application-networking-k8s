package lattice

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

var validListenerProtocols = map[string]struct{}{
	"HTTP":            {},
	"HTTPS":           {},
	"TLS_PASSTHROUGH": {},
}

type Listener struct {
	core.ResourceMeta `json:"-"`
	Spec              ListenerSpec    `json:"spec"`
	Status            *ListenerStatus `json:"status,omitempty"`
}

type ListenerSpec struct {
	StackServiceId    string         `json:"stackserviceid"`
	K8SRouteName      string         `json:"k8sroutename"`
	K8SRouteNamespace string         `json:"k8sroutenamespace"`
	Port              int64          `json:"port"`
	Protocol          string         `json:"protocol"`
	DefaultAction     *DefaultAction `json:"defaultaction"`
}

type DefaultAction struct {
	FixedResponseStatusCode *int64      `json:"fixedresponsestatuscode"`
	Forward                 *RuleAction `json:"forward"`
}

type ListenerStatus struct {
	Name        string `json:"name"`
	ListenerArn string `json:"listenerarn"`
	Id          string `json:"listenerid"`
	ServiceId   string `json:"serviceid"`
}

func NewListener(stack core.Stack, spec ListenerSpec) (*Listener, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	id, err := core.IdFromHash(spec)
	if err != nil {
		return nil, err
	}

	listener := &Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::Listener", id),
		Spec:         spec,
		Status:       nil,
	}

	stack.AddResource(listener)

	return listener, nil
}

func (spec *ListenerSpec) Validate() error {
	if _, exists := validListenerProtocols[spec.Protocol]; !exists {
		return fmt.Errorf("invalid listener protocol %s", spec.Protocol)
	}
	if spec.DefaultAction == nil {
		return fmt.Errorf("listener default action is required")
	}
	isFixedResponse := spec.DefaultAction.FixedResponseStatusCode != nil
	isForward := spec.DefaultAction.Forward != nil
	if isFixedResponse == isForward { // either both true or both false
		return fmt.Errorf("invalid listener default action, must be either fixed response or forward")
	}
	if spec.Protocol != vpclattice.ListenerProtocolTlsPassthrough && !isFixedResponse {
		return fmt.Errorf("non TLS_PASSTHROUGH listener default action must be fixed response")
	}
	if spec.Protocol == vpclattice.ListenerProtocolTlsPassthrough && !isForward {
		return fmt.Errorf("TLS_PASSTHROUGH listener default action must be forward")
	}
	return nil
}
