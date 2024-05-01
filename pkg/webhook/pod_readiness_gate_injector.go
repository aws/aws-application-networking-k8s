package webhook

import (
	"context"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	k8sutils "github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	PodReadinessGateConditionType = "application-networking.k8s.aws/pod-readiness-gate"
)

func NewPodReadinessGateInjector(k8sClient client.Client, log gwlog.Logger) *PodReadinessGateInjector {
	return &PodReadinessGateInjector{
		k8sClient: k8sClient,
		log:       log,
	}
}

type PodReadinessGateInjector struct {
	k8sClient client.Client
	log       gwlog.Logger
}

func (m *PodReadinessGateInjector) MutateCreate(ctx context.Context, pod *corev1.Pod) error {
	pct := corev1.PodConditionType(PodReadinessGateConditionType)
	m.log.Debugf("Webhook invoked for pod %s/%s", pod.Name, pod.Namespace)

	found := false
	for _, rg := range pod.Spec.ReadinessGates {
		if rg.ConditionType == pct {
			found = true
			break
		}
	}
	if !found {
		requiresGate, err := m.requiresReadinessGate(ctx, pod)
		if err != nil {
			return err
		}
		if requiresGate {
			pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, corev1.PodReadinessGate{
				ConditionType: pct,
			})
		}
	}
	return nil
}

// checks if the pod requires a readiness gate
// mostly debug logs to reduce noise, intended to be tolerant of most failures
func (m *PodReadinessGateInjector) requiresReadinessGate(ctx context.Context, pod *corev1.Pod) (bool, error) {
	// fetch all services in the namespace, see if their selector matches the pod
	svcList := &corev1.ServiceList{}
	if err := m.k8sClient.List(ctx, svcList, client.InNamespace(pod.Namespace)); err != nil {
		return false, errors.Wrap(err, "unable to determine readiness gate requirement")
	}

	svcMatches := m.servicesForPod(pod, svcList)
	if len(svcMatches) == 0 {
		m.log.Debugf("No services found for pod %s/%s", pod.Name, pod.Namespace)
		return false, nil
	}

	// for each route, check if it has a backendRef to one of the services
	routes := m.listAllRoutes(ctx)
	for _, route := range routes {
		if svc := m.isPodUsedByRoute(route, svcMatches); svc != nil {
			if m.routeHasLatticeGateway(ctx, route) {
				m.log.Debugf("Pod %s/%s is used by service %s/%s and route %s/%s", pod.Name, pod.Namespace,
					svc.Name, svc.Namespace, route.Name(), route.Namespace())
				return true, nil
			}
		}
	}

	// lastly, check if there's a service export for any of the services
	for _, svc := range svcMatches {
		svcExport := &anv1alpha1.ServiceExport{}
		if err := m.k8sClient.Get(ctx, k8sutils.NamespacedName(svc), svcExport); err != nil {
			continue
		}

		m.log.Debugf("Pod %s/%s is used by service %s/%s and service export %s/%s", pod.Name, pod.Namespace,
			svc.Name, svc.Namespace, svcExport.Name, svcExport.Namespace)
		return true, nil
	}

	m.log.Debugf("Pod %s/%s does not require a readiness gate", pod.Name, pod.Namespace)
	return false, nil
}

func (m *PodReadinessGateInjector) listAllRoutes(ctx context.Context) []core.Route {
	// fetch all routes in all namespaces - backendRefs can reference other namespaces
	var routes []core.Route
	httpRouteList := &gwv1beta1.HTTPRouteList{}
	err := m.k8sClient.List(ctx, httpRouteList)
	if err != nil {
		m.log.Errorf("Error fetching HTTPRoutes: %s", err)
	}
	for _, k8sRoute := range httpRouteList.Items {
		routes = append(routes, core.NewHTTPRoute(k8sRoute))
	}
	grpcRouteList := &gwv1alpha2.GRPCRouteList{}
	err = m.k8sClient.List(ctx, grpcRouteList)
	if err != nil {
		m.log.Errorf("Error fetching GRPCRoutes: %s", err)
	}
	for _, k8sRoute := range grpcRouteList.Items {
		routes = append(routes, core.NewGRPCRoute(k8sRoute))
	}
	return routes
}

// returns a map of services that match the pod labels
func (m *PodReadinessGateInjector) servicesForPod(pod *corev1.Pod, svcList *corev1.ServiceList) map[string]*corev1.Service {
	svcMatches := make(map[string]*corev1.Service)
	podLabels := labels.Set(pod.Labels)
	for _, svc := range svcList.Items {
		svcSelector := labels.SelectorFromSet(svc.Spec.Selector)
		if svcSelector.Matches(podLabels) {
			svcMatches[svc.Name] = &svc
		}
	}
	return svcMatches
}

func (m *PodReadinessGateInjector) isPodUsedByRoute(route core.Route, svcMap map[string]*corev1.Service) *corev1.Service {
	for _, rule := range route.Spec().Rules() {
		for _, backendRef := range rule.BackendRefs() {
			// from spec: "When [Group] unspecified or empty string, core API group is inferred."
			isGroupEqual := backendRef.Group() == nil || string(*backendRef.Group()) == corev1.GroupName
			isKindEqual := backendRef.Kind() != nil && string(*backendRef.Kind()) == "Service"
			svc, isNameEqual := svcMap[string(backendRef.Name())]

			namespace := route.Namespace()
			if backendRef.Namespace() != nil {
				namespace = string(*backendRef.Namespace())
			}
			isNamespaceEqual := svc != nil && namespace == svc.GetNamespace()

			if isGroupEqual && isKindEqual && isNameEqual && isNamespaceEqual {
				return svc
			}
		}
	}
	return nil
}

func (m *PodReadinessGateInjector) routeHasLatticeGateway(ctx context.Context, route core.Route) bool {
	if len(route.Spec().ParentRefs()) == 0 {
		return false
	}

	gw := &gwv1beta1.Gateway{}

	gwNamespace := route.Namespace()
	if route.Spec().ParentRefs()[0].Namespace != nil {
		gwNamespace = string(*route.Spec().ParentRefs()[0].Namespace)
	}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		Name:      string(route.Spec().ParentRefs()[0].Name),
	}

	if err := m.k8sClient.Get(ctx, gwName, gw); err != nil {
		return false
	}

	// make sure gateway is an aws-vpc-lattice
	gwClass := &gwv1beta1.GatewayClass{}
	gwClassName := types.NamespacedName{
		Namespace: "default",
		Name:      string(gw.Spec.GatewayClassName),
	}

	if err := m.k8sClient.Get(ctx, gwClassName, gwClass); err != nil {
		return false
	}

	if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
		return true
	}

	return false
}
