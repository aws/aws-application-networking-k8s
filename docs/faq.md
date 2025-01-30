# Frequently Asked Questions (FAQ)



**How can I get involved with AWS Gateway API Controller?**

We welcome general feedback, questions, feature requests, or bug reports by creating a [Github issue](https://github.com/aws/aws-application-networking-k8s/issues/new).

**Where can I find AWS Gateway API Controller releases?**

AWS Gateway API Controller releases are tags of the Github repository. The [Github releases page](https://github.com/aws/aws-application-networking-k8s/releases) shows all the releases.

**Which EKS CNI versions are supported?**

Your AWS VPC CNI must be v1.8.0 or later to work with VPC Lattice.

**Which versions of Gateway API are supported?**

AWS Gateway API Controller supports Gateway API CRD bundle versions `v1.1` or greater. Not all features of Gateway API are supported - for detailed features and limitation, please refer to individual API references. Please note that users are required to install Gateway API CRDs themselves as these are no longer bundled as of release `v1.1.0`. The latest Gateway API CRDs are available [here](https://gateway-api.sigs.k8s.io/). Please [follow this installation](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api) process.