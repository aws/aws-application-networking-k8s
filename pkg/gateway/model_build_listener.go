package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

const (
	awsCustomCertARN = "application-networking.k8s.aws/certificate-arn"
)

func (t *latticeServiceModelBuildTask) extractListenerInfo(
	ctx context.Context,
	parentRef gwv1.ParentReference,
	gw *gwv1.Gateway,
) (int64, string, error) {
	if parentRef.SectionName != nil {
		t.log.Debugf(ctx, "Listener parentRef SectionName is %s", *parentRef.SectionName)
	}

	t.log.Debugf(ctx, "Building Listener for Route %s-%s", t.route.Name(), t.route.Namespace())
	// If no SectionName is specified, use the first listener port
	if parentRef.SectionName == nil {
		if len(gw.Spec.Listeners) == 0 {
			return 0, "", errors.New("error building listener, there is NO listeners on GW")
		}
		listenerPort := int(gw.Spec.Listeners[0].Port)
		protocol := gw.Spec.Listeners[0].Protocol
		return int64(listenerPort), string(protocol), nil
	}
	// Find the matching section name
	for _, section := range gw.Spec.Listeners {
		if section.Name == *parentRef.SectionName {
			listenerPort := int(section.Port)
			protocol := section.Protocol
			if isTLSPassthroughGatewayListener(&section) {
				t.log.Debugf(ctx, "Found TLS passthrough section %v", section.TLS)
				protocol = vpclattice.ListenerProtocolTlsPassthrough
			}
			return int64(listenerPort), string(protocol), nil
		}
	}
	return 0, "", fmt.Errorf("error building listener, no matching sectionName in parentRef for Name %s, Section %s", parentRef.Name, *parentRef.SectionName)
}

func isTLSPassthroughGatewayListener(listener *gwv1.Listener) bool {
	return listener.Protocol == gwv1.TLSProtocolType && listener.TLS != nil && listener.TLS.Mode != nil && *listener.TLS.Mode == gwv1.TLSModePassthrough
}

func (t *latticeServiceModelBuildTask) findGateway(ctx context.Context) (*gwv1.Gateway, error) {
	gw := &gwv1.Gateway{}
	gwNamespace := t.route.Namespace()
	fails := []string{}
	for _, parentRef := range t.route.Spec().ParentRefs() {
		if parentRef.Namespace != nil {
			gwNamespace = string(*parentRef.Namespace)
		}
		gwName := client.ObjectKey{
			Namespace: gwNamespace,
			Name:      string(parentRef.Name),
		}
		if err := t.client.Get(ctx, gwName, gw); err != nil {
			t.log.Infof(ctx, "Ignoring route %s because failed to get gateway %s: %v", t.route.Name(), parentRef.Name, err)
			continue
		}
		if k8s.IsControlledByLatticeGatewayController(ctx, t.client, gw) {
			return gw, nil
		}
		fails = append(fails, gwName.String())

	}
	return nil, fmt.Errorf("failed to get gateway, name %s", fails)
}

func (t *latticeServiceModelBuildTask) buildListeners(ctx context.Context, stackSvcId string) error {
	if !t.route.DeletionTimestamp().IsZero() {
		t.log.Debugf(ctx, "Route %s-%s is deleted, skipping listener build", t.route.Name(), t.route.Namespace())
		return nil
	}
	if len(t.route.Spec().ParentRefs()) == 0 {
		t.log.Debugf(ctx, "No ParentRefs on route %s-%s, nothing to do", t.route.Name(), t.route.Namespace())
		return nil
	}

	// when a service is associate to multiple service network(s), all listener config MUST be same
	// so here we are only using the 1st gateway
	gw, err := t.findGateway(ctx)
	if err != nil {
		return err
	}

	for _, parentRef := range t.route.Spec().ParentRefs() {
		if string(parentRef.Name) != gw.Name {
			continue
		}

		port, protocol, err := t.extractListenerInfo(ctx, parentRef, gw)
		if err != nil {
			return err
		}

		defaultAction, err := t.getListenerDefaultAction(ctx, protocol)
		if err != nil {
			return err
		}
		spec := model.ListenerSpec{
			StackServiceId:    stackSvcId,
			K8SRouteName:      t.route.Name(),
			K8SRouteNamespace: t.route.Namespace(),
			Port:              port,
			Protocol:          protocol,
			DefaultAction:     defaultAction,
		}

		spec.AdditionalTags = k8s.GetAdditionalTagsFromAnnotations(ctx, t.route.K8sObject())

		modelListener, err := model.NewListener(t.stack, spec)
		if err != nil {
			return err
		}

		t.log.Debugf(ctx, "Added listener %s-%s to the stack (ID %s)",
			modelListener.Spec.K8SRouteName, modelListener.Spec.K8SRouteNamespace, modelListener.ID())
	}

	return nil
}

func (t *latticeServiceModelBuildTask) getListenerDefaultAction(ctx context.Context, modelListenerProtocol string) (
	*model.DefaultAction, error,
) {
	if modelListenerProtocol != vpclattice.ListenerProtocolTlsPassthrough {
		return &model.DefaultAction{
			FixedResponseStatusCode: aws.Int64(model.DefaultActionFixedResponseStatusCode),
		}, nil
	}

	if len(t.route.Spec().Rules()) != 1 {
		return nil, fmt.Errorf("only support exactly 1 rule for TLSRoute %s/%s, but got %d", t.route.Namespace(), t.route.Name(), len(t.route.Spec().Rules()))
	}
	modelRouteRule := t.route.Spec().Rules()[0]
	ruleTgList, err := t.getTargetGroupsForRuleAction(ctx, modelRouteRule)
	if err != nil {
		return nil, err
	}

	return &model.DefaultAction{
		Forward: &model.RuleAction{
			TargetGroups: ruleTgList,
		},
	}, nil
}
