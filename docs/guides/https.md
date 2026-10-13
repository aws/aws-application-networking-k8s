## Configure HTTPs connections

The [Getting Started](./getstarted.md) guide uses `HTTP` communications by default. Using the examples here, you can change that to `HTTPs`. If you choose, you can further customize your `HTTPs` connections by adding custom domain names and certificates, as described below.

**NOTE**: You can get the yaml files used on this page by cloning the [AWS Gateway API Controller for VPC Lattice](https://github.com/aws/aws-application-networking-k8s) site. The files are in the `files/examples/` directory.

### Securing Traffic using HTTPs

By adding https to the amazon-vpc-lattice gateway, you can tell the listener to use HTTPs communications.
The following modifications to the `files/examples/my-hotel-gateway.yaml` file add HTTPs communications:

```yaml title="my-hotel-gateway.yaml" hl_lines="11 12 13"
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-hotel
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

Next, the following modifications to the `files/examples/rate-route-path.yaml` file tell the `rates` HTTPRoute to use HTTPs for communications:

```yaml title="rate-route-path.yaml" hl_lines="10"
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: rates
spec:
  parentRefs:
  - name: my-hotel
    sectionName: http 
  - name: my-hotel      # Specify the parentRefs name
    sectionName: https  # Specify all traffic MUST use HTTPs
  rules:
...
```

In this case, the VPC Lattice service automatically generates a managed ACM certificate and uses it for encrypting client to service traffic.

### Bring Your Own Certificate (BYOC)

If you want to use a custom domain name along with its own certificate, follow instructions on [Requesting a public certificate](https://docs.aws.amazon.com/acm/latest/userguide/gs-acm-request-public.html) to create a certificate for your custom domain name in ACM. 

Note that only `Terminate` mode is supported for HTTPS listeners. For TLS Passthrough, use a `protocol: TLS` listener with a [TLSRoute](../api-types/tls-route.md) instead.

#### Automatic Certificate Discovery

If you have an ACM certificate matching your route's hostname, the controller can automatically discover and use it. Configure an HTTPS listener without the `certificate-arn` option:

```yaml title="my-hotel-gateway.yaml"
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-hotel
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
  - name: https
    protocol: HTTPS      # This is required
    port: 443
    tls:
      mode: Terminate    # This is required
      certificateRefs:   # This is required per API spec, but currently not used by the controller
      - name: unused
      # No certificate-arn option needed — the controller discovers it from ACM
```

The HTTPRoute must specify a `hostnames` field matching the certificate's domain or SAN:

```yaml title="rate-route-path.yaml"
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: rates
spec:
  hostnames:
    - review.my-test.com               # MUST match the DNS in the certificate
  parentRefs:
  - name: my-hotel
    sectionName: https
  rules:
  ...
```

#### Manual Certificate ARN

If you want to explicitly specify which certificate to use, add the ARN to the listener configuration. This takes priority over automatic discovery.

```yaml title="my-hotel-gateway.yaml" hl_lines="16 17 18 19"
apiVersion: gateway.networking.k8s.io/v1
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
  - name: https
    protocol: HTTPS      # This is required
    port: 443
    tls:
      mode: Terminate    # This is required
      certificateRefs:   # This is required per API spec, but currently not used by the controller
      - name: unused
      options:           # Instead, we specify ACM certificate ARN under this section
        application-networking.k8s.aws/certificate-arn: arn:aws:acm:us-west-2:<account>:certificate/<certificate-id>
```

Next, associate the HTTPRoute to the listener configuration you just configured:

```yaml title="rate-route-path.yaml" hl_lines="7"
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: rates
spec:
  hostnames:
    - review.my-test.com               # MUST match the DNS in the certificate
  parentRefs:
  - name: my-hotel
    sectionName: http 
  - name: my-hotel                     # Use the listener defined above as parentRef
    sectionName: https
...
```

### Enabling TLS connection on the backend

If your backend pods require TLS connections, you can configure VPC Lattice to re-encrypt traffic before forwarding it to your pods. This is useful when you want VPC Lattice to terminate client-facing TLS (for HTTP routing and inspection) while still encrypting the connection from VPC Lattice to your pods.

!!! note
    If you want end-to-end passthrough without TLS termination, use a [TLSRoute](../api-types/tls-route.md) with a `protocol: TLS`, `mode: Passthrough` Gateway listener instead. The approach below is for re-encryption, where VPC Lattice terminates and then re-establishes TLS to the backend.

To configure TLS re-encryption, create a `TargetGroupPolicy` with `protocol: HTTPS`:

```yaml title="target-group-policy.yaml" hl_lines="10"
apiVersion: application-networking.k8s.aws/v1alpha1
kind: TargetGroupPolicy
metadata:
    name: test-policy
spec:
    targetRef:
        group: ""
        kind: Service
        name: my-parking-service # Put service name here
    protocol: HTTPS
    protocolVersion: HTTP1
```

This creates a VPC Lattice target group with HTTPS protocol. Lattice will use TLS when forwarding traffic to your pods.

!!! warning "Health check protocol"
    If your pods only accept TLS connections, you **must** also set `healthCheck.protocol: HTTPS` in the TargetGroupPolicy. Health checks default to HTTP, so without this setting they will fail and targets will never become healthy.

    ```yaml
    healthCheck:
        protocol: HTTPS
        path: /healthz
        statusMatch: "200"
    ```

#### Certificate requirements

VPC Lattice does not validate backend certificates, so self-signed certificates work without any CA or trust bundle configuration. Re-encryption provides transport-level encryption between VPC Lattice and your pods, but does not authenticate the backend server's identity.

For more details on TargetGroupPolicy fields, see the [TargetGroupPolicy API reference](../api-types/target-group-policy.md).
