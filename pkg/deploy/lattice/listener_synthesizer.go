package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang/glog"

	//"string"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-sdk-go/aws"
)

type listenerSynthesizer struct {
	listener     ListenerManager
	stack        core.Stack
	latticestore *latticestore.LatticeDataStore
}

func NewListenerSynthesizer(ListenerManager ListenerManager, stack core.Stack, store *latticestore.LatticeDataStore) *listenerSynthesizer {

	return &listenerSynthesizer{
		listener:     ListenerManager,
		stack:        stack,
		latticestore: store,
	}
}

func (l *listenerSynthesizer) Synthesize(ctx context.Context) error {
	var resListener []*latticemodel.Listener

	l.stack.ListResources(&resListener)

	glog.V(6).Infof("Synthesize Listener:  %v\n", resListener)

	for _, listener := range resListener {
		status, err := l.listener.Create(ctx, listener)

		if err != nil {
			errmsg := fmt.Sprintf("ListenerSynthesie: failed to create listener %v, err %v", listener, err)
			glog.V(6).Infof("Fail to listenerSynthesizer: %s \n", errmsg)
			return errors.New(errmsg)
		}

		glog.V(6).Infof("Success synthesize listern %v \n", listener)
		l.latticestore.AddListener(listener.Spec.Name, listener.Spec.Namespace, listener.Spec.Port,
			listener.Spec.Protocol,
			status.ListenerARN, status.ListenerID)
	}

	// handle delete
	sdkListeners, err := l.getSDKListeners(ctx)

	glog.V(6).Infof("getSDKlistener: %v, err: %v\n", sdkListeners, err)

	for _, sdkListener := range sdkListeners {
		_, err := l.findMatchListener(ctx, sdkListener, resListener)

		if err == nil {
			continue
		}

		glog.V(6).Infof("ListenerSynthesize >>>> delete staled sdk listener %v\n", *sdkListener)
		l.listener.Delete(ctx, sdkListener.ListenerID, sdkListener.ServiceID)

		l.latticestore.DelListener(sdkListener.Name, sdkListener.Namespace, sdkListener.Port, sdkListener.Protocol)
	}

	return nil
}

func (l *listenerSynthesizer) findMatchListener(ctx context.Context, sdkListener *latticemodel.ListenerStatus, resListener []*latticemodel.Listener) (latticemodel.Listener, error) {

	for _, moduleListener := range resListener {
		if moduleListener.Spec.Port != sdkListener.Port ||
			moduleListener.Spec.Protocol != sdkListener.Protocol {
			glog.V(6).Infof("findMatchListener: skip due to modelListener %v dismatch sdkListener %v\n",
				moduleListener, sdkListener)
			continue
		}

		glog.V(6).Infof("findMatchListener: found matching moduleListener %v \n", moduleListener)
		return *moduleListener, nil

	}
	return latticemodel.Listener{}, errors.New("failed to find matching listener in model")
}

func (l *listenerSynthesizer) getSDKListeners(ctx context.Context) ([]*latticemodel.ListenerStatus, error) {
	var sdkListeners []*latticemodel.ListenerStatus

	var resService []*latticemodel.Service

	err := l.stack.ListResources(&resService)

	glog.V(6).Infof("getSDKListeners service: %v err: %v \n", resService, err)

	for _, service := range resService {
		latticeService, err := l.latticestore.GetLatticeService(service.Spec.Name, service.Spec.Namespace)
		if err != nil {
			glog.V(6).Infof("getSDKRules: failed to find in store,  service: %v, err: %v \n", service, err)
			return sdkListeners, errors.New("getSDKRules: failed to find service in store")
		}

		listenerSummaries, err := l.listener.List(ctx, latticeService.ID)

		for _, listenerSummary := range listenerSummaries {

			sdkListeners = append(sdkListeners, &latticemodel.ListenerStatus{
				Name:        aws.StringValue(listenerSummary.Name),
				Namespace:   service.Spec.Namespace,
				ListenerARN: aws.StringValue(listenerSummary.Arn),
				ListenerID:  aws.StringValue(listenerSummary.Id),
				ServiceID:   aws.StringValue(&latticeService.ID),
				Port:        aws.Int64Value(listenerSummary.Port),
				Protocol:    aws.StringValue(listenerSummary.Protocol),
			})
		}

	}

	glog.V(6).Infof("getSDKListeners result >> %v\n", sdkListeners)
	return sdkListeners, nil

}

func (l *listenerSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
