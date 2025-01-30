package lattice

import (
	"context"
	"errors"
	"fmt"
	"reflect"

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
	Upsert(ctx context.Context, modelListener *model.Listener, modelSvc *model.Service) (model.ListenerStatus, error)
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
) (model.ListenerStatus, error) {
	if modelSvc.Status == nil || modelSvc.Status.Id == "" {
		return model.ListenerStatus{}, errors.New("model service is missing id")
	}

	d.log.Infof(ctx, "Upsert listener %s-%s", modelListener.Spec.K8SRouteName, modelListener.Spec.K8SRouteNamespace)
	latticeSvcId := modelSvc.Status.Id
	latticeListenerSummary, err := d.findListenerByPort(ctx, latticeSvcId, modelListener.Spec.Port)

	if err != nil {
		return model.ListenerStatus{}, err
	}

	defaultAction, err := d.getLatticeListenerDefaultAction(ctx, modelListener)
	if err != nil {
		return model.ListenerStatus{}, err
	}

	if latticeListenerSummary == nil {
		// listener not found, create new one
		return d.create(ctx, latticeSvcId, modelListener, defaultAction)
	}

	existingListenerStatus := model.ListenerStatus{
		Name:        aws.StringValue(latticeListenerSummary.Name),
		ListenerArn: aws.StringValue(latticeListenerSummary.Arn),
		Id:          aws.StringValue(latticeListenerSummary.Id),
		ServiceId:   latticeSvcId,
	}
	if modelListener.Spec.Protocol != vpclattice.ListenerProtocolTlsPassthrough {
		// The only mutable field for lattice listener is defaultAction, for non-TLS_PASSTHROUGH listener, the defaultAction is always the FixedResponse 404. Don't need to update.
		return existingListenerStatus, nil
	}

	// For TLS_PASSTHROUGH listener, check whether it needs to update defaultAction
	needToUpdateDefaultAction, err := d.needToUpdateDefaultAction(ctx, latticeSvcId, *latticeListenerSummary.Id, defaultAction)
	if err != nil {
		return model.ListenerStatus{}, err
	}
	if needToUpdateDefaultAction {
		if err = d.update(ctx, latticeSvcId, latticeListenerSummary, defaultAction); err != nil {
			return model.ListenerStatus{}, err
		}
	}
	return existingListenerStatus, nil
}

func (d *defaultListenerManager) create(ctx context.Context, latticeSvcId string, modelListener *model.Listener, defaultAction *vpclattice.RuleAction) (
	model.ListenerStatus, error) {
	listenerInput := vpclattice.CreateListenerInput{
		ClientToken:       nil,
		DefaultAction:     defaultAction,
		Name:              aws.String(k8sLatticeListenerName(modelListener)),
		Port:              aws.Int64(modelListener.Spec.Port),
		Protocol:          aws.String(modelListener.Spec.Protocol),
		ServiceIdentifier: aws.String(latticeSvcId),
		Tags:              d.cloud.DefaultTags(),
	}

	resp, err := d.cloud.Lattice().CreateListenerWithContext(ctx, &listenerInput)
	if err != nil {
		return model.ListenerStatus{},
			fmt.Errorf("Failed CreateListener %s due to %s", aws.StringValue(listenerInput.Name), err)
	}
	d.log.Infof(ctx, "Success CreateListener %s, %s", aws.StringValue(resp.Name), aws.StringValue(resp.Id))

	return model.ListenerStatus{
		Name:        aws.StringValue(resp.Name),
		ListenerArn: aws.StringValue(resp.Arn),
		Id:          aws.StringValue(resp.Id),
		ServiceId:   latticeSvcId,
	}, nil
}

func (d *defaultListenerManager) update(ctx context.Context, latticeSvcId string, listener *vpclattice.ListenerSummary, defaultAction *vpclattice.RuleAction) error {

	d.log.Debugf(ctx, "Updating listener %s default action", aws.StringValue(listener.Id))
	_, err := d.cloud.Lattice().UpdateListenerWithContext(ctx, &vpclattice.UpdateListenerInput{
		DefaultAction:      defaultAction,
		ListenerIdentifier: listener.Id,
		ServiceIdentifier:  aws.String(latticeSvcId),
	})
	if err != nil {
		return fmt.Errorf("failed to update lattice listener %s due to %s", aws.StringValue(listener.Id), err)
	}
	d.log.Infof(ctx, "Success update listener %s default action", aws.StringValue(listener.Id))
	return nil
}

func (d *defaultListenerManager) getLatticeListenerDefaultAction(ctx context.Context, stackListener *model.Listener) (*vpclattice.RuleAction, error) {
	if stackListener.Spec.DefaultAction.FixedResponseStatusCode != nil {
		return &vpclattice.RuleAction{
			FixedResponse: &vpclattice.FixedResponseAction{
				StatusCode: stackListener.Spec.DefaultAction.FixedResponseStatusCode,
			},
		}, nil
	}
	hasValidTargetGroup := false
	for _, tg := range stackListener.Spec.DefaultAction.Forward.TargetGroups {
		if tg.LatticeTgId != model.InvalidBackendRefTgId {
			hasValidTargetGroup = true
			break
		}
	}
	if !hasValidTargetGroup {
		if stackListener.Spec.Protocol == vpclattice.ListenerProtocolTlsPassthrough {
			return nil, fmt.Errorf("TLSRoute %s/%s must have at least one valid backendRef target group", stackListener.Spec.K8SRouteNamespace, stackListener.Spec.K8SRouteName)
		} else {
			return nil, fmt.Errorf("unreachable code, since the defaultAction for non-TLS_PASSTHROUGH listener is always the FixedResponse 404")
		}
	}

	var latticeTGs []*vpclattice.WeightedTargetGroup
	for _, modelTg := range stackListener.Spec.DefaultAction.Forward.TargetGroups {
		// skip any invalid TGs - eventually VPC Lattice may support weighted fixed response
		// and this logic can be more in line with the spec
		if modelTg.LatticeTgId == model.InvalidBackendRefTgId {
			continue
		}
		latticeTG := vpclattice.WeightedTargetGroup{
			TargetGroupIdentifier: aws.String(modelTg.LatticeTgId),
			Weight:                aws.Int64(modelTg.Weight),
		}
		latticeTGs = append(latticeTGs, &latticeTG)
	}

	d.log.Debugf(ctx, "DefaultAction Forward target groups: %v", latticeTGs)
	return &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: latticeTGs,
		},
	}, nil
}

func k8sLatticeListenerName(modelListener *model.Listener) string {
	protocol := strings.ToLower(modelListener.Spec.Protocol)
	if modelListener.Spec.Protocol == vpclattice.ListenerProtocolTlsPassthrough {
		protocol = "tls"
	}

	listenerName := fmt.Sprintf("%s-%s-%d-%s",
		utils.Truncate(modelListener.Spec.K8SRouteName, 20),
		utils.Truncate(modelListener.Spec.K8SRouteNamespace, 18),
		modelListener.Spec.Port,
		protocol)
	return listenerName
}

func (d *defaultListenerManager) List(ctx context.Context, serviceID string) ([]*vpclattice.ListenerSummary, error) {
	var sdkListeners []*vpclattice.ListenerSummary

	d.log.Debugf(ctx, "Listing listeners for service %s", serviceID)
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

func (d *defaultListenerManager) needToUpdateDefaultAction(
	ctx context.Context,
	latticeSvcId string,
	latticeListenerId string,
	listenerDefaultActionFromStack *vpclattice.RuleAction) (bool, error) {

	resp, err := d.cloud.Lattice().GetListenerWithContext(ctx, &vpclattice.GetListenerInput{
		ServiceIdentifier:  &latticeSvcId,
		ListenerIdentifier: &latticeListenerId,
	})
	if err != nil {
		return false, err
	}

	return !reflect.DeepEqual(resp.DefaultAction, listenerDefaultActionFromStack), nil
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
			d.log.Debugf(ctx, "Port %d already in use by listener %s for service %s", port, *r.Arn, latticeSvcId)
			return r, nil
		}
	}

	return nil, nil
}

func (d *defaultListenerManager) Delete(ctx context.Context, modelListener *model.Listener) error {
	if modelListener == nil || modelListener.Status == nil {
		return errors.New("model listener and model listener status cannot be nil")
	}

	d.log.Debugf(ctx, "Deleting listener %s in service %s", modelListener.Status.Id, modelListener.Status.ServiceId)
	listenerDeleteInput := vpclattice.DeleteListenerInput{
		ServiceIdentifier:  aws.String(modelListener.Status.ServiceId),
		ListenerIdentifier: aws.String(modelListener.Status.Id),
	}

	_, err := d.cloud.Lattice().DeleteListenerWithContext(ctx, &listenerDeleteInput)
	if err != nil {
		if services.IsLatticeAPINotFoundErr(err) {
			d.log.Debugf(ctx, "Listener already deleted")
			return nil
		}
		return fmt.Errorf("Failed DeleteListener %s, %s due to %s", modelListener.Status.Id, modelListener.Status.ServiceId, err)
	}

	d.log.Infof(ctx, "Success DeleteListener %s, %s", modelListener.Status.Id, modelListener.Status.ServiceId)
	return nil
}
