package lattice

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/glog"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type ListenerManager interface {
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

func (s *defaultListenerManager) Create(ctx context.Context, listener *latticemodel.Listener) (latticemodel.ListenerStatus, error) {
	glog.V(6).Infof("Creating listener >>>> %v \n", listener)

	serviceStatus, err := s.latticeDataStore.GetLatticeService(listener.Spec.Name, listener.Spec.Namespace)

	if err != nil {
		errmsg := fmt.Sprintf("Service %v not found during listener creation", listener.Spec)
		glog.V(6).Infof("Error during create listner %s \n", errmsg)
		return latticemodel.ListenerStatus{}, errors.New(errmsg)
	}

	lis, err := s.findListenerByNamePort(ctx, serviceStatus.ID, listener.Spec.Port)

	glog.V(6).Infof("findListenerByNamePort %v , lisenter %v error %v\n", listener, lis, err)

	if err == nil {
		// update Listener
		// TODO
		k8sname, k8snamespace := latticeName2k8s(aws.StringValue(lis.Name))
		return latticemodel.ListenerStatus{
			Name:        k8sname,
			Namespace:   k8snamespace,
			Port:        aws.Int64Value(lis.Port),
			Protocol:    aws.StringValue(lis.Protocol),
			ListenerARN: aws.StringValue(lis.Arn),
			ListenerID:  aws.StringValue(lis.Id),
			ServiceID:   serviceStatus.ID,
		}, nil
	}

	tgName := latticestore.TargetGroupName(listener.Spec.DefaultAction.BackendServiceName, listener.Spec.DefaultAction.BackendServiceNamespace)

	tg, err := s.latticeDataStore.GetTargetGroup(tgName, listener.Spec.DefaultAction.Is_Import)

	if err != nil {
		errmsg := fmt.Sprintf("default target group %s not found during listerner creation %v", tgName, listener.Spec)
		glog.V(6).Infof("Error during create listener: %s\n", errmsg)
		return latticemodel.ListenerStatus{}, errors.New(errmsg)
	}

	listenerInput := vpclattice.CreateListenerInput{
		ClientToken: nil,
		DefaultAction: &vpclattice.RuleAction{
			Forward: &vpclattice.ForwardAction{
				TargetGroups: []*vpclattice.WeightedTargetGroup{
					&vpclattice.WeightedTargetGroup{
						TargetGroupIdentifier: aws.String(tg.ID),
						Weight:                aws.Int64(1)},
				},
			},
		},

		// TODO take care namespace
		Name:              aws.String(k8sLatticeListenerName(listener.Spec.Name, listener.Spec.Namespace, int(listener.Spec.Port), listener.Spec.Protocol)),
		Port:              aws.Int64(listener.Spec.Port),
		Protocol:          aws.String(listener.Spec.Protocol),
		ServiceIdentifier: aws.String(serviceStatus.ID),
		Tags:              nil,
	}

	latticeSess := s.cloud.Lattice()

	resp, err := latticeSess.CreateListener(&listenerInput)

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
		ServiceID:   serviceStatus.ID,
		Port:        listener.Spec.Port,
		Protocol:    listener.Spec.Protocol}, nil
}

func k8s2LatticeName(name string, namespace string) string {
	// TODO handle namespace
	return name

}

func k8sLatticeListenerName(name string, namespace string, port int, protocol string) string {
	listenerName := fmt.Sprintf("%s-%s-%d-%s", name, namespace, port, strings.ToLower(protocol))

	return listenerName
}
func latticeName2k8s(name string) (string, string) {

	// TODO handle namespace
	return name, "default"

}

func (s *defaultListenerManager) List(ctx context.Context, serviceID string) ([]*vpclattice.ListenerSummary, error) {
	var sdkListeners []*vpclattice.ListenerSummary

	glog.V(6).Infof("List - defaultListenerManager  serviceID %v \n", serviceID)
	latticeSess := s.cloud.Lattice()
	listenerListInput := vpclattice.ListListenersInput{
		ServiceIdentifier: aws.String(serviceID),
	}

	resp, err := latticeSess.ListListeners(&listenerListInput)

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

func (s *defaultListenerManager) findListenerByNamePort(ctx context.Context, serviceID string, port int64) (*vpclattice.ListenerSummary, error) {
	glog.V(6).Infof("calling findListenerByNamePort serviceID %v port %d \n", serviceID, port)
	latticeSess := s.cloud.Lattice()
	listenerListInput := vpclattice.ListListenersInput{
		ServiceIdentifier: aws.String(serviceID),
	}

	resp, err := latticeSess.ListListeners(&listenerListInput)

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

func (s *defaultListenerManager) Delete(ctx context.Context, listenerID string, serviceID string) error {

	// TODO
	glog.V(6).Infof("listern--Delete >>> listener %v in service %v\n", listenerID, serviceID)
	listenerDeleteInput := vpclattice.DeleteListenerInput{
		ServiceIdentifier:  aws.String(serviceID),
		ListenerIdentifier: aws.String(listenerID),
	}

	resp, err := s.cloud.Lattice().DeleteListener(&listenerDeleteInput)

	glog.V(2).Infoln("############ req delete listner ###########")
	glog.V(2).Infoln(listenerDeleteInput)
	glog.V(2).Infoln("############resp delete listner ###########")
	glog.V(2).Infof("Delete  listener resp %vm err :%v\n", resp, err)
	return err
}
