# Report on Gateway API Conformance Testing

[Kubernetes Gateway API Conformance](https://gateway-api.sigs.k8s.io/concepts/conformance/?h=conformance)

## Summary of Test Result

| Category | Test Cases | Status | Notes |
| - | - | - | - |
| GatewayClass | [GatewayClassObservedGenerationBump](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gatewayclass-observed-generation-bump.go) | ok |
| Gateway | [GatewayObservedGenerationBump](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-observed-generation-bump.go) | ok |
| | [GatewayInvalidRouteKind](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-invalid-route-kind.go) | ok |
| | [GatewayWithAttachedRoutes](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-with-attached-routes.go) | ok |
| | | | |
| | [GatewaySecretInvalidReferenceGrants](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-invalid-reference-grant.go) | N/A | VPC Lattice supports ACM certs |
| | [GatewaySecretMissingReferenceGrant](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-missing-reference-grant.go) | N/A | VPC Lattice supports ACM certs |
| | [GatewaySecretReferenceGrantAllInNamespace](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-reference-grant-all-in-namespace.go) | N/A | VPC Lattice supports ACM Certs |
| | [GatewaySecretReferenceGrantSpecific](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-reference-grant-specific.go) | N/A | VPC Lattice supports ACM certs |
| | | | |
| HTTPRoute | [HTTPRouteCrossNamespace](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-cross-namespace.go) | ok |
| | [HTTPExactPathMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-exact-path-matching.go) | ok |
| | [HTTPRouteHeaderMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-header-matching.go) | fail | Test data exceeds Lattice limit on # of rules |
| | [HTTPRouteSimpleSameNamespace](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-simple-same-namespace.go) | ok |
| | [HTTPRouteListenerHostnameMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-listener-hostname-matching.go) | N/A | Listener hostname not supported |
| | [HTTPRouteMatchingAcrossRoutes](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-matching-across-routes.go) | N/A | Custom domain name conflict not allowed |
| | [HTTPRouteMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-matching.go) | fail | Route precedence |
| | [HTTPRouteObservedGenerationBump](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-observed-generation-bump.go) | ok |
| | [HTTPRoutePathMatchOrder](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-path-match-order.go) | fail | Test data exceeds Lattice limit on # of rules |
| | [HTTPRouteReferenceGrant](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-reference-grant.go) | N/A |
| | [HTTPRouteDisallowedKind](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-disallowed-kind.go) | N/A | Only HTTPRoute is supported |
| | [HTTPRouteInvalidNonExistentBackendRef](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-backendref-nonexistent.go) | fail | [#277](https://github.com/aws/aws-application-networking-k8s/issues/277) |
| | [HTTPRouteInvalidBackendRefUnknownKind](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-backendref-unknown-kind.go) | fail | [#277](https://github.com/aws/aws-application-networking-k8s/issues/277) |
| | [HTTPRouteInvalidCrossNamespaceBackendRef](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-cross-namespace-backend-ref.go) | fail | [#277](https://github.com/aws/aws-application-networking-k8s/issues/277) |
| | [HTTPRouteInvalidCrossNamespaceParentRef](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-cross-namespace-parent-ref.go)  | fail | [#277](https://github.com/aws/aws-application-networking-k8s/issues/277) |
| | [HTTPRouteInvalidParentRefNotMatchingListenerPort](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-parentref-not-matching-listener-port.go) | fail | [#277](https://github.com/aws/aws-application-networking-k8s/issues/277) |
| | [HTTPRouteInvalidParentRefNotMatchingSectionName](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-parentref-not-matching-section-name.go) | fail | [#277](https://github.com/aws/aws-application-networking-k8s/issues/277) |
| | | | |
| | [HTTPRouteMethodMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-method-matching.go) | fail | not supported in controller yet. [#123](https://github.com/aws/aws-application-networking-k8s/issues/123) |
| | | | |
| | [HTTPRouteHostnameIntersection](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-hostname-intersection.go) | N/A | VPC lattice only supports one custom domain |
| | HTTPRouteQueryParamMatching | N/A | Not supported by lattice |
| | HTTPRouteRedirectHostAndStatus | N/A | Not supported by lattice |
| | HTTPRouteRedirectPath | N/A | Not supported by lattice |
| | HTTPRouteRedirectPort | N/A | Not supported by lattice | 
| | HTTPRouteRedirectScheme | N/A | Not supported by lattice |
| | HTTPRouteRequestHeaderModifier | N/A | Not supported by lattice |
| | HTTPRouteResponseHeaderModifier | N/A | Not supported by lattice |
| | HTTPRouteRewriteHost | N/A | Not supported by lattice |
| | HTTPRouteRewritePath | N/A | Not supported by lattice |

## Running Gateway API Conformance

### Running controller from cloud desktop

```
# create a gateway first in the cluster
kubectl apply -f examples/my-hotel-gateway.yaml

# run controller in following mode

REGION=us-west-2 CLUSTER_LOCAL_GATEWAY=my-hotel TARGET_GROUP_NAME_LEN_MODE="long" \
make run
```

### Run individual conformance test

Conformance tests directly send traffic, so they should run inside the VPC that the cluster is operating on.

```
go test ./conformance/ --run "TestConformance/HTTPRouteCrossNamespace$" -v -args -gateway-class amazon-vpc-lattice \
-supported-features Gateway,HTTPRoute,GatewayClassObservedGenerationBump

```






