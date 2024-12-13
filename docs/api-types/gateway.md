# Gateway API Reference

## Introduction

`Gateway` allows you to configure network traffic through AWS Gateway API Controller.
When a Gateway is defined with `amazon-vpc-lattice` GatewayClass, the controller will watch for the gateway
and the resources under them, creating required resources under Amazon VPC Lattice.

Internally, a Gateway points to a VPC Lattice [service network](https://docs.aws.amazon.com/vpc-lattice/latest/ug/service-networks.html).
Service networks are identified by Gateway name (without namespace) - for example, a Gateway named `my-gateway`
will point to a VPC Lattice service network `my-gateway`. If multiple Gateways share the same name, all of them
will point to the same service network.

VPC Lattice service networks must be managed separately, as it is a broader concept that can cover resources
outside the Kubernetes cluster. To create and manage a service network, you can either:

- Specify `DEFAULT_SERVICE_NETWORK` configuration option on the controller. This will make the controller
  to create a service network with such name, and associate the cluster VPC to it for you. This is suitable
  for simple use cases with single service network.
- Manage service networks outside the cluster, using AWS Console, CDK, CloudFormation, etc. This is recommended
  for more advanced use cases that cover multiple clusters and VPCs.

Gateways with `amazon-vpc-lattice` GatewayClass do not create a single entrypoint to bind Listeners and Routes
under them. Instead, each Route will have its own domain name assigned. To see an example of how domain names
are assigned, please refer to our [Getting Started Guide](../guides/getstarted.md).

### Supported GatewayClass
- `amazon-vpc-lattice`  
  This is the default GatewayClass for managing traffic using Amazon VPC Lattice.

### Limitations
- GatewayAddress status does not represent all accessible endpoints belong to a Gateway.
  Instead, you should check annotations of each Route.
- Only `Terminate` is supported for TLS mode. TLSRoute is currently not supported.
- TLS certificate cannot be provided through `certificateRefs` field by `Secret` resource.
  Instead, you can create an ACM certificate and put its ARN to the `options` field.

## Example Configuration

Here is a sample configuration that demonstrates how to set up a `Gateway`:

```yaml
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
    - name: https
      protocol: HTTPS
      port: 443
      tls:
        mode: Terminate
        certificateRefs:
          - name: unused
        options:
          application-networking.k8s.aws/certificate-arn: <certificate-arn>
```

The created Gateway will point to a VPC Lattice service network named `my-hotel`. Routes under this Gateway can have
either `http` or `https` listener as a parent based on their desired protocol to use.

---

This `Gateway` documentation provides a detailed introduction, feature set, and a basic example of how to configure
and use the resource within AWS Gateway API Controller project. For in-depth details and specifications, you can refer to the
official [Gateway API documentation](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.Gateway).