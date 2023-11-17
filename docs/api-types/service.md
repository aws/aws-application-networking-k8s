# Service API Reference

## Introduction

Kubernetes Services define a logical set of Pods and a policy by which to access them, often referred to as a
microservice. The set of Pods targeted by a Service is determined by a `selector`.

### Service Key Features & Limitations

**Features**:

- **Load Balancing**: Services offer load balancing, distributing network traffic across the Pods.
- **Service Types**: Supports different types, such as ClusterIP (default), NodePort, LoadBalancer, and ExternalName.
- **Stable IP Address**: Each Service has a stable IP address, even when the Pods it routes to change.

**Limitations**:

- **Immutable Selector**: Once a Service is created, its `selector` and `type` fields cannot be updated.
- **Single Namespace**: Services can only route to Pods within the same namespace.
- **ExternalName Limitation**: `ExternalName` type is not supported by this controller.

## Example Configuration:

### Example 1

Here's a basic example of a Service that routes traffic to Pods with the label `app=MyApp`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  selector:
    app: MyApp
  ports:
    - protocol: TCP
      port: 80
      targetPort: 8080
```

In this example:

- The Service is named `my-service`.
- It targets Pods with the label `app=MyApp`.
- The Service exposes port 80, which routes to target port 8080 on the Pods.

---

This `Service` documentation provides an overview of its key features, limitations, and basic examples of configuration
within Kubernetes. For detailed specifications and advanced configurations, refer to the official
[Kubernetes Service documentation](https://kubernetes.io/docs/concepts/services-networking/service/).
