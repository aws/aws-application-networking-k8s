# Gateway API Reference

## Introduction

`Gateway` allows you to configure network traffic through the Amazon VPC Lattice Gateway API Controller.
When a Gateway is defined with `amazon-vpc-lattice` GatewayClass, the controller will watch for the gateway
and the resources under them, creating required resources under Amazon VPC Lattice.

Internally, a Gateway points to a VPC Lattice [service network](https://docs.aws.amazon.com/vpc-lattice/latest/ug/service-networks.html).
Service networks are identified by Gateway name (without namespace) - for example, a Gateway named `my-gateway`
will point to a VPC Lattice service network `my-gateway`. If multiple Gateways share the same name, all of them
will point to the same service network.

### ServiceNetwork Lifecycle

When a Gateway is created, the controller automatically creates a VPC Lattice ServiceNetwork with the same name
and associates it with the cluster VPC. When the Gateway is deleted, the controller deletes the ServiceNetwork
if it was created by the controller (tracked via the `application-networking.k8s.aws/ManagedBy` tag).

Auto-created ServiceNetworks use **default VPC Lattice settings** — no auth policy, no sharing configuration,
and no custom attributes. If you need to configure auth type, sharing, or other
[ServiceNetwork attributes](https://docs.aws.amazon.com/vpc-lattice/latest/APIReference/API_CreateServiceNetwork.html),
create the ServiceNetwork externally (via Console, CLI, CDK, CloudFormation, etc.) before creating the Gateway.
The controller will detect the existing ServiceNetwork by name and reuse it without modifying or deleting it.

**Deletion behavior:**

- If multiple Gateways share the same ServiceNetwork name, deleting one Gateway will **not** delete the
  ServiceNetwork as long as another active Gateway with the same name still exists.
- If the ServiceNetwork has active service associations, the controller will not delete it and will report
  an error asking you to detach all services first.
- Externally-created ServiceNetworks are never deleted by the controller.

In addition to auto-creation, you can also use the `DEFAULT_SERVICE_NETWORK` configuration option on the controller
to create a default ServiceNetwork at startup.

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
and use the resource within the Amazon VPC Lattice Gateway API Controller project. For in-depth details and specifications, you can refer to the
official [Gateway API documentation](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.Gateway).
