package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func tlsBackend(name, namespace string) gwv1.BackendRef {
	return tlsBackendOfKind(name, namespace, "Service")
}

func tlsBackendOfKind(name, namespace, kind string) gwv1.BackendRef {
	return gwv1.BackendRef{
		BackendObjectReference: gwv1.BackendObjectReference{
			Name:      gwv1.ObjectName(name),
			Namespace: ptr.To(gwv1.Namespace(namespace)),
			Kind:      ptr.To(gwv1.Kind(kind)),
			Port:      ptr.To(gwv1.PortNumber(443)),
		},
	}
}

func buildTLSListeners(t *testing.T, objs []client.Object, backends ...gwv1.BackendRef) []*model.Listener {
	t.Helper()
	section := gwv1.SectionName("tls")
	gwClass := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "lattice"},
		Spec:       gwv1.GatewayClassSpec{ControllerName: config.LatticeGatewayControllerName},
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "foo"},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "lattice",
			Listeners: []gwv1.Listener{{
				Name:     section,
				Port:     443,
				Protocol: gwv1.TLSProtocolType,
				TLS:      &gwv1.ListenerTLSConfig{Mode: ptr.To(gwv1.TLSModePassthrough)},
			}},
		},
	}
	route := core.NewTLSRoute(gwv1.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "foo"},
		Spec: gwv1.TLSRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{Name: "gw", SectionName: &section}},
			},
			Rules: []gwv1.TLSRouteRule{{BackendRefs: backends}},
		},
	})
	route.Status().SetParents([]gwv1.RouteParentStatus{{
		ParentRef:  gwv1.ParentReference{Name: "gw", SectionName: &section},
		Conditions: []metav1.Condition{{Type: string(gwv1.RouteConditionAccepted), Status: metav1.ConditionTrue}},
	}})

	stack := core.NewDefaultStack(core.StackID{Namespace: "foo", Name: "route"})
	task := &latticeServiceModelBuildTask{
		log:         gwlog.FallbackLogger,
		route:       route,
		client:      rgTestClient(t, append([]client.Object{gwClass, gw}, objs...)...),
		stack:       stack,
		brTgBuilder: &dummyTgBuilder{},
	}
	assert.NoError(t, task.buildListeners(context.Background(), "svc-id"))

	var listeners []*model.Listener
	assert.NoError(t, stack.ListResources(&listeners))
	return listeners
}

func TestBuildListeners_TLSPassthroughReferenceGrant(t *testing.T) {
	setReferenceGrantServed(t, true)
	tlsGrant := grant("bar", gwv1.ReferenceGrantFrom{
		Group:     gwv1.GroupName,
		Kind:      "TLSRoute",
		Namespace: "foo",
	}, serviceTo(""))

	t.Run("all backends blocked, no listener", func(t *testing.T) {
		// With no listener in the stack, the listener synthesizer deletes the existing one,
		// so the route stops forwarding.
		assert.Empty(t, buildTLSListeners(t, nil, tlsBackend("svc", "bar")))
	})

	t.Run("one backend blocked, listener forwards to the permitted one", func(t *testing.T) {
		listeners := buildTLSListeners(t, []client.Object{tlsGrant},
			tlsBackend("svc", "bar"), tlsBackend("other", "baz"))
		assert.Len(t, listeners, 1)
		tgs := listeners[0].Spec.DefaultAction.Forward.TargetGroups
		assert.Len(t, tgs, 2)
		assert.Equal(t, "tg-0", tgs[0].StackTargetGroupId)
		assert.Equal(t, model.InvalidBackendRefTgId, tgs[1].StackTargetGroupId)
	})

	t.Run("ServiceImport backend counts as valid", func(t *testing.T) {
		// A ServiceImport resolves to an existing target group, so it has no stack target
		// group id; it must not be mistaken for an invalid backend.
		listeners := buildTLSListeners(t, nil, tlsBackendOfKind("imp", "foo", "ServiceImport"))
		assert.Len(t, listeners, 1)
		assert.NotNil(t, listeners[0].Spec.DefaultAction.Forward.TargetGroups[0].SvcImportTG)
	})
}
