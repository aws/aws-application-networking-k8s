package integration

import (
	"context"
	"fmt"
	"time"

	"slices"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	acm "github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	route53 "github.com/aws/aws-sdk-go-v2/service/route53"
	vpclattice "github.com/aws/aws-sdk-go-v2/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("ACM certificate discovery", Ordered, func() {

	const (
		hostedZoneName = "cert-discovery-e2e.com"
		certDnsName    = "*." + hostedZoneName
		cname          = "app." + hostedZoneName
	)

	var (
		log       = testFramework.Log.Named("cert-discovery")
		awsCfg = lo.Must(awsconfig.LoadDefaultConfig(context.TODO(), awsconfig.WithRegion(config.Region)))
		acmClient = acm.NewFromConfig(awsCfg)
		r53Client = route53.NewFromConfig(awsCfg)

		certArn       string
		hostedZoneId  string
		latticeSvcDns string
		err           error
		deployment    *appsv1.Deployment
		service       *corev1.Service
		httpRoute     *gwv1.HTTPRoute
	)

	BeforeAll(func() {
		log := log.Named("before-all")
		// create and deploy certificate to acm
		certArn, err = createCertIfNotExists(acmClient, certDnsName)
		Expect(err).To(BeNil())
		log.Infof(ctx, "created certificate: %s", certArn)

		// Wait for cert to appear in ListCertificates
		Eventually(func(g Gomega) {
			out, err := acmClient.ListCertificates(context.TODO(), &acm.ListCertificatesInput{
				CertificateStatuses: []acmtypes.CertificateStatus{acmtypes.CertificateStatusIssued},
			})
			g.Expect(err).To(BeNil())
			found := false
			for _, c := range out.CertificateSummaryList {
				if aws.ToString(c.CertificateArn) == certArn {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(), "cert not yet visible in ListCertificates")
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		log.Infof(ctx, "certificate visible in ListCertificates")

		// add HTTPS listener without manual cert ARN — controller must discover it
		addGatewayCertDiscoveryListener(cname)
		log.Infof(ctx, "added cert-discovery listener (no cert ARN) to gateway")

		// create and deploy service for traffic test
		deployment, service = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "cert-discovery-app",
			Namespace: k8snamespace,
		})

		// create http route targeting the cert-discovery listener
		httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")
		httpRoute.Spec.Hostnames = []gwv1.Hostname{cname}
		sectionName := gwv1.SectionName("cert-discovery")
		httpRoute.Spec.ParentRefs[0].SectionName = &sectionName
		testFramework.ExpectCreated(context.TODO(), deployment, service, httpRoute)

		// get lattice service dns name for route53 cname
		svc := testFramework.GetVpcLatticeService(context.TODO(), core.NewHTTPRoute(gwv1.HTTPRoute(*httpRoute)))
		latticeSvcDns = *svc.DnsEntry.DomainName
		log.Infof(ctx, "deployed lattice service, dns name: %s", latticeSvcDns)

		// create route 53 hosted zone and cname
		hz, err := createHostedZoneIfNotExists(r53Client, hostedZoneName)
		Expect(err).To(BeNil())
		hostedZoneId = *hz.Id
		log.Infof(ctx, "created route53 hosted zone, id: %s", hostedZoneId)
		err = createCnameIfNotExists(r53Client, hostedZoneId, cname, latticeSvcDns)
		Expect(err).To(BeNil())
		log.Infof(ctx, "created cname for lattice service, cname: %s, value: %s", cname, latticeSvcDns)
	})

	It("auto-discovers ACM certificate and serves HTTPS traffic", func() {
		log := log.Named("traffic-test")

		// Verify the controller attached the discovered cert to the Lattice service
		svc := testFramework.GetVpcLatticeService(context.TODO(), core.NewHTTPRoute(gwv1.HTTPRoute(*httpRoute)))
		Eventually(func(g Gomega) {
			out, err := testFramework.LatticeClient.GetService(ctx, &vpclattice.GetServiceInput{
				ServiceIdentifier: svc.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(out.CertificateArn).ToNot(BeNil())
			g.Expect(*out.CertificateArn).To(Equal(certArn))
			log.Infof(ctx, "lattice service has cert: %s", *out.CertificateArn)
		}).WithTimeout(3 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())

		// Verify HTTPS traffic works
		pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
		pod := pods[0]
		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl -k https://%s/", cname)
			log.Infof(ctx, "calling lattice service, cmd=%s, pod=%s/%s", cmd, pod.Namespace, pod.Name)
			stdout, _, err := testFramework.PodExec(pod, cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("cert-discovery-app handler pod"))
		}).Should(Succeed())
	})

	AfterAll(func() {
		log := log.Named("after-all")
		err = deleteHostedZoneRecords(r53Client, hostedZoneId)
		Expect(err).To(BeNil())
		log.Infof(ctx, "deleted hosted zone records, id: %s", hostedZoneId)

		err = deleteHostedZone(r53Client, hostedZoneId)
		Expect(err).To(BeNil())
		log.Infof(ctx, "deleted route53 hosted zone, id: %s", hostedZoneId)

		testFramework.ExpectDeletedThenNotFound(context.TODO(), httpRoute, service, deployment)

		removeGatewayCertDiscoveryListener()
		log.Infof(ctx, "removed cert-discovery listener from gateway")

		err = deleteCert(acmClient, certArn)
		Expect(err).To(BeNil())
		log.Infof(ctx, "removed cert from acm, arn: %s", certArn)
	})
})

func addGatewayCertDiscoveryListener(hostname string) {
	gw := &gwv1.Gateway{}
	testFramework.Get(context.TODO(), types.NamespacedName{
		Namespace: testGateway.Namespace,
		Name:      testGateway.Name,
	}, gw)
	tlsMode := gwv1.TLSModeTerminate
	listener := gwv1.Listener{
		Name:     "cert-discovery",
		Port:     443,
		Hostname: lo.ToPtr(gwv1.Hostname(hostname)),
		Protocol: gwv1.HTTPSProtocolType,
		TLS: &gwv1.ListenerTLSConfig{
			Mode: &tlsMode,
			// No Options — no manual certificate-arn annotation.
			// The controller must auto-discover the cert from ACM.
			CertificateRefs: []gwv1.SecretObjectReference{
				{
					Name: "dummy",
				},
			},
		},
	}
	gw.Spec.Listeners = append(gw.Spec.Listeners, listener)
	testFramework.ExpectUpdated(context.TODO(), gw)
}

func removeGatewayCertDiscoveryListener() {
	gw := &gwv1.Gateway{}
	testFramework.Get(context.TODO(), types.NamespacedName{
		Namespace: testGateway.Namespace,
		Name:      testGateway.Name,
	}, gw)
	gw.Spec.Listeners = slices.DeleteFunc(gw.Spec.Listeners,
		func(l gwv1.Listener) bool {
			return l.Name == "cert-discovery"
		})
	testFramework.ExpectUpdated(context.TODO(), gw)
}
