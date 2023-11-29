package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/pkg/errors"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core/graph"
)

//go:generate mockgen -destination stack_mock.go -package core github.com/aws/aws-application-networking-k8s/pkg/model/core Stack

// Stack presents a resource graph, where resources can depend on each other.
type Stack interface {
	// stackID returns a unique ID for stack.
	StackID() StackID

	// Add a resource into stack.
	AddResource(res Resource) error

	// Get a resource by its id and type, pointer will be populated after call
	GetResource(id string, res Resource) error

	// Add a dependency relationship between resources.
	AddDependency(dependee Resource, depender Resource) error

	// ListResources list all resources for specific type.
	// pResourceSlice must be a pointer to a slice of resources, which will be filled.
	ListResources(pResourceSlice interface{}) error

	// TopologicalTraversal visits resources in stack in topological order.
	TopologicalTraversal(visitor ResourceVisitor) error
}

// NewDefaultStack constructs new stack.
func NewDefaultStack(stackID StackID) *defaultStack {
	return &defaultStack{
		stackID: stackID,

		resources:     make(map[graph.ResourceUID]Resource),
		resourceGraph: graph.NewDefaultResourceGraph(),
	}
}

// default implementation for stack.
type defaultStack struct {
	stackID StackID

	resources     map[graph.ResourceUID]Resource
	resourceGraph graph.ResourceGraph
}

func (s *defaultStack) StackID() StackID {
	return s.stackID
}

// Add a resource.
func (s *defaultStack) AddResource(res Resource) error {
	resUID := s.computeResourceUID(res)
	if _, ok := s.resources[resUID]; ok {
		return errors.Errorf("resource already exists, type: %v, id: %v", res.Type(), res.ID())
	}
	s.resources[resUID] = res
	s.resourceGraph.AddNode(resUID)
	return nil
}

// Get a resource from the pointer, then return the result
// will ensure the resource is of the specified type
func (s *defaultStack) GetResource(id string, res Resource) error {
	t := reflect.TypeOf(res)
	resUID := graph.ResourceUID{
		ResType: t,
		ResID:   id,
	}

	r, ok := s.resources[resUID]
	if !ok {
		return fmt.Errorf("resource %s not found", id)
	}

	// since the type makes up the key used to retrieve,
	// it will be safe to assign the value to the param
	v := reflect.ValueOf(res)
	v.Elem().Set(reflect.ValueOf(r).Elem())
	return nil
}

// Add a dependency relationship between resources.
func (s *defaultStack) AddDependency(dependee Resource, depender Resource) error {
	dependeeResUID := s.computeResourceUID(dependee)
	dependerResUID := s.computeResourceUID(depender)
	if _, ok := s.resources[dependeeResUID]; !ok {
		return errors.Errorf("dependee resource didn't exists, type: %v, id: %v", dependee.Type(), dependee.ID())
	}
	if _, ok := s.resources[dependerResUID]; !ok {
		return errors.Errorf("depender resource didn't exists, type: %v, id: %v", depender.Type(), depender.ID())
	}
	s.resourceGraph.AddEdge(dependeeResUID, dependerResUID)
	return nil
}

// ListResources list all resources for specific type.
// pResourceSlice must be a pointer to a slice of resources, which will be filled.
// note this list is ORDERED according to the order in which resources were added
// this is to increase predictability, issue reproducibility, and for ease of testing
func (s *defaultStack) ListResources(pResourceSlice interface{}) error {
	v := reflect.ValueOf(pResourceSlice)
	if v.Kind() != reflect.Ptr {
		return errors.New("pResourceSlice must be pointer to resource slice")
	}
	v = v.Elem()
	if v.Kind() != reflect.Slice {
		return errors.New("pResourceSlice must be pointer to resource slice")
	}
	resType := v.Type().Elem()
	var resForType []Resource
	for _, node := range s.resourceGraph.Nodes() {
		if node.ResType == resType {
			res := s.resources[node]
			resForType = append(resForType, res)
		}
	}
	v.Set(reflect.MakeSlice(v.Type(), len(resForType), len(resForType)))
	for i := range resForType {
		v.Index(i).Set(reflect.ValueOf(resForType[i]))
	}

	return nil
}

func (s *defaultStack) TopologicalTraversal(visitor ResourceVisitor) error {
	return graph.TopologicalTraversal(s.resourceGraph, func(uid graph.ResourceUID) error {
		return visitor.Visit(s.resources[uid])
	})
}

// computeResourceUID returns the UID for resources.
func (s *defaultStack) computeResourceUID(res Resource) graph.ResourceUID {
	return graph.ResourceUID{
		ResType: reflect.TypeOf(res),
		ResID:   res.ID(),
	}
}

func IdFromHash(res any) (string, error) {
	bytes, err := json.Marshal(res)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(bytes)
	id := fmt.Sprintf("id-%s", hex.EncodeToString(hash[:]))
	return id, nil
}
