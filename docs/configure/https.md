## Configure HTTPs connections

The Getting Started guide uses HTTP (insecure) communications by default.
Using the examples here, you can change that to HTTPS (secure) communications.
If you choose, you can further customize your HTTPS connections by adding custom domain names and certificates, as described below.

**NOTE**: You can get the yaml files used on this page by cloning the [AWS Gateway API Controller for VPC Lattice](https://github.com/aws/aws-application-networking-k8s) site. The files are in the `examples/` directory.

### Securing Traffic using HTTPS

By adding https to the amazon-vpc-lattice gateway, you can tell the listener to use HTTPs communications.
The following modifications to the `examples/my-hotel-gateway.yaml` file add HTTPs communications:

```
apiVersion: gateway.networking.k8s.io/v1beta1
kind: Gateway
metadata:
  name: my-hotel
  annotations:
    application-networking.k8s.aws/lattice-vpc-association: "true"
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
  - name: http
    protocol: HTTP
    port: 80
  - name: https         # Specify https listener
    protocol: HTTPS     # Specify HTTPS protocol
    port: 443           # Specify communication on port 443
...
```    
Next, the following modifications to the `examples/rate-route-path.yaml` file tell the `rates` HTTPRoute to use HTTPS for communications:

```
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: rates
spec:
  parentRefs:
  - name: my-hotel
    sectionName: http 
  - name: my-hotel      # Specify the parertRefs name
    sectionName: https  # Specify all traffic MUST use HTTPs
  rules:
...
```

In this case, the VPC Lattice service automatically generates a managed ACM certificate and uses it for encryting client to service traffic.

### Bring Your Own Certificate (BYOC)

If you want to use a custom domain name along with its own certificate, you can:

* Follow instructions on [Requesting a public certificate](https://docs.aws.amazon.com/acm/latest/userguide/gs-acm-request-public.html) and get an ACM certificate ARN.
* Add the ARN to the listener configuration as shown below.

The following shows modifications to `examples/my-hotel.yaml` to add a custom certificate:
```
apiVersion: gateway.networking.k8s.io/v1beta1
kind: Gateway
metadata:
  name: my-hotel
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
  - name: http
    protocol: HTTP
    port: 80
  - name: https
    protocol: HTTPS
    port: 443
  - name: tls-with-custom-cert       # Specify listener with custom certs
    protocol: HTTPS                  # Specify HTTPS protocol 
    port: 443                        # Specify communication on port 443
    tls:                             # Specify TLS configuration
      mode: Terminate                # Specify mode for TLS listener
      options:                       # Specify certificate options
        application-networking.k8s.aws/certificate-arn: arn:aws:acm:us-west-2:<account>:certificate/4555204d-07e1-43f0-a533-d02750f41545 
```
Note that only `Terminate` mode is supported (Passthrough is not supported).

Next, associate the HTTPRoute to the listener configuration you just configured:

```
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: rates
spec:
  hostnames:
    - review.my-test.com               # MUST match the DNS in the certificate
  parentRefs:
  - name: my-hotel
    sectionName: http 
  - name: my-hotel                     # Specify parentRefs
    sectionName: tls-with-custom-cert  # Specify custom-defined certificate 
...
```        
