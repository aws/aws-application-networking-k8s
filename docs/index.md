# AWS Gateway API Controller User Guide

The AWS Gateway API Controller lets you connect services across multiple Kubernetes clusters through the Kubernetes Gateway API interface.
It is also designed to connect services running on EC2 instances, containers, and as serverless functions.
It does this by leveraging Amazon VPC Lattice, which works with Kubernetes Gateway API calls to manage Kubernetes objects. 

This document describes how to set up the AWS Gateway API Controller and provides example use cases.
With the controller deployed and running, you will be able to manage services for multiple Kubernetes clusters and other targets on AWS through the following:

* **CLI**: Use `aws` and `eksctl` to create clusters and set up AWS policies. Then use `kubectl` and YAML files to set up Kubernetes objects.
* **AWS Console**: View VPC Lattice assets through the VPC area of the AWS console.

While separating the application developer from the details of the underling infrastructure, the controller also provides a Kubernetes-native experience, rather than creating a lot of new AWS ways of managing services.
It does this by integrating with the Kubernetes Gateway API.
This lets you work with Kubernetes service-related resources using Kubernetes APIs and custom resource definitions (CRDs) defined by the Kubernetes [networking.k8s.io specification](https://gateway-api.sigs.k8s.io/references/spec/).

For more information on this technology, see [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/). 

A few things to keep in mind:

* If you are new to the VPC Lattice service, keep in mind that names you use for objects must be unique across your entire account and not just across each cluster used by that account.
* Your AWS CNI must be v1.8.0 or later to work with VPC Lattice.


