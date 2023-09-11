package latticestore

import (
	"errors"
	"fmt"
	"sync"

	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
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
	log             gwlog.Logger
	lock            sync.Mutex
	latticeServices LatticeServicePool
	targetGroups    TargetGroupPool
	listeners       ListenerPool
}

type LatticeDataStoreInfo struct {
	LatticeServices map[string]LatticeService
	TargetGroups    map[string]TargetGroup
	Listeners       map[string]Listener
}

var defaultLatticeDataStore *LatticeDataStore

func NewLatticeDataStoreWithLog(log gwlog.Logger) *LatticeDataStore {
	defaultLatticeDataStore = &LatticeDataStore{
		log:             log,
		latticeServices: make(LatticeServicePool),
		targetGroups:    make(TargetGroupPool),
		listeners:       make(ListenerPool),
	}
	return defaultLatticeDataStore
}

func NewLatticeDataStore() *LatticeDataStore {
	return NewLatticeDataStoreWithLog(gwlog.FallbackLogger)
}

func dumpCurrentLatticeDataStore(ds *LatticeDataStore) *LatticeDataStoreInfo {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	var store = LatticeDataStoreInfo{
		LatticeServices: make(map[string]LatticeService),
		TargetGroups:    make(map[string]TargetGroup),
		Listeners:       make(map[string]Listener),
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

func (ds *LatticeDataStore) AddLatticeService(name string, namespace string, arn string, id string, dns string) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	serviceKey := LatticeServiceKey{
		Name:      name,
		Namespace: namespace,
	}

	_, ok := ds.latticeServices[serviceKey]
	if ok {
		ds.log.Debugf("UpdateLatticeService: name: %s namespace: %s, arn: %s", name, namespace, arn)
	} else {
		ds.log.Debugf("AddLatticeService API, name: %s, namespace: %s, arn: %s", name, namespace, arn)
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

	ds.log.Debugf("DelLatticeService API, name: %s namespace: %s", name, namespace)

	serviceKey := LatticeServiceKey{
		Name:      name,
		Namespace: namespace,
	}

	_, ok := ds.latticeServices[serviceKey]

	if !ok {
		ds.log.Debugf("Deleting unknown service: name: %s, namespace: %s", name, namespace)
		return errors.New(DATASTORE_SERVICE_NOT_EXIST)
	}
	delete(ds.latticeServices, serviceKey)
	return nil

}

func (ds *LatticeDataStore) GetLatticeService(name string, namespace string) (LatticeService, error) {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	ds.log.Debugf("GetLatticeService API, name: %s, namespace: %s", name, namespace)
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
// worst case - k8s-(50)-(50)-https-http2 (117 chars)
func TargetGroupName(name, namespace string) string {
	return fmt.Sprintf("k8s-%s-%s",
		utils.Truncate(name, 50),
		utils.Truncate(namespace, 50),
	)
}

// worst case - (70)-(20)-(21)-https-http2 (125 chars)
func TargetGroupLongName(defaultName, routeName, vpcId string) string {
	return fmt.Sprintf("%s-%s-%s",
		utils.Truncate(defaultName, 70),
		utils.Truncate(routeName, 20),
		utils.Truncate(vpcId, 21),
	)
}

func LatticeServiceName(name string, namespace string) string {
	return fmt.Sprintf("%s-%s", utils.Truncate(name, 20), utils.Truncate(namespace, 18))
}

func (ds *LatticeDataStore) AddTargetGroup(name string, vpc string, arn string, tgID string,
	isServiceImport bool, routeName string) error {
	ds.lock.Lock()
	defer ds.lock.Unlock()

	ds.log.Debugf("AddTargetGroup, name: %s, isServiceImport: %t, vpc: %s, arn: %s", name, isServiceImport, vpc, arn)

	targetGroupKey := TargetGroupKey{
		Name:            name,
		RouteName:       routeName,
		IsServiceImport: isServiceImport,
	}

	tg, ok := ds.targetGroups[targetGroupKey]

	if ok {
		ds.log.Debugf("UpdateTargetGroup, name: %s, vpc: %s, arn: %s", name, vpc, arn)
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

	ds.log.Debugf("DelTargetGroup, name: %s, isServiceImport: %t", name, isServiceImport)

	targetGroupKey := TargetGroupKey{
		Name:            name,
		RouteName:       routeName,
		IsServiceImport: isServiceImport,
	}

	_, ok := ds.targetGroups[targetGroupKey]

	if !ok {
		ds.log.Debugf("Deleting unknown TargetGroup, name: %s, isServiceImport: %t", name, isServiceImport)
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
		return TargetGroup{}, errors.New(DATASTORE_TG_NOT_EXIST)
	}

	return *tg, nil

}

func (ds *LatticeDataStore) GetTargetGroupsByName(name string) []TargetGroup {
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
		ds.log.Debugf("UpdateTargetGroup name does NOT exist: %s", name)
		return errors.New(DATASTORE_TG_NOT_EXIST)
	}

	tg.EndPoints = make([]Target, len(targetList))
	copy(tg.EndPoints, targetList)

	ds.log.Debugf("Success UpdateTarget Group name: %s,  targetIPList: %+v", name, tg.EndPoints)

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

	ds.log.Debugf("AddListener name: %s, arn: %s, id %s", name, arn, id)

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

	ds.log.Debugf("DataStore: deleting listener name: %+v", listenerKey)
	_, ok := ds.listeners[listenerKey]

	if !ok {
		ds.log.Debugf("Deleting unknown listener %+v", listenerKey)
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
		ds.log.Debugf("Deleting unknown listener %+v", listenerKey)
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
