package gateway

import (
	"context"
	"errors"
	"fmt"

	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"

	"k8s.io/apimachinery/pkg/types"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	resourceIDListenerConfig = "ListenerConfig"

	awsCustomCertARN = "application-networking.k8s.aws/certificate-arn"
)

func (t *latticeServiceModelBuildTask) extractListenerInfo(
	ctx context.Context,
	parentRef gateway_api.ParentReference,
) (int64, string, string, error) {
	protocol := gateway_api.HTTPProtocolType
	if parentRef.SectionName != nil {
		t.log.Infof("SectionName %s", *parentRef.SectionName)
	}

	t.log.Infof("Building Listener for Route Name %s NameSpace %s", t.route.Name(), t.route.Namespace())
	var gwNamespace = t.route.Namespace()
	if t.route.Spec().ParentRefs()[0].Namespace != nil {
		gwNamespace = string(*t.route.Spec().ParentRefs()[0].Namespace)
	}
	t.log.Infof("build Listener, Parent Name %s Namespace %s", t.route.Spec().ParentRefs()[0].Name, gwNamespace)
	var listenerPort = 0
	gw := &gateway_api.Gateway{}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		Name:      string(t.route.Spec().ParentRefs()[0].Name),
	}

	if err := t.client.Get(ctx, gwName, gw); err != nil {
		return 0, "", "", fmt.Errorf("failed to build Listener due to unknow http parent ref, Name %v, err %w", gwName, err)
	}

	var certARN = ""
	// go through parent find out the matching section name
	if parentRef.SectionName != nil {
		t.log.Infof("SectionName %s", *parentRef.SectionName)
		found := false
		for _, section := range gw.Spec.Listeners {
			t.log.Infof("listener: %v", section)
			if section.Name == *parentRef.SectionName {
				listenerPort = int(section.Port)
				protocol = section.Protocol
				found = true

				if section.TLS != nil {
					if section.TLS.Mode != nil && *section.TLS.Mode == gateway_api.TLSModeTerminate {
						curCertARN, ok := section.TLS.Options[awsCustomCertARN]
						if ok {
							t.log.Infof("Found certification %v under section %v",
								curCertARN, section.Name)
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
			t.log.Infof("Error building listener, there is NO listeners on GW for %v",
				gwName)
			return 0, "", "", errors.New("error building listener, there is NO listeners on GW")
		}
		listenerPort = int(gw.Spec.Listeners[0].Port)
	}

	return int64(listenerPort), string(protocol), certARN, nil

}

func (t *latticeServiceModelBuildTask) buildListener(ctx context.Context) error {

	for _, parentRef := range t.route.Spec().ParentRefs() {
		if parentRef.Name != t.route.Spec().ParentRefs()[0].Name {
			// when a service is associate to multiple service network(s), all listener config MUST be same
			// so here we are only using the 1st gateway
			t.log.Infof("Ignore parentref of different gateway %v", parentRef.Name)
			continue
		}

		port, protocol, certARN, err := t.extractListenerInfo(ctx, parentRef)

		if err != nil {
			t.log.Infof("Error on buildListener %v", err)
			return err
		}
		if t.latticeService != nil {
			t.latticeService.Spec.CustomerCertARN = certARN
		}

		t.log.Infof("Building Listener: found matching listner Port %v", port)

		if len(t.route.Spec().Rules()) == 0 {
			return fmt.Errorf("error building listener, there are no rules for %v", t.route)
		}

		rule := t.route.Spec().Rules()[0]

		if len(rule.BackendRefs()) == 0 {
			return fmt.Errorf("error building listener, there are no backend refs for %v", t.route)
		}

		backendRef := rule.BackendRefs()[0]

		var isImport = false
		var targetGroupName = ""
		var targetGroupNamespace = t.route.Namespace()

		if string(*backendRef.Kind()) == "Service" {
			if backendRef.Namespace() != nil {
				targetGroupNamespace = string(*backendRef.Namespace())
			}
			targetGroupName = string(backendRef.Name())
			isImport = false
		}

		if string(*backendRef.Kind()) == "ServiceImport" {
			isImport = true
			if backendRef.Namespace() != nil {
				targetGroupNamespace = string(*backendRef.Namespace())
			}
			targetGroupName = string(backendRef.Name())
		}

		action := latticemodel.DefaultAction{
			Is_Import:               isImport,
			BackendServiceName:      targetGroupName,
			BackendServiceNamespace: targetGroupNamespace,
		}

		listenerResourceName := fmt.Sprintf("%s-%s-%d-%s", t.route.Name(), t.route.Namespace(), port, protocol)
		t.log.Infof("listenerResourceName : %v", listenerResourceName)

		latticemodel.NewListener(t.stack, listenerResourceName, port, protocol, t.route.Name(), t.route.Namespace(), action)
	}

	return nil

}
