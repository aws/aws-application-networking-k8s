# Frequently Asked Questions (FAQ)



**How can I get involved with AWS Gateway API Controller?**

We welcome general feedback, questions, feature requests, or bug reports by creating a [Github issue](https://github.com/aws/aws-application-networking-k8s/issues/new).

**Where can I find AWS Gateway API Controller releases?**

AWS Gateway API Controller releases are tags of the Github repository. The [Github releases page](https://github.com/aws/aws-application-networking-k8s/releases) shows all the releases.

**Which EKS CNI versions are supported?**

Your AWS VPC CNI must be v1.8.0 or later to work with VPC Lattice.

**Which versions of Gateway API are supported?**

AWS Gateway API Controller supports Gateway API CRD bundle versions `v1.1` or greater. Not all features of Gateway API are supported - for detailed features and limitation, please refer to individual API references. Please note that users are required to install Gateway API CRDs themselves as these are no longer bundled as of release `v1.1.0`. The latest Gateway API CRDs are available [here](https://gateway-api.sigs.k8s.io/). Please [follow this installation](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api) process.

**How do health checks work in multi-cluster deployments?**

In multi-cluster deployments, when you apply a TargetGroupPolicy to a ServiceExport, the health check configuration is automatically propagated to all target groups across all clusters that participate in the service mesh. This ensures consistent health monitoring behavior regardless of which cluster contains the route resource.

**How do I prevent 503 errors during deployments?**

When using AWS Gateway API Controller with EKS, customers may experience 503 errors during deployments due to a timing gap between pod termination and VPC Lattice configuration propagation, which affects the time controller takes to deregister a terminating pod. We recommend setting `terminationGracePeriod` to at least 150 seconds and implementing a preStop hook that has a sleep of 60 seconds (but no more than the `terminationGracePeriod`). For optimal performance, also consider setting `ROUTE_MAX_CONCURRENT_RECONCILES` to 10 which further accelerates the pod deregistration process, regardless of the number of targets.