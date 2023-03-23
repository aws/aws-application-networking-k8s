# Configure a Custom Domain Name for HTTPRoute
Today when you create a HTTPRoute using `amazon-vpc-lattice` gatewayclass, Lattice gateway-api-controller creates a AWS VPC Lattice Service during reconciliation. And VPC Lattice generates a unique Fully Qualified Domain Name (FQDN). However, this VPC Lattice generated domain name is not easy for customers to remember and use.

If you'd prefer to use a custom domain name for a HTTPRoute, you can specify them in hostname field of HTTPRoute.  Here is one example

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

## Notes

* You MUST have a registered domain name (e.g. `my-test.com`) in route53 and complete the `Prerequisites` mentioned in [Configure a custom domain name for your service](https://docs.aws.amazon.com/vpc-lattice/latest/ug/service-custom-domain-name.html#dns-associate-custom).

* In addition, you NEED to manually associate your custom domain name with your service following [Configure a custom domain name for your service](https://docs.aws.amazon.com/vpc-lattice/latest/ug/service-custom-domain-name.html#dns-associate-custom).

