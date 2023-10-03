package gateway

import (
	"context"
	"errors"
	"fmt"

	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"

	"k8s.io/apimachinery/pkg/types"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	awsCustomCertARN = "application-networking.k8s.aws/certificate-arn"
)

func (t *latticeServiceModelBuildTask) extractListenerInfo(
	ctx context.Context,
	parentRef gwv1beta1.ParentReference,
) (int64, string, string, error) {
	protocol := gwv1beta1.HTTPProtocolType
	if parentRef.SectionName != nil {
		t.log.Debugf("Listener parentRef SectionName is %s", *parentRef.SectionName)
	}

	t.log.Debugf("Building Listener for Route %s-%s", t.route.Name(), t.route.Namespace())
	var gwNamespace = t.route.Namespace()
	if t.route.Spec().ParentRefs()[0].Namespace != nil {
		gwNamespace = string(*t.route.Spec().ParentRefs()[0].Namespace)
	}

	var listenerPort = 0
	gw := &gwv1beta1.Gateway{}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		Name:      string(t.route.Spec().ParentRefs()[0].Name),
	}

	if err := t.client.Get(ctx, gwName, gw); err != nil {
		return 0, "", "", fmt.Errorf("failed to build Listener due to unknow http parent ref, Name %s, err %w", gwName, err)
	}

	var certARN = ""
	// go through parent find out the matching section name
	if parentRef.SectionName != nil {
		found := false
		for _, section := range gw.Spec.Listeners {
			if section.Name == *parentRef.SectionName {
				listenerPort = int(section.Port)
				protocol = section.Protocol
				found = true

				if section.TLS != nil {
					if section.TLS.Mode != nil && *section.TLS.Mode == gwv1beta1.TLSModeTerminate {
						curCertARN, ok := section.TLS.Options[awsCustomCertARN]
						if ok {
							t.log.Debugf("Found certification %s under section %s", curCertARN, section.Name)
							certARN = string(curCertARN)
						}
					}
				}
				break
			}
		}
		if !found {
			return 0, "", "", fmt.Errorf("error building listener, no matching sectionName in parentRef for Name %s, Section %s", parentRef.Name, *parentRef.SectionName)
		}
	} else {
		// use 1st listener port
		// TODO check no listener
		if len(gw.Spec.Listeners) == 0 {
			return 0, "", "", errors.New("error building listener, there is NO listeners on GW")
		}
		listenerPort = int(gw.Spec.Listeners[0].Port)
	}

	return int64(listenerPort), string(protocol), certARN, nil

}

func (t *latticeServiceModelBuildTask) buildListeners(ctx context.Context) error {
	for _, parentRef := range t.route.Spec().ParentRefs() {
		if parentRef.Name != t.route.Spec().ParentRefs()[0].Name {
			// when a service is associate to multiple service network(s), all listener config MUST be same
			// so here we are only using the 1st gateway
			t.log.Debugf("Ignore parentref of different gateway %s-%s", parentRef.Name, parentRef.Namespace)
			continue
		}

		port, protocol, certARN, err := t.extractListenerInfo(ctx, parentRef)
		if err != nil {
			return err
		}

		if t.latticeService != nil {
			t.latticeService.Spec.CustomerCertARN = certARN
		}

		t.log.Debugf("Building Listener: found matching listner Port %d", port)

		if len(t.route.Spec().Rules()) == 0 {
			return fmt.Errorf("error building listener, there are no rules for route %s-%s",
				t.route.Name(), t.route.Namespace())
		}

		rule := t.route.Spec().Rules()[0]

		if len(rule.BackendRefs()) == 0 {
			return fmt.Errorf("error building listener, there are no backend refs for route %s-%s",
				t.route.Name(), t.route.Namespace())
		}

		backendRef := rule.BackendRefs()[0]
		targetGroupName := string(backendRef.Name())

		var targetGroupNamespace = t.route.Namespace()
		if backendRef.Namespace() != nil {
			targetGroupNamespace = string(*backendRef.Namespace())
		}

		action := model.DefaultAction{
			BackendServiceName:      targetGroupName,
			BackendServiceNamespace: targetGroupNamespace,
		}

		listenerResourceName := fmt.Sprintf("%s-%s-%d-%s", t.route.Name(), t.route.Namespace(), port, protocol)
		t.log.Infof("Creating new listener with name %s", listenerResourceName)
		model.NewListener(t.stack, listenerResourceName, port, protocol, t.route.Name(), t.route.Namespace(), action)
	}

	return nil
}
