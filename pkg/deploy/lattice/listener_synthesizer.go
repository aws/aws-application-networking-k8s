package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type listenerSynthesizer struct {
	log          gwlog.Logger
	listener     ListenerManager
	stack        core.Stack
	latticeStore *latticestore.LatticeDataStore
}

func NewListenerSynthesizer(
	log gwlog.Logger,
	listenerManager ListenerManager,
	stack core.Stack,
	store *latticestore.LatticeDataStore,
) *listenerSynthesizer {
	return &listenerSynthesizer{
		log:          log,
		listener:     listenerManager,
		stack:        stack,
		latticeStore: store,
	}
}

func (l *listenerSynthesizer) Synthesize(ctx context.Context) error {
	var resListener []*latticemodel.Listener

	l.stack.ListResources(&resListener)

	l.log.Infof("Synthesize Listener:  %v", resListener)

	for _, listener := range resListener {
		status, err := l.listener.Create(ctx, listener)

		if err != nil {
			errmsg := fmt.Sprintf("ListenerSynthesie: failed to create listener %v, err %v", listener, err)
			l.log.Infof("Fail to listenerSynthesizer: %s", errmsg)
			return errors.New(errmsg)
		}

		l.log.Infof("Success synthesize listern %v", listener)
		l.latticeStore.AddListener(listener.Spec.Name, listener.Spec.Namespace, listener.Spec.Port,
			listener.Spec.Protocol,
			status.ListenerARN, status.ListenerID)
	}

	// handle delete
	sdkListeners, err := l.getSDKListeners(ctx)

	l.log.Infof("getSDKlistener: %v, err: %v", sdkListeners, err)

	for _, sdkListener := range sdkListeners {
		_, err := l.findMatchListener(ctx, sdkListener, resListener)

		if err == nil {
			continue
		}

		l.log.Infof("ListenerSynthesize >>>> delete staled sdk listener %v", *sdkListener)
		l.listener.Delete(ctx, sdkListener.ListenerID, sdkListener.ServiceID)
		k8sname, k8snamespace := latticeName2k8s(sdkListener.Name)

		l.latticeStore.DelListener(k8sname, k8snamespace, sdkListener.Port, sdkListener.Protocol)
	}

	return nil
}

func (l *listenerSynthesizer) findMatchListener(ctx context.Context, sdkListener *latticemodel.ListenerStatus, resListener []*latticemodel.Listener) (latticemodel.Listener, error) {

	for _, moduleListener := range resListener {
		if moduleListener.Spec.Port != sdkListener.Port ||
			moduleListener.Spec.Protocol != sdkListener.Protocol {
			l.log.Infof("findMatchListener: skip due to modelListener %v dismatch sdkListener %v",
				moduleListener, sdkListener)
			continue
		}

		l.log.Infof("findMatchListener: found matching moduleListener %v", moduleListener)
		return *moduleListener, nil

	}
	return latticemodel.Listener{}, errors.New("failed to find matching listener in model")
}

func (l *listenerSynthesizer) getSDKListeners(ctx context.Context) ([]*latticemodel.ListenerStatus, error) {
	var sdkListeners []*latticemodel.ListenerStatus

	var resService []*latticemodel.Service

	err := l.stack.ListResources(&resService)

	l.log.Infof("getSDKListeners service: %v err: %v", resService, err)

	for _, service := range resService {
		latticeService, err := l.latticeStore.GetLatticeService(service.Spec.Name, service.Spec.Namespace)
		if err != nil {
			l.log.Infof("getSDKRules: failed to find in store,  service: %v, err: %v", service, err)
			return sdkListeners, errors.New("getSDKRules: failed to find service in store")
		}

		listenerSummaries, err := l.listener.List(ctx, latticeService.ID)

		for _, listenerSummary := range listenerSummaries {

			sdkListeners = append(sdkListeners, &latticemodel.ListenerStatus{
				Name:        aws.StringValue(listenerSummary.Name),
				ListenerARN: aws.StringValue(listenerSummary.Arn),
				ListenerID:  aws.StringValue(listenerSummary.Id),
				ServiceID:   aws.StringValue(&latticeService.ID),
				Port:        aws.Int64Value(listenerSummary.Port),
				Protocol:    aws.StringValue(listenerSummary.Protocol),
			})
		}

	}

	l.log.Infof("getSDKListeners result >> %v", sdkListeners)
	return sdkListeners, nil

}

func (l *listenerSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
