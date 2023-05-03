package latticestore

import (
	"errors"
	"fmt"
	"github.com/golang/glog"
	"sync"
)

// ERROR CODE
const (
	DATASTORE_SERVICE_NETWORK_NOT_EXIST = "service network does not exist in Data Store"
	DATASTORE_SERVICE_NOT_EXIST         = "service does not exist in Data Store"
	DATASTORE_TG_NOT_EXIST              = "target Group does not exist in Data Store"
	DATASTORE_LISTENER_NOT_EXIST        = "listener does not exist in Data Store"
)

// Status
const (
	DATASTORE_SERVICE_NETWORK_CREATE_IN_PROGRESS = "service network is create-in-progress"
	DATASTORE_SERVICE_NETWORK_CREATED            = "service network is created and associated"
)

// this package is used to cache lattice info that relates to K8S object.
// e.g. the AWSARN for the matching K8S object

type ServiceNetworkKey struct {
	Name      string
	AccountID string
}

type ServiceNetwork struct {
	Key    ServiceNetworkKey `json:"meshkey"`
	ARN    string            `json:"arn"`
	ID     string            `json:"id"`
	Status string            `json:"status"`
}

type ServiceNetworkPool map[ServiceNetworkKey]*ServiceNetwork

type LatticeServiceKey struct {
	Name      string
	Namespace string
}

type LatticeService struct {
	LatticeServiceKey LatticeServiceKey
	ARN               string
	ID                string
	DNS               string
}

type LatticeServicePool map[LatticeServiceKey]*LatticeService

type ListenerKey struct {
	Name      string
	Namespace string
	Port      int64
	Protocol  string
	//TODO for TLS we need to add Protocol
}

type Listener struct {
	Key ListenerKey
	ARN string
	ID  string
}

type ListenerPool map[ListenerKey]*Listener

type TargetGroupKey struct {
	Name            string
	RouteName       string
	IsServiceImport bool
}

type TargetGroup struct {
	TargetGroupKey  TargetGroupKey
	ARN             string
	ID              string
	EndPoints       []Target
	VpcID           string
	ByServiceExport bool // triggered by K8S serviceexport object
	ByBackendRef    bool // triggered by backend ref which points to service
}

type Target struct {
	TargetIP   string
	TargetPort int64
}

type TargetGroupPool map[TargetGroupKey]*TargetGroup

type LatticeDataStore struct {
	lock sync.Mutex

	serviceNetworks ServiceNetworkPool
	latticeServices LatticeServicePool
	targetGroups    TargetGroupPool
	listeners       ListenerPool
}

type LatticeDataStoreInfo struct {
	ServiceNetworks map[string]ServiceNetwork
	LatticeServices map[string]LatticeService
	TargetGroups    map[string]TargetGroup
	Listeners       map[string]Listener
}

var defaultLatticeDataStore *LatticeDataStore

func NewLatticeDataStore() *LatticeDataStore {
	defaultLatticeDataStore = &LatticeDataStore{
		serviceNetworks: make(ServiceNetworkPool),
		latticeServices: make(LatticeServicePool),
		targetGroups:    make(TargetGroupPool),
		listeners:       make(ListenerPool),
	}

	return defaultLatticeDataStore
}

func dumpCurrentLatticeDataStore(ds *LatticeDataStore) *LatticeDataStoreInfo {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	var store = LatticeDataStoreInfo{
		ServiceNetworks: make(map[string]ServiceNetwork),
		LatticeServices: make(map[string]LatticeService),
		TargetGroups:    make(map[string]TargetGroup),
		Listeners:       make(map[string]Listener),
	}

	for _, sn := range ds.serviceNetworks {
		key := fmt.Sprintf("%s-%s", sn.Key.Name, sn.Key.AccountID)
		store.ServiceNetworks[key] = *sn
	}

	for _, svc := range ds.latticeServices {
		key := fmt.Sprintf("%s-%s", svc.LatticeServiceKey.Name, svc.LatticeServiceKey.Namespace)
		store.LatticeServices[key] = *svc
	}

	for tgkey, targetgroup := range ds.targetGroups {

		key := fmt.Sprintf("%s-%s", tgkey.Name, targetgroup.VpcID)
		store.TargetGroups[key] = *targetgroup
	}

	for listenerKey, listener := range ds.listeners {
		key := fmt.Sprintf("%s-%s-%d", listenerKey.Name, listener.Key.Namespace, listenerKey.Port)
		store.Listeners[key] = *listener
	}

	return &store

}
func GetDefaultLatticeDataStore() *LatticeDataStore {
	return defaultLatticeDataStore
}

func (ds *LatticeDataStore) AddServiceNetwork(name string, account string, arn string, id string, status string) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	glog.V(6).Infof("AddServiceNetwork name:[%s], account[%s] arn[%s], id[%s]\n", name, account, arn, id)

	Key := ServiceNetworkKey{Name: name, AccountID: account}
	_, ok := ds.serviceNetworks[Key]

	if ok {
		glog.V(6).Infof("UpdateServiceNetwork name:[%s], account[%s] arn[%s] id[%s]\n", name, account, arn, id)
	}

	ds.serviceNetworks[Key] = &ServiceNetwork{
		Key:    Key,
		ARN:    arn,
		ID:     id,
		Status: status,
	}

	return nil
}

func (ds *LatticeDataStore) DelServiceNetwork(name string, account string) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	glog.V(6).Infof("DelServiceNetwork name:[%s], account[%s] \n", name, account)

	key := ServiceNetworkKey{Name: name, AccountID: account}
	_, ok := ds.serviceNetworks[key]

	if !ok {
		glog.V(6).Infof("Deleting unknown service network: name:[%s], account[%s] \n", name, account)
		return errors.New(DATASTORE_SERVICE_NETWORK_NOT_EXIST)
	}

	delete(ds.serviceNetworks, key)
	return nil
}

func (ds *LatticeDataStore) GetServiceNetworkStatus(name string, account string) (ServiceNetwork, error) {
	ds.lock.Lock()
	defer ds.lock.Unlock()
	key := ServiceNetworkKey{Name: name, AccountID: account}

	mesh, ok := ds.serviceNetworks[key]

	if !ok {
		glog.V(6).Infof("GetServiceNetworkStatus NOT found name:[%s], account[%s]\n", name, account)
		return ServiceNetwork{}, errors.New(DATASTORE_SERVICE_NETWORK_NOT_EXIST)
	}

	return *mesh, nil

}

func (ds *LatticeDataStore) AddLatticeService(name string, namespace string, arn string, id string, dns string) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	glog.V(6).Infof("AddLatticeService API, name[%s] namespace[%s] arn[%s] ds [%v]\n", name, namespace, arn, ds)

	serviceKey := LatticeServiceKey{
		Name:      name,
		Namespace: namespace,
	}

	_, ok := ds.latticeServices[serviceKey]

	if ok {
		glog.V(6).Infof("UpdateLatticeService: name[%s] namespace[%s] arn[%s]\n", name, namespace, arn)
	}

	ds.latticeServices[serviceKey] = &LatticeService{
		LatticeServiceKey: serviceKey,
		ARN:               arn,
		ID:                id,
		DNS:               dns,
	}

	return nil
}

func (ds *LatticeDataStore) DelLatticeService(name string, namespace string) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	glog.V(6).Infof("DelLatticeService API, name[%s] namespace[%s] \n", name, namespace)

	serviceKey := LatticeServiceKey{
		Name:      name,
		Namespace: namespace,
	}

	_, ok := ds.latticeServices[serviceKey]

	if !ok {
		glog.V(6).Infof("Deleting unknown service: name[%s] namespace[%s] \n", name, namespace)
		return errors.New(DATASTORE_SERVICE_NOT_EXIST)
	}
	delete(ds.latticeServices, serviceKey)
	return nil

}

func (ds *LatticeDataStore) GetLatticeService(name string, namespace string) (LatticeService, error) {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	glog.V(6).Infof("GetLatticeService API, name[%s] namespace[%s] ds [%v]\n", name, namespace, ds)
	var svc = LatticeService{}

	serviceKey := LatticeServiceKey{
		Name:      name,
		Namespace: namespace,
	}

	_, ok := ds.latticeServices[serviceKey]

	if ok {
		svc.LatticeServiceKey = serviceKey
		svc.ARN = ds.latticeServices[serviceKey].ARN
		svc.ID = ds.latticeServices[serviceKey].ID
		svc.DNS = ds.latticeServices[serviceKey].DNS
		return svc, nil
	} else {
		return svc, errors.New(DATASTORE_SERVICE_NOT_EXIST)
	}

}

// the max tg name length is 127
func TargetGroupName(name string, namespace string) string {
	return fmt.Sprintf("k8s-%0.20s-%0.20s", name, namespace)
}

func TargetGroupLongName(k8sName string, routeName string, vpcid string) string {
	return fmt.Sprintf("k8s-%0.40s-%0.20s-%0.20s", k8sName, routeName, vpcid)
}

// TODO , find out a good name
// AWSserviceName,  or VSNServiceName or LatticeServiceName
// the max name length is 40
func AWSServiceName(name string, namespace string) string {
	return fmt.Sprintf("%0.20s-%0.18s", name, namespace)
}

func (ds *LatticeDataStore) AddTargetGroup(name string, vpc string, arn string, tgID string,
	isServiceImport bool, routeName string) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	glog.V(6).Infof("AddTargetGroup, name[%s]  isServiceImport[%v] vpc[%s] arn[%s] ds [%v]\n",
		name, isServiceImport, vpc, arn, ds)

	targetGroupKey := TargetGroupKey{
		Name:            name,
		RouteName:       routeName,
		IsServiceImport: isServiceImport,
	}

	tg, ok := ds.targetGroups[targetGroupKey]

	if ok {
		glog.V(6).Infof("UpdateTargetGroup, name[%s] vpc[%s] arn[%s]\n",
			name, vpc, arn)
		if arn != "" {
			tg.ARN = arn
		}
		tg.VpcID = vpc

		if tgID != "" {
			tg.ID = tgID
		}

	} else {

		ds.targetGroups[targetGroupKey] = &TargetGroup{
			TargetGroupKey:  targetGroupKey,
			ARN:             arn,
			VpcID:           vpc,
			ID:              tgID,
			ByServiceExport: false,
			ByBackendRef:    false,
		}
		tg, _ = ds.targetGroups[targetGroupKey]
	}

	return nil
}

func (ds *LatticeDataStore) SetTargetGroupByServiceExport(name string, isServiceImport bool, byServiceExport bool) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	targetGroupKey := TargetGroupKey{
		Name:            name,
		IsServiceImport: isServiceImport,
	}

	tg, ok := ds.targetGroups[targetGroupKey]

	if ok {
		tg.ByServiceExport = byServiceExport
		return nil
	} else {
		return errors.New(DATASTORE_TG_NOT_EXIST)
	}

}

func (ds *LatticeDataStore) SetTargetGroupByBackendRef(name string, routeName string, isServiceImport bool, byBackendRef bool) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	targetGroupKey := TargetGroupKey{
		Name:            name,
		RouteName:       routeName,
		IsServiceImport: isServiceImport,
	}

	tg, ok := ds.targetGroups[targetGroupKey]

	if ok {
		tg.ByBackendRef = byBackendRef
		return nil
	} else {
		return errors.New(DATASTORE_TG_NOT_EXIST)
	}

}

func (ds *LatticeDataStore) DelTargetGroup(name string, routeName string, isServiceImport bool) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	glog.V(6).Infof("DelTargetGroup, name[%s] isServiceImport[%v]\n",
		name, isServiceImport)

	targetGroupKey := TargetGroupKey{
		Name:            name,
		RouteName:       routeName,
		IsServiceImport: isServiceImport,
	}

	_, ok := ds.targetGroups[targetGroupKey]

	if !ok {
		glog.V(6).Infof("Deleting unknown TargetGroup, name[%s]  isServiceImport[%v]\n",
			name, isServiceImport)
		return errors.New(DATASTORE_TG_NOT_EXIST)
	}

	delete(ds.targetGroups, targetGroupKey)
	return nil

}

func (ds *LatticeDataStore) GetTargetGroup(name string, routeName string, isServiceImport bool) (TargetGroup, error) {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	targetGroupKey := TargetGroupKey{
		Name:            name,
		RouteName:       routeName,
		IsServiceImport: isServiceImport,
	}

	tg, ok := ds.targetGroups[targetGroupKey]

	if !ok {
		//glog.V(6).Infof("GetTargetGroup name[%s]  isServiceImport[%v] ds [%v]does NOT exist\n",
		//	name, isServiceImport, ds)
		return TargetGroup{}, errors.New(DATASTORE_TG_NOT_EXIST)
	}

	//glog.V(6).Infof("GetTargetGroup :%v\n", *tg)

	return *tg, nil

}

func (ds *LatticeDataStore) GetTargetGroupsByTG(name string) []TargetGroup {
	tgs := make([]TargetGroup, 0)

	for _, tg := range ds.targetGroups {
		if tg.TargetGroupKey.Name == name && !tg.TargetGroupKey.IsServiceImport {
			tgs = append(tgs, *tg)

		}
	}

	return tgs
}

func (ds *LatticeDataStore) UpdateTargetsForTargetGroup(name string, routeName string, targetList []Target) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	targetGroupKey := TargetGroupKey{
		Name:            name,
		RouteName:       routeName,
		IsServiceImport: false, // only update target list in the local cluster
	}

	tg, ok := ds.targetGroups[targetGroupKey]

	if !ok {
		glog.V(6).Infof("UpdateTargetGroup name does NOT exist[%s] ds [%v]\n",
			name, ds)
		return errors.New(DATASTORE_TG_NOT_EXIST)
	}

	tg.EndPoints = make([]Target, len(targetList))
	copy(tg.EndPoints, targetList)

	glog.V(6).Infof("Success UpdateTarget Group name[%s]  targetIPList {%v}\n",
		name, tg.EndPoints)

	return nil
}

func (ds *LatticeDataStore) AddListener(name string, namespace string, port int64, protocol string, arn string, id string) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	listenerKey := ListenerKey{
		Name:      name,
		Namespace: namespace,
		Port:      port,
		Protocol:  protocol,
	}

	ds.listeners[listenerKey] = &Listener{
		Key: listenerKey,
		ARN: arn,
		ID:  id,
	}

	glog.V(6).Infof("AddListener :%v, ARN %s, ID %s\n", *ds.listeners[listenerKey], arn, id)

	return nil
}

func (ds *LatticeDataStore) DelListener(name string, namespace string, port int64, protocol string) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	listenerKey := ListenerKey{
		Name:      name,
		Namespace: namespace,
		Port:      port,
		Protocol:  protocol,
	}

	glog.V(6).Infof("DataStore: deleting listener %v \n", listenerKey)
	_, ok := ds.listeners[listenerKey]

	if !ok {
		glog.V(6).Infof("Deleting unknown listener %v \n", listenerKey)
		return errors.New(DATASTORE_LISTENER_NOT_EXIST)
	}

	delete(ds.listeners, listenerKey)

	return nil

}

func (ds *LatticeDataStore) GetlListener(name string, namespace string, port int64, protocol string) (Listener, error) {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	listenerKey := ListenerKey{
		Name:      name,
		Namespace: namespace,
		Port:      port,
		Protocol:  protocol,
	}

	listener, ok := ds.listeners[listenerKey]

	if !ok {
		glog.V(6).Infof("Deleting unknown listener %v \n", listenerKey)
		return Listener{}, errors.New(DATASTORE_LISTENER_NOT_EXIST)
	}

	return *listener, nil
}

func (ds *LatticeDataStore) GetAllListeners(name string, namespace string) ([]*Listener, error) {
	var listenerList []*Listener

	ds.lock.Lock()
	defer ds.lock.Unlock()

	for _, lis := range ds.listeners {

		if lis.Key.Name == name &&
			lis.Key.Namespace == namespace {
			listener := Listener{
				Key: ListenerKey{
					Name:      name,
					Namespace: namespace,
					Port:      lis.Key.Port,
					Protocol:  lis.Key.Protocol,
				},
				ID: lis.ID,
			}
			listenerList = append(listenerList, &listener)
		}
	}

	return listenerList, nil

}

//TODO delete
