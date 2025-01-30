package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type listenerSynthesizer struct {
	log         gwlog.Logger
	listenerMgr ListenerManager
	tgManager   TargetGroupManager
	stack       core.Stack
}

func NewListenerSynthesizer(
	log gwlog.Logger,
	ListenerManager ListenerManager,
	tgManager TargetGroupManager,
	stack core.Stack,
) *listenerSynthesizer {
	return &listenerSynthesizer{
		log:         log,
		listenerMgr: ListenerManager,
		tgManager:   tgManager,
		stack:       stack,
	}
}

func (l *listenerSynthesizer) Synthesize(ctx context.Context) error {
	var stackListeners []*model.Listener

	err := l.stack.ListResources(&stackListeners)
	if err != nil {
		return err
	}

	var listenerErr error
	for _, listener := range stackListeners {
		svc := &model.Service{}
		err := l.stack.GetResource(listener.Spec.StackServiceId, svc)
		if err != nil {
			return err
		}

		if listener.Spec.DefaultAction.Forward != nil {
			// Fill the listener forward action target group ids
			if err := l.tgManager.ResolveRuleTgIds(ctx, listener.Spec.DefaultAction.Forward, l.stack); err != nil {
				return fmt.Errorf("failed to resolve rule tg ids, err = %v", err)
			}
		}

		status, err := l.listenerMgr.Upsert(ctx, listener, svc)
		if err != nil {
			listenerErr = errors.Join(listenerErr,
				fmt.Errorf("failed ListenerManager.Upsert %s-%s due to err %s",
					listener.Spec.K8SRouteName, listener.Spec.K8SRouteNamespace, err))
			continue
		}

		listener.Status = &status
	}

	if listenerErr != nil {
		return listenerErr
	}

	// All deletions happen here, we fetch all listeners for NON-deleted
	// services, since service deletion will delete its listeners
	latticeListenersAsModel, err := l.getLatticeListenersAsModels(ctx)
	if err != nil {
		return err
	}

	for _, latticeListenerAsModel := range latticeListenersAsModel {
		if l.shouldDelete(latticeListenerAsModel, stackListeners) {
			err = l.listenerMgr.Delete(ctx, latticeListenerAsModel)
			if err != nil {
				l.log.Infof(ctx, "Failed ListenerManager.Delete %s due to %s", latticeListenerAsModel.Status.Id, err)
			}
		}
	}

	return nil
}

func (l *listenerSynthesizer) shouldDelete(listenerToFind *model.Listener, stackListeners []*model.Listener) bool {
	for _, candidate := range stackListeners {
		if candidate.Spec.Port == listenerToFind.Spec.Port && candidate.Spec.Protocol == listenerToFind.Spec.Protocol {
			// found a match, do not delete
			return false
		}
	}
	// there is no matching listener
	return true
}

// retrieves all the listeners for all the non-deleted services currently in the stack
func (l *listenerSynthesizer) getLatticeListenersAsModels(ctx context.Context) ([]*model.Listener, error) {
	var latticeListenersAsModel []*model.Listener
	var modelSvcs []*model.Service

	err := l.stack.ListResources(&modelSvcs)
	if err != nil {
		return latticeListenersAsModel, err
	}

	// get the listeners for each service
	for _, modelSvc := range modelSvcs {
		if modelSvc.IsDeleted {
			l.log.Debugf(ctx, "Ignoring deleted service %s", modelSvc.LatticeServiceName())
			continue
		}

		listenerSummaries, err := l.listenerMgr.List(ctx, modelSvc.Status.Id)
		if err != nil {
			l.log.Infof(ctx, "Ignoring error when listing listeners %s", err)
			continue
		}
		for _, latticeListener := range listenerSummaries {

			spec := model.ListenerSpec{
				StackServiceId: modelSvc.ID(),
				Port:           aws.Int64Value(latticeListener.Port),
				Protocol:       aws.StringValue(latticeListener.Protocol),
			}
			status := model.ListenerStatus{
				Name:        aws.StringValue(latticeListener.Name),
				ListenerArn: aws.StringValue(latticeListener.Arn),
				Id:          aws.StringValue(latticeListener.Id),
				ServiceId:   modelSvc.Status.Id,
			}

			latticeListenersAsModel = append(latticeListenersAsModel, &model.Listener{
				Spec:   spec,
				Status: &status,
			})
		}
	}

	return latticeListenersAsModel, nil
}

func (l *listenerSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
