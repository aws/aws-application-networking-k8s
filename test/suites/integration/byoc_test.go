package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/route53"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Bring your own certificate (BYOC)", Ordered, func() {

	const (
		hostedZoneName = "e2e-test.com"
		certDnsName    = "*." + hostedZoneName
		cname          = "byoc." + hostedZoneName
	)

	var (
		log       = testFramework.Log.Named("byoc")
		awsCfg    = aws.NewConfig().WithRegion(config.Region)
		sess, _   = session.NewSession(awsCfg)
		acmClient = acm.New(sess, awsCfg)
		r53Client = route53.New(sess)

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

		// add new certificate to gateway spec
		addGatewayBYOCListener(cname, certArn)
		log.Infof(ctx, "added listener with cert to gateway")

		// create and deploy service for traffic test
		deployment, service = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "byoc-app",
			Namespace: k8snamespace,
		})

		// create http route with custom certificate setting
		httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")
		httpRoute.Spec.Hostnames = []gwv1.Hostname{cname}
		sectionName := gwv1.SectionName("byoc")
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

	It("same pod https traffic test", func() {
		log := log.Named("traffic test")
		pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
		pod := pods[0]
		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl -v -k https://%s/", cname)
			log.Infof(ctx, "calling lattice service, cmd=%s, pod=%s/%s", cmd, pod.Namespace, pod.Name)
			stdout, stderr, err := testFramework.PodExec(pod, cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("byoc-app handler pod"))
			g.Expect(stderr).To(ContainSubstring("issuer: O=byoc-e2e-test"))
		}).Should(Succeed())
	})

	AfterAll(func() {
		log := log.Named("after-all")
		err = deleteHostedZoneRecords(r53Client, hostedZoneId)
		Expect(err).To(BeNil())
		log.Infof(ctx, "deleted hosted zone records for, id: %s", hostedZoneId)

		err = deleteHostedZone(r53Client, hostedZoneId)
		Expect(err).To(BeNil())
		log.Infof(ctx, "deleted route53 hosted zone, id: %s", hostedZoneId)

		testFramework.ExpectDeletedThenNotFound(context.TODO(), httpRoute, service, deployment)

		removeGatewayBYOCListener()
		log.Infof(ctx, "removed listener with custom cert from gateway")

		err = deleteCert(acmClient, certArn)
		Expect(err).To(BeNil())
		log.Infof(ctx, "removed custom cert from acm, arn: %s", certArn)
	})
})

func createCertIfNotExists(client *acm.ACM, dnsName string) (string, error) {
	arn, err := findCert(client, dnsName)
	if err != nil {
		return "", err
	}
	if arn != "" {
		return arn, nil
	}
	arn, err = createCert(client, dnsName)
	if err != nil {
		return "", err
	}
	return arn, nil
}

func createCert(client *acm.ACM, dnsName string) (string, error) {
	cert, priv, err := genCert(dnsName)
	if err != nil {
		return "", err
	}
	arn, err := uploadCert(client, cert, priv)
	if err != nil {
		return "", err
	}
	return arn, nil
}
func genCert(dnsName string) ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			Organization: []string{"byoc-e2e-test"},
		},
		DNSNames:              []string{dnsName},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 7),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	derCert, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}
	pemCert, err := derToPem(derCert, "CERTIFICATE")
	if err != nil {
		return nil, nil, err
	}
	derPriv, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	pemPriv, err := derToPem(derPriv, "PRIVATE KEY")
	if err != nil {
		return nil, nil, err
	}
	return pemCert, pemPriv, nil
}

func derToPem(der []byte, block string) ([]byte, error) {
	var buf bytes.Buffer
	err := pem.Encode(&buf, &pem.Block{
		Type:  block,
		Bytes: der,
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func findCert(client *acm.ACM, dnsName string) (string, error) {
	out, err := client.ListCertificates(&acm.ListCertificatesInput{
		CertificateStatuses: []*string{aws.String(acm.CertificateStatusIssued)},
	})
	if err != nil {
		return "", err
	}
	for _, cert := range out.CertificateSummaryList {
		if *cert.DomainName == dnsName {
			return *cert.CertificateArn, nil
		}
	}
	return "", nil
}

func uploadCert(client *acm.ACM, cert []byte, priv []byte) (string, error) {
	out, err := client.ImportCertificate(&acm.ImportCertificateInput{
		Certificate: cert,
		PrivateKey:  priv,
	})
	if err != nil {
		return "", err
	}
	return *out.CertificateArn, nil
}

func deleteCert(client *acm.ACM, arn string) error {
	_, err := client.DeleteCertificate(&acm.DeleteCertificateInput{
		CertificateArn: &arn,
	})
	return err
}

func addGatewayBYOCListener(cname, certArn string) {
	gw := &gwv1.Gateway{}
	testFramework.Get(context.TODO(), types.NamespacedName{
		Namespace: testGateway.Namespace,
		Name:      testGateway.Name,
	}, gw)
	tlsMode := gwv1.TLSModeTerminate
	byocListener := gwv1.Listener{
		Name:     "byoc",
		Port:     443,
		Hostname: lo.ToPtr(gwv1.Hostname(cname)),
		Protocol: gwv1.HTTPSProtocolType,
		TLS: &gwv1.GatewayTLSConfig{
			Mode: &tlsMode,
			Options: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
				"application-networking.k8s.aws/certificate-arn": gwv1.AnnotationValue(certArn),
			},
			CertificateRefs: []gwv1.SecretObjectReference{
				{
					Name: "dummy",
				},
			},
		},
	}
	gw.Spec.Listeners = append(gw.Spec.Listeners, byocListener)
	testFramework.ExpectUpdated(context.TODO(), gw)
}

func removeGatewayBYOCListener() {
	gw := &gwv1.Gateway{}
	testFramework.Get(context.TODO(), types.NamespacedName{
		Namespace: testGateway.Namespace,
		Name:      testGateway.Name,
	}, gw)
	gw.Spec.Listeners = slices.DeleteFunc(gw.Spec.Listeners,
		func(l gwv1.Listener) bool {
			return l.Name == "byoc"
		})
	testFramework.ExpectUpdated(context.TODO(), gw)
}

func createHostedZoneIfNotExists(client *route53.Route53, hostedZoneName string) (*route53.HostedZone, error) {
	hostedZone, err := findHostedZone(client, hostedZoneName)
	if err != nil {
		return nil, err
	}
	if hostedZone != nil {
		return hostedZone, nil
	}
	hostedZone, err = createHostedZone(client, hostedZoneName)
	if err != nil {
		return nil, err
	}
	return hostedZone, nil
}

func findHostedZone(client *route53.Route53, name string) (*route53.HostedZone, error) {
	out, err := client.ListHostedZonesByName(&route53.ListHostedZonesByNameInput{DNSName: &name})
	if err != nil {
		return nil, err
	}
	if len(out.HostedZones) == 0 {
		return nil, nil
	}
	hz := out.HostedZones[0]
	if aws.StringValue(hz.Name) != name {
		return nil, nil
	}
	return hz, nil
}

func createHostedZone(client *route53.Route53, name string) (*route53.HostedZone, error) {
	out, err := client.CreateHostedZone(&route53.CreateHostedZoneInput{
		CallerReference: aws.String(fmt.Sprintf("%s-%d", name, time.Now().Unix())),
		HostedZoneConfig: &route53.HostedZoneConfig{
			Comment:     aws.String("eks byoc test"),
			PrivateZone: aws.Bool(true),
		},
		VPC:  &route53.VPC{VPCId: &config.VpcID, VPCRegion: &config.Region},
		Name: &name,
	})
	if err != nil {
		return nil, err
	}
	err = client.WaitUntilResourceRecordSetsChanged(&route53.GetChangeInput{Id: out.ChangeInfo.Id})
	if err != nil {
		return nil, err
	}
	return out.HostedZone, nil
}

func createCnameIfNotExists(client *route53.Route53, hostedZoneId, cname, cvalue string) error {
	exists, err := cnameExists(client, hostedZoneId, cname)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	err = createCname(client, hostedZoneId, cname, cvalue)
	if err != nil {
		return err
	}
	return nil
}

func createCname(client *route53.Route53, hostedZoneId, cname string, cvalue string) error {
	out, err := client.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(route53.ChangeActionCreate),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name:            &cname,
						ResourceRecords: []*route53.ResourceRecord{{Value: &cvalue}},
						TTL:             aws.Int64(300),
						Type:            aws.String(route53.RRTypeCname),
					},
				},
			},
			Comment: aws.String("create cname for byoc"),
		},
		HostedZoneId: &hostedZoneId,
	})
	if err != nil {
		return err
	}
	err = client.WaitUntilResourceRecordSetsChanged(&route53.GetChangeInput{Id: out.ChangeInfo.Id})
	if err != nil {
		return err
	}
	return nil
}

func cnameExists(client *route53.Route53, hostedZoneId, cname string) (bool, error) {
	rrs, err := client.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId: &hostedZoneId,
	})
	if err != nil {
		return false, err
	}
	for _, rec := range rrs.ResourceRecordSets {
		if *rec.Name == cname+"." && *rec.Type == route53.RRTypeCname {
			return true, nil
		}
	}
	return false, nil
}

func deleteHostedZoneRecords(client *route53.Route53, hostedZoneId string) error {
	rrs, err := client.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId: &hostedZoneId,
	})
	changes := []*route53.Change{}
	for _, rec := range rrs.ResourceRecordSets {
		if *rec.Type == route53.RRTypeNs || *rec.Type == route53.RRTypeSoa {
			continue
		}
		changes = append(changes, &route53.Change{
			Action:            aws.String(route53.ChangeActionDelete),
			ResourceRecordSet: rec,
		})
	}
	out, err := client.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: changes,
			Comment: aws.String("cleanup byoc test"),
		},
		HostedZoneId: &hostedZoneId,
	})
	err = client.WaitUntilResourceRecordSetsChanged(&route53.GetChangeInput{Id: out.ChangeInfo.Id})
	if err != nil {
		return err
	}
	return nil
}

func deleteHostedZone(client *route53.Route53, hostedZoneId string) error {
	out, err := client.DeleteHostedZone(&route53.DeleteHostedZoneInput{
		Id: &hostedZoneId,
	})
	if err != nil {
		return err
	}
	err = client.WaitUntilResourceRecordSetsChanged(&route53.GetChangeInput{Id: out.ChangeInfo.Id})
	if err != nil {
		return err
	}
	return nil
}
