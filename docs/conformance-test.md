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
| | [GatewaySecretInvalidReferenceGrants](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-invalid-reference-grant.go) | NA | VPC Lattice supports ACM certs |
| | [GatewaySecretMissingReferenceGrant](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-missing-reference-grant.go) | NA | same as above
| | [GatewaySecretReferenceGrantAllInNamespace](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-reference-grant-all-in-namespace.go) | NA | same as above
| | [GatewaySecretReferenceGrantSpecific](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-reference-grant-specific.go) | NA | same as above
| | | | |
| HTTPRoute | [HTTPRouteCrossNamespace](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-cross-namespace.go) | ok |
| | [HTTPExactPathMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-exact-path-matching.go) | ok |
| | [HTTPRouteHeaderMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-header-matching.go) | ok |
| | [HTTPRouteSimpleSameNamespace](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-simple-same-namespace.go) | ok |
| | [HTTPRouteListenerHostnameMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-listener-hostname-matching.go) | ok |
| | [HTTPRouteMatchingAcrossRoutes](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-matching-across-routes.go) | ok |
| | [HTTPRouteMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-matching.go) | ok |
| | [HTTPRouteObservedGenerationBump](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-observed-generation-bump.go) | ok |
| | [HTTPRoutePathMatchOrder](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-path-match-order.go) | ok |
| | [HTTPRouteReferenceGrant](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-reference-grant.go) | ok |
| | [HTTPRouteDisallowedKind](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-disallowed-kind.go) | ok |
| | [HTTPRouteInvalidNonExistentBackendRef](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-backendref-nonexistent.go) | ok |
| | [HTTPRouteInvalidBackendRefUnknownKind](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-backendref-unknown-kind.go) | ok |
| | [HTTPRouteInvalidCrossNamespaceBackendRef](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-cross-namespace-backend-ref.go) | ok |
| | [HTTPRouteInvalidCrossNamespaceParentRef](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-cross-namespace-parent-ref.go)  | ok |
| | [HTTPRouteInvalidParentRefNotMatchingListenerPort](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-parentref-not-matching-listener-port.go) | ok |
| | [HTTPRouteInvalidParentRefNotMatchingSectionName](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-parentref-not-matching-section-name.go) | ok |
| | | | |
| | [HTTPRouteMethodMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-method-matching.go) | nok | not support in controller yet. [#123](https://github.com/aws/aws-application-networking-k8s/issues/123) |
| | | | |
| | [HTTPRouteHostnameIntersection](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-hostname-intersection.go) | NA | VPC lattice only support one hostname for BYOC
| | HTTPRouteQueryParamMatching | NA | Not supported by lattice |
| | HTTPRouteRedirectHostAndStatus | NA | same as above |
| | HTTPRouteRedirectPath | NA | same as above |
| | HTTPRouteRedirectPort | NA | same as above | 
| | HTTPRouteRedirectScheme | NA | same as above |
| | HTTPRouteRequestHeaderModifier | NA | same as above |
| | HTTPRouteResponseHeaderModifier | NA | same as above |
| | HTTPRouteRewriteHost | NA | same as above |
| | HTTPRouteRewritePath | NA | same as above |

## Running Gateway API Conformance

### Running controller from cloud desktop

```
# create a gateway first in the cluster
kubectl apply -f example my-hotel-gateway.yaml

# run controller in following mode

REGION=us-west-2 CLUSTER_LOCAL_GATEWAY=my-hotel TARGET_GROUP_NAME_LEN_MODE="long" \
make run
```

### Run individual conformance test

```
# 
# go test ./conformance/... --run TestConformance/<ShortName>


 go test ./conformance/... --run TestConformance/HTTPRouteInvalidParentRefNotMatchingSectionName \
 -args -gateway-class=amazon-vpc-lattice

```






