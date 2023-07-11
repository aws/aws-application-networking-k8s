# Configure a Custom Domain Name for HTTPRoute
When you create a HTTPRoute under `amazon-vpc-lattice` gatewayclass, the controller creates a AWS VPC Lattice Service during reconciliation.
VPC Lattice generates a unique Fully Qualified Domain Name (FQDN) for you; however, this auto-generated domain name is not easy to remember.

If you'd prefer to use a custom domain name for a HTTPRoute, you can specify them in hostname field of HTTPRoute.  Here is one example:

```
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: HTTPRoute
metadata:
  name: review
spec:
  hostnames:
  - review.my-test.com  <-----------  this is the custom domain name
  parentRefs:
  - name: my-hotel
    sectionName: http
  rules:    
  - backendRefs:
    - name: review2
      kind: Service
      port: 8090
    matches:
    - path:
        type: PathPrefix
        value: /review2

```


## Generating DNS records using ExternalDNS

AWS Gateway API Controller supports integration with [ExternalDNS](https://github.com/kubernetes-sigs/external-dns) using CRD source.

1. Install DNSEndpoint CRD. This is bundled with Gateway API Controller helm chart, but also can be installed manually by the following command:
   ```sh
   kubectl apply -f config/crds/bases/externaldns.k8s.io_dnsendpoints.yaml
   ```
2. Restart the controller if running already.
3. Run ExternalDNS controller watching `crd` source. The following example command runs ExternalDNS compiled from source:
   ```sh
   build/external-dns --source crd --crd-source-apiversion externaldns.k8s.io/v1alpha1 \
   --crd-source-kind DNSEndpoint --provider aws
   ```
4. Create HTTPRoutes and Services. The controller should create DNSEndpoint resource owned by the HTTPRoute you created.
5. ExternalDNS will watch the changes and create DNS record on the configured DNS provider.

## Notes

* You MUST have a registered domain name (e.g. `my-test.com`) in route53 and complete the `Prerequisites` mentioned in [Configure a custom domain name for your service](https://docs.aws.amazon.com/vpc-lattice/latest/ug/service-custom-domain-name.html#dns-associate-custom).
* If you are not using ExternalDNS, you should manually associate your custom domain name with your service following [Configure a custom domain name for your service](https://docs.aws.amazon.com/vpc-lattice/latest/ug/service-custom-domain-name.html#dns-associate-custom).

