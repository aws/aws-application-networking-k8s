package lattice

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/utils"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

//go:generate mockgen -destination listener_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice ListenerManager

type ListenerManager interface {
	Upsert(ctx context.Context, modelListener *model.Listener, modelSvc *model.Service, moduleTG *model.TargetGroup) (model.ListenerStatus, error)
	Delete(ctx context.Context, modelListener *model.Listener) error
	List(ctx context.Context, serviceID string) ([]*vpclattice.ListenerSummary, error)
}

type defaultListenerManager struct {
	log   gwlog.Logger
	cloud pkg_aws.Cloud
}

func NewListenerManager(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
) *defaultListenerManager {
	return &defaultListenerManager{
		log:   log,
		cloud: cloud,
	}
}

func (d *defaultListenerManager) Upsert(
	ctx context.Context,
	modelListener *model.Listener,
	modelSvc *model.Service,
	modelTG *model.TargetGroup,
) (model.ListenerStatus, error) {
	if modelSvc.Status == nil || modelSvc.Status.Id == "" {
		return model.ListenerStatus{}, errors.New("model service is missing id")
	}

	d.log.Infof("Upsert listener %s-%s", modelListener.Spec.K8SRouteName, modelListener.Spec.K8SRouteNamespace)

	latticeListener, err := d.findListenerByPort(ctx, modelSvc.Status.Id, modelListener.Spec.Port)
	if err != nil {
		return model.ListenerStatus{}, err
	}
	if latticeListener != nil {
		// we do not support listener updates as the only mutable property
		// is the default action, which we set to 404 as required by the gw spec
		// so here we just return the existing one
		d.log.Debugf("Found existing listener %s, nothing to update", aws.StringValue(latticeListener.Id))
		return model.ListenerStatus{
			Name:        aws.StringValue(latticeListener.Name),
			ListenerArn: aws.StringValue(latticeListener.Arn),
			Id:          aws.StringValue(latticeListener.Id),
			ServiceId:   modelSvc.Status.Id,
		}, nil
	}

	// no listener currently exists, create
	defaultStatus := aws.Int64(404)
	defaultResp := vpclattice.FixedResponseAction{
		StatusCode: defaultStatus,
	}
	action := vpclattice.RuleAction{
		FixedResponse: &defaultResp,
	}

	// need to find out the 1st rule
	if modelListener.Spec.Protocol == "TLS_PASSTHROUGH" {

		fmt.Printf("liwwu >> tg id = %v\n", modelTG.Status.Id)

		var latticeTGs []*vpclattice.WeightedTargetGroup

		latticeTG := vpclattice.WeightedTargetGroup{
			TargetGroupIdentifier: aws.String(modelTG.Status.Id),
			Weight:                aws.Int64(1),
		}

		latticeTGs = append(latticeTGs, &latticeTG)

		action = vpclattice.RuleAction{
			Forward: &vpclattice.ForwardAction{
				TargetGroups: latticeTGs,
			},
		}

		fmt.Printf("liwwu >>> action = %v \n", action)
	}
	listenerInput := vpclattice.CreateListenerInput{
		ClientToken:       nil,
		DefaultAction:     &action,
		Name:              aws.String(k8sLatticeListenerName(modelListener)),
		Port:              aws.Int64(modelListener.Spec.Port),
		Protocol:          aws.String(modelListener.Spec.Protocol),
		ServiceIdentifier: aws.String(modelSvc.Status.Id),
		Tags:              d.cloud.DefaultTags(),
	}

	resp, err := d.cloud.Lattice().CreateListenerWithContext(ctx, &listenerInput)
	if err != nil {
		return model.ListenerStatus{},
			fmt.Errorf("Failed CreateListener %s due to %s", aws.StringValue(listenerInput.Name), err)
	}
	d.log.Infof("Success CreateListener %s, %s", aws.StringValue(resp.Name), aws.StringValue(resp.Id))

	return model.ListenerStatus{
		Name:        aws.StringValue(resp.Name),
		ListenerArn: aws.StringValue(resp.Arn),
		Id:          aws.StringValue(resp.Id),
		ServiceId:   modelSvc.Status.Id,
	}, nil
}

func k8sLatticeListenerName(modelListener *model.Listener) string {
	proto := strings.ToLower(modelListener.Spec.Protocol)
	if modelListener.Spec.Protocol == "TLS_PASSTHROUGH" {
		proto = "tls"

	}
	listenerName := fmt.Sprintf("%s-%s-%d-%s",
		utils.Truncate(modelListener.Spec.K8SRouteName, 20),
		utils.Truncate(modelListener.Spec.K8SRouteNamespace, 18),
		modelListener.Spec.Port,
		proto)
	return listenerName
}

func (d *defaultListenerManager) List(ctx context.Context, serviceID string) ([]*vpclattice.ListenerSummary, error) {
	var sdkListeners []*vpclattice.ListenerSummary

	d.log.Debugf("Listing listeners for service %s", serviceID)
	listenerListInput := vpclattice.ListListenersInput{
		ServiceIdentifier: aws.String(serviceID),
	}

	resp, err := d.cloud.Lattice().ListListenersWithContext(ctx, &listenerListInput)
	if err != nil {
		return sdkListeners, err
	}

	for _, r := range resp.Items {
		listener := vpclattice.ListenerSummary{
			Arn:      r.Arn,
			Id:       r.Id,
			Port:     r.Port,
			Protocol: r.Protocol,
			Name:     r.Name,
		}
		sdkListeners = append(sdkListeners, &listener)
	}

	return sdkListeners, nil
}

func (d *defaultListenerManager) findListenerByPort(
	ctx context.Context,
	latticeSvcId string,
	port int64,
) (*vpclattice.ListenerSummary, error) {
	listenerListInput := vpclattice.ListListenersInput{
		ServiceIdentifier: aws.String(latticeSvcId),
	}

	resp, err := d.cloud.Lattice().ListListenersWithContext(ctx, &listenerListInput)
	if err != nil {
		return nil, err
	}

	for _, r := range resp.Items {
		if aws.Int64Value(r.Port) == port {
			d.log.Debugf("Port %d already in use by listener %s for service %s", port, *r.Arn, latticeSvcId)
			return r, nil
		}
	}

	return nil, nil
}

func (d *defaultListenerManager) Delete(ctx context.Context, modelListener *model.Listener) error {
	if modelListener == nil || modelListener.Status == nil {
		return errors.New("model listener and model listener status cannot be nil")
	}

	d.log.Debugf("Deleting listener %s in service %s", modelListener.Status.Id, modelListener.Status.ServiceId)
	listenerDeleteInput := vpclattice.DeleteListenerInput{
		ServiceIdentifier:  aws.String(modelListener.Status.ServiceId),
		ListenerIdentifier: aws.String(modelListener.Status.Id),
	}

	_, err := d.cloud.Lattice().DeleteListenerWithContext(ctx, &listenerDeleteInput)
	if err != nil {
		if services.IsLatticeAPINotFoundErr(err) {
			d.log.Debugf("Listener already deleted")
			return nil
		}
		return fmt.Errorf("Failed DeleteListener %s, %s due to %s", modelListener.Status.Id, modelListener.Status.ServiceId, err)
	}

	d.log.Infof("Success DeleteListener %s, %s", modelListener.Status.Id, modelListener.Status.ServiceId)
	return nil
}
