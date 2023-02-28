package integration

import (
	"os"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var _ = Describe("HTTPRoute", func() {
	Context("HTTPRules", func() {
		It("httprules should support multiple paths", func() {
			deploymentV1, serviceV1 := test.HTTPApp(test.HTTPAppOptions{Name: "test-v1"})
			deploymentV2, serviceV2 := test.HTTPApp(test.HTTPAppOptions{Name: "test-v2"})
			httpRoute := test.New(&v1alpha2.HTTPRoute{
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{{
							Name:        v1alpha2.ObjectName(test.Gateway.Name),
							SectionName: lo.ToPtr(v1alpha2.SectionName("http")),
						}},
					},
					Rules: []v1alpha2.HTTPRouteRule{
						{
							BackendRefs: []v1alpha2.HTTPBackendRef{{
								BackendRef: v1alpha2.BackendRef{
									BackendObjectReference: v1alpha2.BackendObjectReference{
										Name: v1alpha2.ObjectName(serviceV1.Name),
										Kind: lo.ToPtr(v1alpha2.Kind("Service")),
									},
								},
							}},
							Matches: []v1alpha2.HTTPRouteMatch{
								{
									Path: &v1alpha2.HTTPPathMatch{
										Type:  lo.ToPtr(v1alpha2.PathMatchPathPrefix),
										Value: lo.ToPtr("/ver1"),
									},
								},
							},
						},
						{
							BackendRefs: []v1alpha2.HTTPBackendRef{{
								BackendRef: v1alpha2.BackendRef{
									BackendObjectReference: v1alpha2.BackendObjectReference{
										Name: v1alpha2.ObjectName(serviceV2.Name),
										Kind: lo.ToPtr(v1alpha2.Kind("Service")),
									},
								},
							}},
							Matches: []v1alpha2.HTTPRouteMatch{
								{
									Path: &v1alpha2.HTTPPathMatch{
										Type:  lo.ToPtr(v1alpha2.PathMatchPathPrefix),
										Value: lo.ToPtr("/ver2"),
									},
								},
							},
						},
					},
				},
			})

			// Create Kubernetes API Objects
			env.ExpectCreated(ctx,
				test.Gateway,
				httpRoute,
				serviceV1,
				deploymentV1,
				serviceV2,
				deploymentV2,
			)

			// Verify AWS API Objects

			// (reverse order to allow dependency propagation)
			targetGroupV1 := env.GetTargetGroup(ctx, serviceV1)
			targetsV1 := env.GetTargets(ctx, targetGroupV1, deploymentV1)
			Expect(*targetGroupV1.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
			Expect(*targetGroupV1.Protocol).To(Equal("HTTP"))                                         // TODO(liwenwu) should this be TCP?
			Expect(*targetGroupV1.Port).To(BeEquivalentTo(serviceV1.Spec.Ports[0].TargetPort.IntVal)) // TODO(liwenwu) should this be .Spec.Port[0].Port?
			for _, target := range targetsV1 {
				Expect(*target.Port).To(BeEquivalentTo(serviceV1.Spec.Ports[0].TargetPort.IntVal))
				Expect(*target.Status).To(BeEquivalentTo("UNUSED")) // TODO(liwenwu) should this be ACTIVE?
			}

			targetGroupV2 := env.GetTargetGroup(ctx, serviceV2)
			targetsV2 := env.GetTargets(ctx, targetGroupV2, deploymentV2)
			Expect(*targetGroupV2.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
			Expect(*targetGroupV2.Protocol).To(Equal("HTTP"))                                         // TODO(liwenwu) should this be TCP?
			Expect(*targetGroupV2.Port).To(BeEquivalentTo(serviceV2.Spec.Ports[0].TargetPort.IntVal)) // TODO(liwenwu) should this be .Spec.Port[0].Port?
			for _, target := range targetsV2 {
				Expect(*target.Port).To(BeEquivalentTo(serviceV2.Spec.Ports[0].TargetPort.IntVal))
				Expect(*target.Status).To(BeEquivalentTo("UNUSED")) // TODO(liwenwu) should this be ACTIVE?
			}

			service := env.GetService(ctx, httpRoute)
			Expect(*service.DnsEntry).To(ContainSubstring(latticestore.AWSServiceName(httpRoute.Name, httpRoute.Namespace))) // TODO(liwenwu) is there something else we should verify about service?

			serviceNetwork := env.GetServiceNetwork(ctx, test.Gateway)
			Expect(*serviceNetwork.NumberOfAssociatedServices).To(BeEquivalentTo(1))
			Expect(*serviceNetwork.NumberOfAssociatedVPCs).To(BeEquivalentTo(0)) // TODO(liwenwu) should this be 1?

			// Cleanup Kubernetes API Objects
			Expect(env.Delete(ctx, test.Gateway)).To(Succeed())
			Expect(env.Delete(ctx, httpRoute)).To(Succeed())
			Expect(env.Delete(ctx, serviceV1)).To(Succeed())
			Expect(env.Delete(ctx, deploymentV1)).To(Succeed())
			Expect(env.Delete(ctx, serviceV2)).To(Succeed())
			Expect(env.Delete(ctx, deploymentV2)).To(Succeed())
			env.ExpectToBeClean(ctx)
		})
	})
})
