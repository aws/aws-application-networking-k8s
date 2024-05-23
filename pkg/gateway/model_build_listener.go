package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

const (
	awsCustomCertARN = "application-networking.k8s.aws/certificate-arn"
)

func (t *latticeServiceModelBuildTask) extractListenerInfo(
	ctx context.Context,
	parentRef gwv1beta1.ParentReference,
) (int64, string, error) {
	if parentRef.SectionName != nil {
		t.log.Debugf("Listener parentRef SectionName is %s", *parentRef.SectionName)
	}

	t.log.Debugf("Building Listener for Route %s-%s", t.route.Name(), t.route.Namespace())
	gw, err := t.getGateway(ctx)
	if err != nil {
		return 0, "", err
	}
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
			if section.TLS != nil && section.TLS.Mode != nil && *section.TLS.Mode == gwv1.TLSModePassthrough {
				t.log.Debugf("Found TLS passthrough section %v", section.TLS)
				// if the k8s gw.listener section has tls.mode: Passthrough, the lattice service listener protocol should be TLS_PASSTHROUGH
				protocol = vpclattice.ListenerProtocolTlsPassthrough
			}
			return int64(listenerPort), string(protocol), nil
		}
	}
	return 0, "", fmt.Errorf("error building listener, no matching sectionName in parentRef for Name %s, Section %s", parentRef.Name, *parentRef.SectionName)
}

func (t *latticeServiceModelBuildTask) getGateway(ctx context.Context) (*gwv1beta1.Gateway, error) {
	var gwNamespace = t.route.Namespace()
	if t.route.Spec().ParentRefs()[0].Namespace != nil {
		gwNamespace = string(*t.route.Spec().ParentRefs()[0].Namespace)
	}

	gw := &gwv1beta1.Gateway{}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		Name:      string(t.route.Spec().ParentRefs()[0].Name),
	}

	if err := t.client.Get(ctx, gwName, gw); err != nil {
		return nil, fmt.Errorf("failed to get gateway, name %s, err %w", gwName, err)
	}
	return gw, nil
}

func (t *latticeServiceModelBuildTask) buildListeners(ctx context.Context, stackSvcId string) error {
	if len(t.route.Spec().ParentRefs()) == 0 {
		t.log.Debugf("No ParentRefs on route %s-%s, nothing to do", t.route.Name(), t.route.Namespace())
	}
	if !t.route.DeletionTimestamp().IsZero() {
		t.log.Debugf("Route %s-%s is deleted, skipping listener build", t.route.Name(), t.route.Namespace())
		return nil
	}

	for _, parentRef := range t.route.Spec().ParentRefs() {
		if parentRef.Name != t.route.Spec().ParentRefs()[0].Name {
			// when a service is associate to multiple service network(s), all listener config MUST be same
			// so here we are only using the 1st gateway
			t.log.Debugf("Ignore parentref of different gateway %s-%s", parentRef.Name, parentRef.Namespace)
			continue
		}

		port, protocol, err := t.extractListenerInfo(ctx, parentRef)
		if err != nil {
			return err
		}

		spec := model.ListenerSpec{
			StackServiceId:    stackSvcId,
			K8SRouteName:      t.route.Name(),
			K8SRouteNamespace: t.route.Namespace(),
			Port:              port,
			Protocol:          protocol,
		}

		modelListener, err := model.NewListener(t.stack, spec)
		if err != nil {
			return err
		}

		t.log.Debugf("Added listener %s-%s to the stack (ID %s)",
			modelListener.Spec.K8SRouteName, modelListener.Spec.K8SRouteNamespace, modelListener.ID())
	}

	return nil
}
