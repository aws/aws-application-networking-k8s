package integration

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/external-dns/endpoint"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("DNSEndpoint opt-out", Ordered, func() {
	var (
		deployment *appsv1.Deployment
		svc        *v1.Service
		httpRoute  *gwv1.HTTPRoute
		dnsKey     types.NamespacedName
	)

	expectDnsEndpoint := func(exists bool) {
		Eventually(func(g Gomega) {
			err := testFramework.Get(ctx, dnsKey, &endpoint.DNSEndpoint{})
			if exists {
				g.Expect(err).To(BeNil())
			} else {
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}
		}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}

	setAnnotation := func(value string) {
		Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), httpRoute)).To(Succeed())
		if httpRoute.Annotations == nil {
			httpRoute.Annotations = map[string]string{}
		}
		if value == "" {
			delete(httpRoute.Annotations, k8s.DnsEndpointAnnotation)
		} else {
			httpRoute.Annotations[k8s.DnsEndpointAnnotation] = value
		}
		testFramework.ExpectUpdated(ctx, httpRoute)
	}

	BeforeAll(func() {
		deployment, svc = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "dns-opt-out-update",
			Namespace: k8snamespace,
		})
		httpRoute = testFramework.NewHttpRoute(testGateway, svc, "Service")
		httpRoute.Spec.Hostnames = []gwv1.Hostname{"dns-opt-out-update.example.internal"}
		httpRoute.Spec.ParentRefs[0].SectionName = lo.ToPtr(gwv1.SectionName("http"))
		dnsKey = types.NamespacedName{Namespace: httpRoute.Namespace, Name: httpRoute.Name + "-dns"}
		testFramework.ExpectCreated(ctx, deployment, svc, httpRoute)
	})

	It("creates the DNSEndpoint by default", func() {
		expectDnsEndpoint(true)
	})

	It("deletes the DNSEndpoint when the annotation is set to disabled", func() {
		setAnnotation(k8s.DnsEndpointDisabledValue)
		expectDnsEndpoint(false)
		Consistently(func(g Gomega) {
			err := testFramework.Get(ctx, dnsKey, &endpoint.DNSEndpoint{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(30 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
	})

	It("recreates the DNSEndpoint when the annotation is removed", func() {
		setAnnotation("")
		expectDnsEndpoint(true)
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx, httpRoute, deployment, svc)
	})
})

var _ = Describe("DNSEndpoint opt-out on create", Ordered, func() {
	var (
		deployment *appsv1.Deployment
		svc        *v1.Service
		httpRoute  *gwv1.HTTPRoute
	)

	It("does not create the DNSEndpoint when the route is created with the annotation", func() {
		deployment, svc = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "dns-opt-out-create",
			Namespace: k8snamespace,
		})
		httpRoute = testFramework.NewHttpRoute(testGateway, svc, "Service")
		httpRoute.Spec.Hostnames = []gwv1.Hostname{"dns-opt-out-create.example.internal"}
		httpRoute.Spec.ParentRefs[0].SectionName = lo.ToPtr(gwv1.SectionName("http"))
		httpRoute.Annotations = map[string]string{k8s.DnsEndpointAnnotation: k8s.DnsEndpointDisabledValue}
		testFramework.ExpectCreated(ctx, deployment, svc, httpRoute)

		testFramework.GetVpcLatticeServiceDns(httpRoute.Name, httpRoute.Namespace)

		dnsKey := types.NamespacedName{Namespace: httpRoute.Namespace, Name: httpRoute.Name + "-dns"}
		Consistently(func(g Gomega) {
			err := testFramework.Get(ctx, dnsKey, &endpoint.DNSEndpoint{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(30 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx, httpRoute, deployment, svc)
	})
})
