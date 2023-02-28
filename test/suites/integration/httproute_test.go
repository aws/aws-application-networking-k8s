package integration

import (
	"os"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

var _ = Describe("HTTPRoute", func() {
	Context("HTTPRules", func() {
		It("httprules should support multiple paths", func() {
			deploymentV1, serviceV1 := test.HTTPApp(test.HTTPAppOptions{Name: "test-v1"})
			deploymentV2, serviceV2 := test.HTTPApp(test.HTTPAppOptions{Name: "test-v2"})
			httpRoute := test.New(&v1beta1.HTTPRoute{
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{{
							Name:        v1beta1.ObjectName(test.Gateway.Name),
							SectionName: lo.ToPtr(v1beta1.SectionName("http")),
						}},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							BackendRefs: []v1beta1.HTTPBackendRef{{
								BackendRef: v1beta1.BackendRef{
									BackendObjectReference: v1beta1.BackendObjectReference{
										Name: v1beta1.ObjectName(serviceV1.Name),
										Kind: lo.ToPtr(v1beta1.Kind("Service")),
									},
								},
							}},
							Matches: []v1beta1.HTTPRouteMatch{
								{
									Path: &v1beta1.HTTPPathMatch{
										Type:  lo.ToPtr(v1beta1.PathMatchPathPrefix),
										Value: lo.ToPtr("/ver1"),
									},
								},
							},
						},
						{
							BackendRefs: []v1beta1.HTTPBackendRef{{
								BackendRef: v1beta1.BackendRef{
									BackendObjectReference: v1beta1.BackendObjectReference{
										Name: v1beta1.ObjectName(serviceV2.Name),
										Kind: lo.ToPtr(v1beta1.Kind("Service")),
									},
								},
							}},
							Matches: []v1beta1.HTTPRouteMatch{
								{
									Path: &v1beta1.HTTPPathMatch{
										Type:  lo.ToPtr(v1beta1.PathMatchPathPrefix),
										Value: lo.ToPtr("/ver2"),
									},
								},
							},
						},
					},
				},
			})

			// Create Kubernetes API Objects
			framework.ExpectCreated(ctx,
				test.Gateway,
				httpRoute,
				serviceV1,
				deploymentV1,
				serviceV2,
				deploymentV2,
			)

			// Verify AWS API Objects

			// (reverse order to allow dependency propagation)
			targetGroupV1 := framework.GetTargetGroup(ctx, serviceV1)
			targetsV1 := framework.GetTargets(ctx, targetGroupV1, deploymentV1)
			Expect(*targetGroupV1.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
			Expect(*targetGroupV1.Protocol).To(Equal("HTTP"))                                         // TODO(liwenwu) should this be TCP?
			Expect(*targetGroupV1.Port).To(BeEquivalentTo(serviceV1.Spec.Ports[0].TargetPort.IntVal)) // TODO(liwenwu) should this be .Spec.Port[0].Port?
			for _, target := range targetsV1 {
				Expect(*target.Port).To(BeEquivalentTo(serviceV1.Spec.Ports[0].TargetPort.IntVal))
				Expect(*target.Status).To(Equal(vpclattice.TargetStatusInitial)) // TODO(liwenwu) should this be HEALTHY?
			}

			targetGroupV2 := framework.GetTargetGroup(ctx, serviceV2)
			targetsV2 := framework.GetTargets(ctx, targetGroupV2, deploymentV2)
			Expect(*targetGroupV2.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
			Expect(*targetGroupV2.Protocol).To(Equal("HTTP"))                                         // TODO(liwenwu) should this be TCP?
			Expect(*targetGroupV2.Port).To(BeEquivalentTo(serviceV2.Spec.Ports[0].TargetPort.IntVal)) // TODO(liwenwu) should this be .Spec.Port[0].Port?
			for _, target := range targetsV2 {
				Expect(*target.Port).To(BeEquivalentTo(serviceV2.Spec.Ports[0].TargetPort.IntVal))
				Expect(*target.Status).To(Equal(vpclattice.TargetStatusInitial)) // TODO(liwenwu) should this be HEALTHY?
			}

			service := framework.GetService(ctx, httpRoute)
			Expect(*service.DnsEntry).To(ContainSubstring(latticestore.AWSServiceName(httpRoute.Name, httpRoute.Namespace))) // TODO(liwenwu) is there something else we should verify about service?

			serviceNetwork := framework.GetServiceNetwork(ctx, test.Gateway)
			Expect(*serviceNetwork.NumberOfAssociatedServices).To(BeEquivalentTo(1))
			Expect(*serviceNetwork.NumberOfAssociatedVPCs).To(BeEquivalentTo(0)) // TODO(liwenwu) should this be 1?

			// Cleanup Kubernetes API Objects
			framework.ExpectDeleted(ctx,
				test.Gateway,
				httpRoute,
				serviceV1,
				deploymentV1,
				serviceV2,
				deploymentV2,
			)
			framework.ExpectToBeClean(ctx)
		})
	})
})
