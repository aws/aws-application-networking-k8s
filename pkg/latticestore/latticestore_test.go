package latticestore

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TODO: with more data
func Test_dumpCurrentLatticeDataStore(t *testing.T) {
	inputDataStore := NewLatticeDataStore()

	store := dumpCurrentLatticeDataStore(inputDataStore)

	fmt.Printf("store:%v \n", store)

	assert.NotEqual(t, store, nil, "Expected store not nil")
}

func Test_GetDefaultLatticeDataStore(t *testing.T) {
	inputDataStore := NewLatticeDataStore()
	defaultDataStore := GetDefaultLatticeDataStore()

	assert.Equal(t, inputDataStore, defaultDataStore, "")
}

func Test_LatticeService(t *testing.T) {
	inputDataStore := NewLatticeDataStore()

	name := "service"
	namespace := "default"
	name1 := "service1"
	namespace1 := "ns1"
	arn := "arn"
	id := "id"
	dns := "dns-name"

	// GetLatticeService on an unknown service
	service, err := inputDataStore.GetLatticeService(name, namespace)
	fmt.Printf("error :%v\n", err)
	assert.NotNil(t, err)
	assert.Equal(t, errors.New(DATASTORE_SERVICE_NOT_EXIST), err)

	// AddLatticeService Happy path
	err = inputDataStore.AddLatticeService(name, namespace, arn, id, dns)
	assert.Nil(t, err)

	store := dumpCurrentLatticeDataStore(inputDataStore)
	fmt.Printf("store:%v \n", store)

	assert.Equal(t, len(store.LatticeServices), 1, "")

	// verify GetLatticeService ok
	service, err = inputDataStore.GetLatticeService(name, namespace)
	assert.Nil(t, err)
	assert.Equal(t, name, service.LatticeServiceKey.Name)
	assert.Equal(t, namespace, service.LatticeServiceKey.Namespace)
	assert.Equal(t, arn, service.ARN)
	assert.Equal(t, id, service.ID)

	// add same service again, no impact
	err = inputDataStore.AddLatticeService(name, namespace, arn, id, dns)
	assert.Nil(t, err)

	store = dumpCurrentLatticeDataStore(inputDataStore)
	fmt.Printf("store:%v \n", store)
	assert.Equal(t, 1, len(store.LatticeServices), "")

	// add another service
	err = inputDataStore.AddLatticeService(name1, namespace1, arn, id, dns)
	assert.Nil(t, err)

	// verify 2 service added
	store = dumpCurrentLatticeDataStore(inputDataStore)
	fmt.Printf("store:%v \n", store)

	assert.Equal(t, 2, len(store.LatticeServices), "")

	// delete 2nd service
	err = inputDataStore.DelLatticeService(name1, namespace1)
	assert.Nil(t, err)

	// delete unknown service, 2nd delete should failed
	err = inputDataStore.DelLatticeService(name1, namespace1)
	assert.Equal(t, errors.New(DATASTORE_SERVICE_NOT_EXIST), err)

	store = dumpCurrentLatticeDataStore(inputDataStore)
	fmt.Printf("store:%v \n", store)
	assert.Equal(t, 1, len(store.LatticeServices), "")

}

func Test_TargetGroup(t *testing.T) {
	inputDataStore := NewLatticeDataStore()

	name := "tg1"
	name1 := "tg2"
	unknowntg := "unknowntg"
	namespace := "default"
	namespace1 := "ns"
	vpc := "vpc-123"
	tgID := "1234"
	arn := "arn"
	serviceImport := true
	//byBackendRef := true
	//byServiceExport := false
	K8SService := false
	routeName := "httproute"

	// GetTargetGroup on an unknown TG
	tgName := TargetGroupName(name, namespace)
	_, err := inputDataStore.GetTargetGroup(tgName, routeName, serviceImport)
	assert.Equal(t, errors.New(DATASTORE_TG_NOT_EXIST), err)

	// Happy Path for a serviceImport
	err = inputDataStore.AddTargetGroup(tgName, vpc, arn, tgID, serviceImport, "")
	assert.Nil(t, err)

	store := dumpCurrentLatticeDataStore(inputDataStore)
	fmt.Printf("store:%v \n", store)

	assert.Equal(t, 1, len(store.TargetGroups), "")

	// Verify GetTargetGroup return TG just added
	tg, err := inputDataStore.GetTargetGroup(tgName, "", serviceImport)
	assert.Nil(t, err)
	assert.Equal(t, vpc, tg.VpcID)
	assert.Equal(t, arn, tg.ARN)
	assert.Equal(t, tgID, tg.ID)
	// by default
	assert.Equal(t, false, tg.ByBackendRef)
	assert.Equal(t, false, tg.ByServiceExport)

	inputDataStore.SetTargetGroupByBackendRef(tgName, "", serviceImport, true)
	tg, err = inputDataStore.GetTargetGroup(tgName, "", serviceImport)
	assert.Nil(t, err)
	assert.Equal(t, true, tg.ByBackendRef)

	inputDataStore.SetTargetGroupByServiceExport(tgName, serviceImport, true)
	tg, err = inputDataStore.GetTargetGroup(tgName, "", serviceImport)
	assert.Nil(t, err)
	assert.Equal(t, true, tg.ByServiceExport)

	// Verify GetTargetGroup will fail if it is K8SService
	_, err = inputDataStore.GetTargetGroup(tgName, "", K8SService)
	assert.Equal(t, errors.New(DATASTORE_TG_NOT_EXIST), err)

	// Add same TG again, no impact
	err = inputDataStore.AddTargetGroup(tgName, vpc, arn, tgID, serviceImport, "")
	assert.Nil(t, err)

	store = dumpCurrentLatticeDataStore(inputDataStore)
	fmt.Printf("store:%v \n", store)
	assert.Equal(t, 1, len(store.TargetGroups), "")

	// add 2nd TG
	tgName1 := TargetGroupName(name1, namespace1)
	err = inputDataStore.AddTargetGroup(tgName1, vpc, arn, tgID, K8SService, routeName)
	assert.Nil(t, err)

	store = dumpCurrentLatticeDataStore(inputDataStore)
	fmt.Printf("store:%v \n", store)
	assert.Equal(t, 2, len(store.TargetGroups), "")

	// add targets
	var targets []Target
	targets = append(targets, Target{TargetIP: "1.1.1.1", TargetPort: 10})
	targets = append(targets, Target{TargetIP: "2.2.2.2", TargetPort: 20})
	unknownTGName := TargetGroupName(unknowntg, namespace)
	// Update an unknown TG
	err = inputDataStore.UpdateTargetsForTargetGroup(unknownTGName, routeName, targets)
	assert.Equal(t, errors.New(DATASTORE_TG_NOT_EXIST), err)

	// update with the correct name
	err = inputDataStore.UpdateTargetsForTargetGroup(tgName1, routeName, targets)
	assert.Nil(t, err)

	store = dumpCurrentLatticeDataStore(inputDataStore)
	fmt.Printf("store:%v \n", store)

	// Update targets
	targets = append(targets, Target{TargetIP: "3.3.3.3", TargetPort: 30})
	err = inputDataStore.UpdateTargetsForTargetGroup(tgName1, routeName, targets)
	assert.Nil(t, err)

	store = dumpCurrentLatticeDataStore(inputDataStore)
	fmt.Printf("store:%v \n", store)

	// delete 2nd TG
	err = inputDataStore.DelTargetGroup(tgName1, routeName, K8SService)
	assert.Nil(t, err)

	_, err = inputDataStore.GetTargetGroup(tgName1, routeName, K8SService)
	assert.Equal(t, errors.New(DATASTORE_TG_NOT_EXIST), err)

	// delete twice
	err = inputDataStore.DelTargetGroup(tgName1, routeName, K8SService)
	assert.Equal(t, errors.New(DATASTORE_TG_NOT_EXIST), err)

}

func Test_Listener(t *testing.T) {

	ds := NewLatticeDataStore()

	listenerName1 := "listener1"
	listenerNamespace1 := "default"
	arn1 := "arn1"
	id1 := "id1"
	port1 := 80
	protocol1 := "http"

	listenerName2 := "listener2"
	listenerNamespace2 := "space2"
	port2 := 443
	protocol2 := "https"
	arn2 := "arn2"
	id2 := "id2"

	err := ds.AddListener(listenerName1, listenerNamespace1, int64(port1), protocol1, arn1, id1)
	assert.NoError(t, err)

	err = ds.AddListener(listenerName1, listenerNamespace1, int64(port1), protocol1, arn1, id1)
	assert.NoError(t, err)

	err = ds.AddListener(listenerName1, listenerNamespace1, int64(port2), protocol2, arn2, id2)
	assert.NoError(t, err)

	err = ds.AddListener(listenerName2, listenerNamespace2, int64(port1), protocol2, arn1, id1)
	assert.NoError(t, err)

	err = ds.AddListener(listenerName2, listenerNamespace2, int64(port1), protocol1, arn1, id1)
	assert.NoError(t, err)

	err = ds.AddListener(listenerName2, listenerNamespace2, int64(port2), protocol2, arn2, id2)
	assert.NoError(t, err)

	listenerList, err := ds.GetAllListeners(listenerName1, listenerNamespace1)
	assert.NoError(t, err)
	assert.Equal(t, len(listenerList), 2)

	listener, err := ds.GetlListener(listenerName1, listenerNamespace1, int64(port1), protocol1)
	assert.NoError(t, err)
	assert.Equal(t, listener.Key.Name, listenerName1)
	assert.Equal(t, listener.Key.Namespace, listenerNamespace1)
	assert.Equal(t, listener.Key.Port, int64(port1))
	assert.Equal(t, listener.ARN, arn1)
	assert.Equal(t, listener.ID, id1)

	err = ds.DelListener(listenerName1, listenerNamespace1, int64(port1), protocol1)
	assert.NoError(t, err)

	_, err = ds.GetlListener(listenerName1, listenerNamespace1, int64(port1), protocol1)
	assert.Error(t, err)
}
