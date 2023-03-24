package gateway

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/glog"

	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"

	"k8s.io/apimachinery/pkg/types"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	resourceIDListenerConfig = "ListenerConfig"

	awsCustomCertARN = "application-networking.k8s.aws/certificate-arn"
)

func (t *latticeServiceModelBuildTask) extractListnerInfo(ctx context.Context, parentRef gateway_api.ParentReference) (int64, string, string, error) {

	var protocol gateway_api.ProtocolType = gateway_api.HTTPProtocolType
	if parentRef.SectionName != nil {
		glog.V(6).Infof("HTTP SectionName %s \n", *parentRef.SectionName)
	}

	glog.V(6).Infof("Building Listener for HTTPRoute Name %s NameSpace %s\n", t.httpRoute.Name, t.httpRoute.Namespace)
	var gwNamespace = "default"

	if t.httpRoute.Spec.ParentRefs[0].Namespace != nil {
		gwNamespace = string(*t.httpRoute.Spec.ParentRefs[0].Namespace)
	}
	glog.V(6).Infof("build Listener, Parent Name %s Namespace %s\n", t.httpRoute.Spec.ParentRefs[0].Name, gwNamespace)
	var listenerPort = 0
	gw := &gateway_api.Gateway{}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		Name:      string(t.httpRoute.Spec.ParentRefs[0].Name),
	}

	if err := t.Client.Get(ctx, gwName, gw); err != nil {
		glog.V(2).Infof("Failed to build Listener due to unknow http parent ref , Name %v, err %v \n", gwName, err)
		return 0, "", "", err
	}

	var certARN = ""
	// go through parent find out the matching section name
	if parentRef.SectionName != nil {
		glog.V(6).Infof("HTTP SectionName %s \n", *parentRef.SectionName)
		for _, section := range gw.Spec.Listeners {
			glog.V(6).Infof("listener: %v\n", section)
			if section.Name == *parentRef.SectionName {
				listenerPort = int(section.Port)
				protocol = section.Protocol

				if section.TLS != nil {
					if section.TLS.Mode != nil && *section.TLS.Mode == gateway_api.TLSModeTerminate {
						curCertARN, ok := section.TLS.Options[awsCustomCertARN]

						if ok {
							glog.V(6).Infof("Found certification %v under section %v",
								curCertARN, section.Name)
							certARN = string(curCertARN)
						}

					}

				}

			}
		}
	} else {
		// use 1st listener port
		// TODO check no listerner
		if len(gw.Spec.Listeners) == 0 {
			glog.V(2).Infof("Error building listener, there is NO listeners on GW for %v\n",
				gwName)
			return 0, "", "", errors.New("Error building listener, there is NO listeners on GW")
		}
		listenerPort = int(gw.Spec.Listeners[0].Port)
	}

	return int64(listenerPort), string(protocol), certARN, nil

}

func (t *latticeServiceModelBuildTask) buildListener(ctx context.Context) error {
	// when a service is associate to multiple service network(s), all listener config MUST be same
	// so here we are only using the 1st parentRe

	if len(t.httpRoute.Spec.ParentRefs) > 0 {
		parentRef := t.httpRoute.Spec.ParentRefs[0]

		port, protocol, certARN, err := t.extractListnerInfo(ctx, parentRef)

		if err != nil {
			glog.V(6).Infof("Error on buildListener %v\n", err)
			return err
		}
		if t.latticeService != nil {
			t.latticeService.Spec.CustomerCertARN = certARN
		}

		glog.V(6).Infof("Building Listener: found matching listner Port %v\n", port)

		if len(t.httpRoute.Spec.Rules) == 0 {
			glog.V(6).Infof("Error building listener, there is no rules for %v \n", t.httpRoute)
			return errors.New("Error building listener, there are no rules")
		}

		rule := t.httpRoute.Spec.Rules[0]

		if len(rule.BackendRefs) == 0 {
			glog.V(6).Infof("Error building listener, there is no backend refs for %v \n", t.httpRoute)
			return errors.New("Error building listener, there are no backend refs")
		}

		httpBackendRef := rule.BackendRefs[0]

		var is_import = false
		var targetgroupName = ""
		var targetgroupNamespace = "default"

		if string(*httpBackendRef.Kind) == "Service" {
			if httpBackendRef.BackendObjectReference.Namespace != nil {
				targetgroupNamespace = string(*httpBackendRef.BackendObjectReference.Namespace)
			}
			targetgroupName = string(httpBackendRef.BackendObjectReference.Name)
			is_import = false

		}

		if string(*httpBackendRef.Kind) == "ServiceImport" {
			is_import = true
			targetgroupNamespace = "default"
			if httpBackendRef.BackendObjectReference.Namespace != nil {
				targetgroupNamespace = string(*httpBackendRef.BackendObjectReference.Namespace)
			}
			targetgroupName = string(httpBackendRef.BackendObjectReference.Name)

		}

		action := latticemodel.DefaultAction{
			Is_Import:               is_import,
			BackendServiceName:      targetgroupName,
			BackendServiceNamespace: targetgroupNamespace,
		}

		listenerResourceName := fmt.Sprintf("%s-%s-%d-%s", t.httpRoute.Name, t.httpRoute.Namespace, port, protocol)
		glog.V(6).Infof("listenerResourceName : %v \n", listenerResourceName)

		latticemodel.NewListener(t.stack, listenerResourceName, port, protocol, t.httpRoute.Name, t.httpRoute.Namespace, action)
	}

	return nil

}
