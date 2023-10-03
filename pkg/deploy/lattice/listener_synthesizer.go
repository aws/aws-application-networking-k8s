package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type listenerSynthesizer struct {
	log          gwlog.Logger
	listenerMgr  ListenerManager
	stack        core.Stack
	latticestore *latticestore.LatticeDataStore
}

func NewListenerSynthesizer(
	log gwlog.Logger,
	ListenerManager ListenerManager,
	stack core.Stack,
	store *latticestore.LatticeDataStore,
) *listenerSynthesizer {
	return &listenerSynthesizer{
		log:          log,
		listenerMgr:  ListenerManager,
		stack:        stack,
		latticestore: store,
	}
}

func (l *listenerSynthesizer) Synthesize(ctx context.Context) error {
	var resListener []*model.Listener

	err := l.stack.ListResources(&resListener)
	if err != nil {
		l.log.Errorf("Failed to list stack Listeners during Listener synthesis due to err %s", err)
	}

	for _, listener := range resListener {
		l.log.Debugf("Attempting to synthesize listener %s-%s", listener.Spec.Name, listener.Spec.Namespace)
		status, err := l.listenerMgr.Create(ctx, listener)
		if err != nil {
			errmsg := fmt.Sprintf("failed to create listener %s-%s during synthesis due to err %s",
				listener.Spec.Name, listener.Spec.Namespace, err)
			return errors.New(errmsg)
		}

		l.log.Debugf("Successfully synthesized listener %s-%s", listener.Spec.Name, listener.Spec.Namespace)
		l.latticestore.AddListener(listener.Spec.Name, listener.Spec.Namespace, listener.Spec.Port,
			listener.Spec.Protocol, status.ListenerARN, status.ListenerID)
	}

	// handle delete
	sdkListeners, err := l.getSDKListeners(ctx)
	if err != nil {
		l.log.Debugf("Failed to get SDK Listeners during Listener synthesis due to err %s", err)
	}

	for _, sdkListener := range sdkListeners {
		_, err := l.findMatchListener(ctx, sdkListener, resListener)
		if err == nil {
			continue
		}

		l.log.Debugf("Deleting stale SDK Listener %s-%s", sdkListener.Name, sdkListener.Namespace)
		err = l.listenerMgr.Delete(ctx, sdkListener.ListenerID, sdkListener.ServiceID)
		if err != nil {
			l.log.Errorf("Failed to delete SDK Listener %s", sdkListener.ListenerID)
		}

		k8sName, k8sNamespace := latticeName2k8s(sdkListener.Name)
		l.latticestore.DelListener(k8sName, k8sNamespace, sdkListener.Port, sdkListener.Protocol)
	}

	return nil
}

func (l *listenerSynthesizer) findMatchListener(
	ctx context.Context,
	sdkListener *model.ListenerStatus,
	resListener []*model.Listener,
) (model.Listener, error) {
	for _, moduleListener := range resListener {
		if moduleListener.Spec.Port == sdkListener.Port && moduleListener.Spec.Protocol == sdkListener.Protocol {
			return *moduleListener, nil
		}
	}
	return model.Listener{}, errors.New("failed to find matching listener in model")
}

func (l *listenerSynthesizer) getSDKListeners(ctx context.Context) ([]*model.ListenerStatus, error) {
	var sdkListeners []*model.ListenerStatus

	var resService []*model.Service

	err := l.stack.ListResources(&resService)
	if err != nil {
		l.log.Debugf("Ignoring error when listing services %s", err)
	}

	for _, service := range resService {
		latticeService, err := l.listenerMgr.Cloud().Lattice().FindService(ctx, service)
		if err != nil {
			errMessage := fmt.Sprintf("failed to find service in store for service %s-%s due to err %s",
				service.Spec.Name, service.Spec.Namespace, err)
			return sdkListeners, errors.New(errMessage)
		}

		listenerSummaries, err := l.listenerMgr.List(ctx, aws.StringValue(latticeService.Id))
		for _, listenerSummary := range listenerSummaries {
			sdkListeners = append(sdkListeners, &model.ListenerStatus{
				Name:        aws.StringValue(listenerSummary.Name),
				ListenerARN: aws.StringValue(listenerSummary.Arn),
				ListenerID:  aws.StringValue(listenerSummary.Id),
				ServiceID:   aws.StringValue(latticeService.Id),
				Port:        aws.Int64Value(listenerSummary.Port),
				Protocol:    aws.StringValue(listenerSummary.Protocol),
			})
		}
	}

	return sdkListeners, nil
}

func (l *listenerSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
