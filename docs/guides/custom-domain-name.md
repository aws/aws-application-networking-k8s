# Configure a Custom Domain Name for HTTPRoute

When you create a HTTPRoute under `amazon-vpc-lattice` gatewayclass, the controller creates a AWS VPC Lattice Service during reconciliation.
VPC Lattice generates a unique Fully Qualified Domain Name (FQDN) for you; however, this auto-generated domain name is not easy to remember.

If you'd prefer to use a custom domain name for a HTTPRoute, you can specify them in hostname field of HTTPRoute. Here is one example:

```yaml title="custom-domain-route.yaml" hl_lines="7"
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: review
spec:
  hostnames:
  - review.my-test.com  # this is the custom domain name
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


## Managing DNS records using ExternalDNS

To register custom domain names to your DNS provider, we recommend using [ExternalDNS](https://github.com/kubernetes-sigs/external-dns).
The AWS VPC Lattice Gateway API Controller supports ExternalDNS integration through CRD source - the controller will manage `DNSEndpoint` resource for you.

To use ExternalDNS with the AWS VPC Lattice Gateway API Controller, you need to:

1. Install `DNSEndpoint` CRD. This is bundled with both Gateway API Controller Helm chart and `files/controller-installation/deploy-*.yaml` manifest, but also can be installed manually by the following command:
   ```sh
   kubectl apply -f config/crds/bases/externaldns.k8s.io_dnsendpoints.yaml
   ```

    !!! Note
        If the `DNSEndpoint` CRD does not exist, `DNSEndpoint` resource will not be created nor will be managed by the controller.

1. Restart the controller if running already.
1. Run ExternalDNS controller watching `crd` source. 
   The following example command runs ExternalDNS compiled from source, using AWS Route53 provider:
   ```sh
   build/external-dns --source crd --crd-source-apiversion externaldns.k8s.io/v1alpha1 \
   --crd-source-kind DNSEndpoint --provider aws --txt-prefix "prefix."
   ```
1. Create HTTPRoutes and Services. The controller should create `DNSEndpoint` resource owned by the HTTPRoute you created.
1. ExternalDNS will watch the changes and create DNS record on the configured DNS provider.

## Notes

* You MUST have a registered hosted zone (e.g. `my-test.com`) in Route53 and complete the `Prerequisites` mentioned in [this section](https://docs.aws.amazon.com/vpc-lattice/latest/ug/service-custom-domain-name.html) of the Amazon VPC Lattice documentation.
* If you are not using ExternalDNS, you should manually associate your custom domain name with your service following [this section](https://docs.aws.amazon.com/vpc-lattice/latest/ug/service-custom-domain-name.html#dns-associate-custom) of the Amazon VPC Lattice documentation.
