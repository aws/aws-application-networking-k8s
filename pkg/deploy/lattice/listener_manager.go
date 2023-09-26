package lattice

import (
	"context"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/utils"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type ListenerManager interface {
	Cloud() lattice_aws.Cloud
	Create(ctx context.Context, service *latticemodel.Listener) (latticemodel.ListenerStatus, error)
	Delete(ctx context.Context, listenerID string, serviceID string) error
	List(ctx context.Context, serviceID string) ([]*vpclattice.ListenerSummary, error)
}

type defaultListenerManager struct {
	log              gwlog.Logger
	cloud            lattice_aws.Cloud
	latticeDataStore *latticestore.LatticeDataStore
}

func NewListenerManager(
	log gwlog.Logger,
	cloud lattice_aws.Cloud,
	latticeDataStore *latticestore.LatticeDataStore,
) *defaultListenerManager {
	return &defaultListenerManager{
		log:              log,
		cloud:            cloud,
		latticeDataStore: latticeDataStore,
	}
}

func (d *defaultListenerManager) Cloud() lattice_aws.Cloud {
	return d.cloud
}

type ListenerLSNProvider struct {
	l *latticemodel.Listener
}

func (r *ListenerLSNProvider) LatticeServiceName() string {
	return utils.LatticeServiceName(r.l.Spec.Name, r.l.Spec.Namespace)
}

func (d *defaultListenerManager) Create(
	ctx context.Context,
	listener *latticemodel.Listener,
) (latticemodel.ListenerStatus, error) {
	listenerSpec := listener.Spec
	d.log.Infof("Creating listener %s-%s", listenerSpec.Name, listenerSpec.Namespace)

	svc, err1 := d.cloud.Lattice().FindService(ctx, &ListenerLSNProvider{listener})
	if err1 != nil {
		if services.IsNotFoundError(err1) {
			errMsg := fmt.Sprintf("Service not found during creation of Listener %s-%s",
				listenerSpec.Name, listenerSpec.Namespace)
			return latticemodel.ListenerStatus{}, fmt.Errorf(errMsg)
		} else {
			return latticemodel.ListenerStatus{}, err1
		}
	}

	lis, err2 := d.findListenerByNamePort(ctx, *svc.Id, listener.Spec.Port)
	if err2 == nil {
		// update Listener
		k8sName, k8sNamespace := latticeName2k8s(aws.StringValue(lis.Name))
		return latticemodel.ListenerStatus{
			Name:        k8sName,
			Namespace:   k8sNamespace,
			Port:        aws.Int64Value(lis.Port),
			Protocol:    aws.StringValue(lis.Protocol),
			ListenerARN: aws.StringValue(lis.Arn),
			ListenerID:  aws.StringValue(lis.Id),
			ServiceID:   aws.StringValue(svc.Id),
		}, nil
	}

	defaultStatus := aws.Int64(404)

	defaultResp := vpclattice.FixedResponseAction{
		StatusCode: defaultStatus,
	}

	listenerInput := vpclattice.CreateListenerInput{
		ClientToken: nil,
		DefaultAction: &vpclattice.RuleAction{
			FixedResponse: &defaultResp,
		},
		Name:              aws.String(k8sLatticeListenerName(listener.Spec.Name, listener.Spec.Namespace, int(listener.Spec.Port), listener.Spec.Protocol)),
		Port:              aws.Int64(listener.Spec.Port),
		Protocol:          aws.String(listener.Spec.Protocol),
		ServiceIdentifier: aws.String(*svc.Id),
		Tags:              nil,
	}

	resp, err := d.cloud.Lattice().CreateListener(&listenerInput)
	if err != nil {
		return latticemodel.ListenerStatus{}, err
	}

	return latticemodel.ListenerStatus{
		Name:        listener.Spec.Name,
		Namespace:   listener.Spec.Namespace,
		ListenerARN: aws.StringValue(resp.Arn),
		ListenerID:  aws.StringValue(resp.Id),
		ServiceID:   aws.StringValue(svc.Id),
		Port:        listener.Spec.Port,
		Protocol:    listener.Spec.Protocol,
	}, nil
}

func k8sLatticeListenerName(name string, namespace string, port int, protocol string) string {
	listenerName := fmt.Sprintf("%s-%s-%d-%s", utils.Truncate(name, 20), utils.Truncate(namespace, 18), port, strings.ToLower(protocol))
	return listenerName
}
func latticeName2k8s(name string) (string, string) {
	// TODO handle namespace
	return name, "default"
}

func (d *defaultListenerManager) List(ctx context.Context, serviceID string) ([]*vpclattice.ListenerSummary, error) {
	var sdkListeners []*vpclattice.ListenerSummary

	d.log.Debugf("Listing listeners for service %s", serviceID)
	listenerListInput := vpclattice.ListListenersInput{
		ServiceIdentifier: aws.String(serviceID),
	}

	resp, err := d.cloud.Lattice().ListListeners(&listenerListInput)
	if err != nil {
		return sdkListeners, err
	}

	d.log.Debugf("Found listeners for service %v", resp.Items)

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

func (d *defaultListenerManager) findListenerByNamePort(
	ctx context.Context,
	serviceId string,
	port int64,
) (*vpclattice.ListenerSummary, error) {
	listenerListInput := vpclattice.ListListenersInput{
		ServiceIdentifier: aws.String(serviceId),
	}

	resp, err := d.cloud.Lattice().ListListenersWithContext(ctx, &listenerListInput)
	if err != nil {
		return nil, err
	}

	for _, r := range resp.Items {
		if aws.Int64Value(r.Port) == port {
			d.log.Debugf("Port %d already in use by listener %s for service %s", port, *r.Arn, serviceId)
			return r, nil
		}
	}

	return nil, fmt.Errorf("listener for service %s and port %d does not exist", serviceId, port)
}

func (d *defaultListenerManager) Delete(ctx context.Context, listenerId string, serviceId string) error {
	d.log.Debugf("Deleting listener %s in service %s", listenerId, serviceId)
	listenerDeleteInput := vpclattice.DeleteListenerInput{
		ServiceIdentifier:  aws.String(serviceId),
		ListenerIdentifier: aws.String(listenerId),
	}

	_, err := d.cloud.Lattice().DeleteListener(&listenerDeleteInput)
	return err
}
