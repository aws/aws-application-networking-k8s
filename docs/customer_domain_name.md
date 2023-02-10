# Configure a Customer Domain Name 
Today when you create a HTTPRoute using `amazon-vpc-lattice` gatewayclass, lattice gateway-api-controller creates a AWS VPC lattice during reconciliation. And VPC Lattice generates a unique Fully Qualified Domain Name (FQDN). However, this VPC Lattice generated domain name is not easy for customers to remember and use.

If you'd prefer to use a customer domain name for a HTTPRoute, you can specify them in hostname field of HTTPRoute.  Here is one example

```
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: HTTPRoute
metadata:
  name: review
spec:
  hostnames:
  - review.my-test.com  <-----------  this is the customer domain name
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

* You MUST complete the `Prerequisites` mentioned in [TODO - public BYOC doc](http://dev-dsk-tnmat-1d-8836d755.us-east-1.amazon.com/mercury/build/AWSMercuryDocs/AWSMercuryDocs-3.0/AL2_x86_64/DEV.STD.PTHREAD/build/server-root/vpc-lattice/latest/ug/service-custom-domain-name.html#dns-associate-custom)

* You MUST configure a custom domain name following [TODO -public BYOC doc](http://dev-dsk-tnmat-1d-8836d755.us-east-1.amazon.com/mercury/build/AWSMercuryDocs/AWSMercuryDocs-3.0/AL2_x86_64/DEV.STD.PTHREAD/build/server-root/vpc-lattice/latest/ug/service-custom-domain-name.html#dns-associate-custom)

* In addition, you NEED to manually associate your custom domain name with your service following [TODO - public BYOC doc](http://dev-dsk-tnmat-1d-8836d755.us-east-1.amazon.com/mercury/build/AWSMercuryDocs/AWSMercuryDocs-3.0/AL2_x86_64/DEV.STD.PTHREAD/build/server-root/vpc-lattice/latest/ug/service-custom-domain-name.html#dns-associate-custom).  We do have [github issue](https://github.com/aws/aws-application-networking-k8s/issues/88), an enhancement request, to automate this process