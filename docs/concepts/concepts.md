# AWS Gateway API Controller User Guide

As part of the VPC Lattice launch, AWS introduced the AWS Gateway API Controller ; an implementation of the Kubernetes Gateway API. Gateway API is an open-source standard interface to enable Kubernetes application networking through expressive, extensible, and role-oriented interfaces. AWS Gateway API controller extends custom resources, defined by Gateway API, which allows you to create VPC Lattice resources using Kubernetes APIs.

When installed in your cluster, the controller watches for the creation of Gateway API resources such as gateways and routes and provisions corresponding Amazon VPC Lattice objects. This enables users to configure VPC Lattice Services, VPC Lattice Service Networks and Target Groups using Kubernetes APIs, without needing to write custom code or manage sidecar proxies. The AWS Gateway API Controller is an open-source project and fully supported by Amazon.

AWS Gateway API Controller integrates with Amazon VPC Lattice and allows you to:

* Handle network connectivity seamlessly between services across VPCs and accounts.
* Discover VPC Lattice services spanning multiple Kubernetes clusters.
* Implement a defense-in-depth strategy to secure communication between those services.
* Observe the request/response traffic across the services.

This documentation describes how to set up the AWS Gateway API Controller, provides example use cases, development concepts, and API references. AWS Gateway API Controller will provide developers the ability to publish services running on Kubernetes cluster and other compute platforms on AWS such as AWS Lambda or Amazon EC2. Once the AWS Gateway API controller deployed and running, you will be able to manage services for multiple Kubernetes clusters and other compute targets on AWS through the following:

* **CLI**: Use `aws` and `eksctl` to create clusters and set up AWS policies. Then use `kubectl` and YAML files to set up Kubernetes objects.
* **AWS Console**: View VPC Lattice assets through the VPC area of the AWS console.

Integrating with the Kubernetes Gateway API provides a kubernetes-native experience for developers to create services, manage network routing and traffic behaviour without the heavy lifting managing the underlying networking infrastrcuture. This lets you work with Kubernetes service-related resources using Kubernetes APIs and custom resource definitions (CRDs) defined by the Kubernetes [networking.k8s.io specification](https://gateway-api.sigs.k8s.io/references/spec/).

For more information on this technology, see [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/). 