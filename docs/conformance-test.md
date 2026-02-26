# Report on Gateway API Conformance Testing

[Kubernetes Gateway API Conformance](https://gateway-api.sigs.k8s.io/concepts/conformance/?h=conformance)

## Test Environment

- **Controller**: Amazon VPC Lattice Gateway API Controller v2.0.1
- **Conformance Test Suite**: Gateway API v1.4.0
- **GatewayClass**: `amazon-vpc-lattice`

## Summary of Test Result

| Category | Test Cases | Status | Notes |
| - | - | - | - |
| GatewayClass | [GatewayClassObservedGenerationBump](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gatewayclass-observed-generation-bump.go) | ok | |
| Gateway | [GatewayObservedGenerationBump](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-observed-generation-bump.go) | ok | |
| | [GatewayInvalidRouteKind](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-invalid-route-kind.go) | ok | |
| | [GatewayModifyListeners](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-modify-listeners.go) | ok | |
| | [GatewayWithAttachedRoutes](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-with-attached-routes.go) | partial | 2/3 subtests pass. Unresolved refs subtest fails because controller does not validate TLS certificateRefs (Lattice uses ACM) |
| | [GatewayInvalidTLSConfiguration](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-invalid-tls-configuration.go) | N/A | VPC Lattice uses ACM for TLS, not K8s Secrets |
| | [GatewaySecretInvalidReferenceGrant](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-invalid-reference-grant.go) | N/A | VPC Lattice uses ACM for TLS |
| | [GatewaySecretMissingReferenceGrant](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-missing-reference-grant.go) | N/A | VPC Lattice uses ACM for TLS |
| | [GatewaySecretReferenceGrantAllInNamespace](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-reference-grant-all-in-namespace.go) | N/A | VPC Lattice uses ACM for TLS |
| | [GatewaySecretReferenceGrantSpecific](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-secret-reference-grant-specific.go) | N/A | VPC Lattice uses ACM for TLS |
| | [GatewayStaticAddresses](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-static-addresses.go) | N/A | Not supported |
| | [GatewayHTTPListenerIsolation](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-http-listener-isolation.go) | N/A | Not supported |
| | [GatewayInfrastructure](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/gateway-infrastructure.go) | N/A | Not supported |
| | | | |
| HTTPRoute | [HTTPRouteSimpleSameNamespace](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-simple-same-namespace.go) | ok | Flaky when run after other tests due to Lattice resource cleanup timing |
| | [HTTPRouteCrossNamespace](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-cross-namespace.go) | ok | |
| | [HTTPRouteExactPathMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-exact-path-matching.go) | ok | 6/6 subtests pass |
| | [HTTPRouteInvalidCrossNamespaceParentRef](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-cross-namespace-parent-ref.go) | ok | |
| | [HTTPRouteInvalidParentRefNotMatchingSectionName](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-parentref-not-matching-section-name.go) | ok | |
| | [HTTPRouteHeaderMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-header-matching.go) | fail | Test data exceeds Lattice limit on # of rules |
| | [HTTPRouteMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-matching.go) | fail | Route precedence not supported by Lattice |
| | [HTTPRouteMatchingAcrossRoutes](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-matching-across-routes.go) | fail | Custom domain name conflict not allowed |
| | [HTTPRoutePathMatchOrder](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-path-match-order.go) | fail | Test data exceeds Lattice limit on # of rules |
| | [HTTPRouteObservedGenerationBump](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-observed-generation-bump.go) | fail | Passes in isolation, fails due to test ordering interference |
| | [HTTPRouteInvalidNonExistentBackendRef](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-backendref-nonexistent.go) | partial | 1/2 subtests pass. Returns 404 instead of expected 500 for invalid backend. Fix: [#899](https://github.com/aws/aws-application-networking-k8s/pull/899) |
| | [HTTPRouteInvalidBackendRefUnknownKind](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-backendref-unknown-kind.go) | partial | Status condition set correctly, returns 404 instead of expected 500 for invalid backend. Fix: [#899](https://github.com/aws/aws-application-networking-k8s/pull/899) |
| | [HTTPRouteWeight](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-weight.go) | fail | DNS resolution issues during test |
| | [HTTPRouteServiceTypes](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-service-types.go) | fail | Headless services not supported |
| | [HTTPRouteHTTPSListener](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-https-listener.go) | fail | Listener hostname not supported |
| | [HTTPRouteRedirectHostAndStatus](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-redirect-host-and-status.go) | N/A | Not supported by Lattice |
| | [HTTPRouteRequestHeaderModifier](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-request-header-modifier.go) | N/A | Not supported by Lattice |
| | [HTTPRouteResponseHeaderModifier](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-response-header-modifier.go) | N/A | Not supported by Lattice |
| | [HTTPRouteRewriteHost](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-rewrite-host.go) | N/A | Not supported by Lattice |
| | [HTTPRouteRewritePath](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-rewrite-path.go) | N/A | Not supported by Lattice |
| | [HTTPRouteRedirectPath](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-redirect-path.go) | N/A | Not supported by Lattice |
| | [HTTPRouteRedirectPort](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-redirect-port.go) | N/A | Not supported by Lattice |
| | [HTTPRouteRedirectScheme](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-redirect-scheme.go) | N/A | Not supported by Lattice |
| | [HTTPRouteHostnameIntersection](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-hostname-intersection.go) | N/A | VPC Lattice only supports one custom domain |
| | [HTTPRouteListenerHostnameMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-listener-hostname-matching.go) | N/A | Listener hostname not supported |
| | [HTTPRouteQueryParamMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-query-param-matching.go) | N/A | Not supported by Lattice |
| | [HTTPRouteMethodMatching](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-method-matching.go) | N/A | Not supported by Lattice |
| | [HTTPRouteReferenceGrant](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-reference-grant.go) | N/A | |
| | [HTTPRouteDisallowedKind](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-disallowed-kind.go) | N/A | Only HTTPRoute is supported |
| | [HTTPRouteInvalidCrossNamespaceBackendRef](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-cross-namespace-backend-ref.go) | N/A | Not supported |
| | [HTTPRouteInvalidParentRefNotMatchingListenerPort](https://github.com/kubernetes-sigs/gateway-api/blob/main/conformance/tests/httproute-invalid-parentref-not-matching-listener-port.go) | N/A | Not supported |

## Running Gateway API Conformance

### Running controller from cloud desktop

```
AWS_EC2_METADATA_DISABLED=false AWS_REGION=us-west-2 \
CLUSTER_VPC_ID=<vpc-id> AWS_ACCOUNT_ID=<account-id> \
CLUSTER_NAME=<cluster-name> DEFAULT_SERVICE_NETWORK=<service-network> \
ENABLE_SERVICE_NETWORK_OVERRIDE=true \
go run cmd/aws-application-networking-k8s/main.go
```

### Running conformance tests

Conformance tests send traffic directly, so they must run inside the VPC. Use an in-cluster pod:

```bash
# Build test binary
cd <gateway-api-repo>
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c ./conformance/ -o conformance.test

# Create runner pod
kubectl run conformance-runner --image=public.ecr.aws/docker/library/alpine:3.19 --command -- sleep infinity
kubectl wait --for=condition=Ready pod/conformance-runner --timeout=60s
kubectl create clusterrolebinding conformance-runner-admin --clusterrole=cluster-admin --serviceaccount=default:default

# Copy binary and run
kubectl cp conformance.test conformance-runner:/conformance.test
kubectl exec conformance-runner -- chmod +x /conformance.test
kubectl exec conformance-runner -- /conformance.test -test.v -test.timeout 180m -test.run TestConformance \
    --gateway-class=amazon-vpc-lattice \
    --supported-features=Gateway,HTTPRoute
```

### Run individual conformance test

```bash
kubectl exec conformance-runner -- /conformance.test -test.v -test.timeout 15m \
    -test.run "TestConformance/HTTPRouteCrossNamespace$" \
    --gateway-class=amazon-vpc-lattice \
    --supported-features=Gateway,HTTPRoute
```
