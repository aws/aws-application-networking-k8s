package lattice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

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

func (s *defaultListenerManager) Create(ctx context.Context, listener *latticemodel.Listener) (latticemodel.ListenerStatus, error) {
	s.log.Infof("Creating listener >>>> %v", listener)

	serviceStatus, err := s.latticeDataStore.GetLatticeService(listener.Spec.Name, listener.Spec.Namespace)

	if err != nil {
		errmsg := fmt.Sprintf("Service %v not found during listener creation", listener.Spec)
		s.log.Infof("Error during create listner %s", errmsg)
		return latticemodel.ListenerStatus{}, errors.New(errmsg)
	}

	lis, err := s.findListenerByNamePort(ctx, serviceStatus.ID, listener.Spec.Port)

	s.log.Infof("findListenerByNamePort %v , lisenter %v error %v", listener, lis, err)

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
		ServiceIdentifier: aws.String(serviceStatus.ID),
		Tags:              nil,
	}

	latticeSess := s.cloud.Lattice()

	resp, err := latticeSess.CreateListener(&listenerInput)

	s.log.Debugln("############req creating listner ###########")
	s.log.Debugln(listenerInput)
	s.log.Debugln("############resp creating listner ###########")
	s.log.Debugf("create listener err :%v", err)
	s.log.Debugln(resp)
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
	listenerName := fmt.Sprintf("%s-%s-%d-%s", utils.Truncate(name, 20), utils.Truncate(namespace, 18), port, strings.ToLower(protocol))

	return listenerName
}
func latticeName2k8s(name string) (string, string) {

	// TODO handle namespace
	return name, "default"

}

func (s *defaultListenerManager) List(ctx context.Context, serviceID string) ([]*vpclattice.ListenerSummary, error) {
	var sdkListeners []*vpclattice.ListenerSummary

	s.log.Infof("List - defaultListenerManager  serviceID %v", serviceID)
	latticeSess := s.cloud.Lattice()
	listenerListInput := vpclattice.ListListenersInput{
		ServiceIdentifier: aws.String(serviceID),
	}

	resp, err := latticeSess.ListListeners(&listenerListInput)

	if err != nil {
		s.log.Infof("defaultListenerManager: Failed to list service err %v", err)
		return sdkListeners, err
	}

	s.log.Infoln("############resp list listener ###########")
	s.log.Infoln(resp)

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
	s.log.Infof("calling findListenerByNamePort serviceID %v port %d", serviceID, port)
	latticeSess := s.cloud.Lattice()
	listenerListInput := vpclattice.ListListenersInput{
		ServiceIdentifier: aws.String(serviceID),
	}

	resp, err := latticeSess.ListListeners(&listenerListInput)

	if err == nil {
		for _, r := range resp.Items {
			s.log.Infof("findListenerByNamePort>> output port %v item: %v", port, r)
			if aws.Int64Value(r.Port) == port {
				s.log.Infof("Listener %s Port %v already exists arn: %v", serviceID, port, r.Arn)
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
	s.log.Infof("listern--Delete >>> listener %v in service %v", listenerID, serviceID)
	listenerDeleteInput := vpclattice.DeleteListenerInput{
		ServiceIdentifier:  aws.String(serviceID),
		ListenerIdentifier: aws.String(listenerID),
	}

	resp, err := s.cloud.Lattice().DeleteListener(&listenerDeleteInput)

	s.log.Debugln("############ req delete listner ###########")
	s.log.Debugln(listenerDeleteInput)
	s.log.Debugln("############resp delete listner ###########")
	s.log.Debugf("Delete  listener resp %vm err :%v", resp, err)
	return err
}
