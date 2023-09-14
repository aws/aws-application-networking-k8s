package lattice

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"strings"

	"github.com/golang/glog"

	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

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
	cloud            lattice_aws.Cloud
	latticeDataStore *latticestore.LatticeDataStore
}

func NewListenerManager(cloud lattice_aws.Cloud, latticeDataStore *latticestore.LatticeDataStore) *defaultListenerManager {
	return &defaultListenerManager{
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

func (d *defaultListenerManager) Create(ctx context.Context, listener *latticemodel.Listener) (latticemodel.ListenerStatus, error) {
	glog.V(6).Infof("Creating listener >>>> %v \n", listener)

	svc, err1 := d.cloud.Lattice().FindService(ctx, &ListenerLSNProvider{listener})
	if err1 != nil {
		if services.IsNotFoundError(err1) {
			errMsg := fmt.Sprintf("Service %v not found during listener creation", listener.Spec.Name)
			glog.V(6).Infof("Error during create listner %s \n", errMsg)
			return latticemodel.ListenerStatus{}, errors.New(errMsg)
		} else {
			return latticemodel.ListenerStatus{}, err1
		}
	}

	lis, err2 := d.findListenerByNamePort(ctx, *svc.Id, listener.Spec.Port)
	glog.V(6).Infof("findListenerByNamePort %v , listener %v error %v\n", listener, lis, err2)

	if err2 == nil {
		// update Listener
		k8sname, k8snamespace := latticeName2k8s(aws.StringValue(lis.Name))
		return latticemodel.ListenerStatus{
			Name:        k8sname,
			Namespace:   k8snamespace,
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

	glog.V(2).Infoln("############req creating listner ###########")
	glog.V(2).Infoln(listenerInput)
	glog.V(2).Infoln("############resp creating listner ###########")
	glog.V(2).Infof("create listener err :%v\n", err)
	glog.V(2).Infoln(resp)
	return latticemodel.ListenerStatus{
		Name:        listener.Spec.Name,
		Namespace:   listener.Spec.Namespace,
		ListenerARN: aws.StringValue(resp.Arn),
		ListenerID:  aws.StringValue(resp.Id),
		ServiceID:   aws.StringValue(svc.Id),
		Port:        listener.Spec.Port,
		Protocol:    listener.Spec.Protocol}, nil
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

	glog.V(6).Infof("List - defaultListenerManager  serviceID %v \n", serviceID)
	listenerListInput := vpclattice.ListListenersInput{
		ServiceIdentifier: aws.String(serviceID),
	}

	resp, err := d.cloud.Lattice().ListListeners(&listenerListInput)

	if err != nil {
		glog.V(6).Infof("defaultListenerManager: Failed to list service err %v \n", err)
		return sdkListeners, err
	}

	glog.V(6).Infoln("############resp list listener ###########")
	glog.V(6).Infoln(resp)

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

func (d *defaultListenerManager) findListenerByNamePort(ctx context.Context, serviceID string, port int64) (*vpclattice.ListenerSummary, error) {
	glog.V(6).Infof("calling findListenerByNamePort serviceID %v port %d \n", serviceID, port)
	listenerListInput := vpclattice.ListListenersInput{
		ServiceIdentifier: aws.String(serviceID),
	}

	resp, err := d.cloud.Lattice().ListListenersWithContext(ctx, &listenerListInput)

	if err == nil {
		for _, r := range resp.Items {
			glog.V(6).Infof("findListenerByNamePort>> output port %v item: %v \n", port, r)
			if aws.Int64Value(r.Port) == port {
				glog.V(6).Infof("Listener %s Port %v already exists arn: %v \n", serviceID, port, r.Arn)
				return r, nil

			}

		}
	} else {
		return nil, err
	}

	return nil, errors.New("Listener does not exist")
}

func (d *defaultListenerManager) Delete(ctx context.Context, listenerID string, serviceID string) error {

	// TODO
	glog.V(6).Infof("listern--Delete >>> listener %v in service %v\n", listenerID, serviceID)
	listenerDeleteInput := vpclattice.DeleteListenerInput{
		ServiceIdentifier:  aws.String(serviceID),
		ListenerIdentifier: aws.String(listenerID),
	}

	resp, err := d.cloud.Lattice().DeleteListener(&listenerDeleteInput)

	glog.V(2).Infoln("############ req delete listner ###########")
	glog.V(2).Infoln(listenerDeleteInput)
	glog.V(2).Infoln("############resp delete listner ###########")
	glog.V(2).Infof("Delete  listener resp %vm err :%v\n", resp, err)
	return err
}
