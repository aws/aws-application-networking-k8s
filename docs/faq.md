# Frequently Asked Questions (FAQ)



**How can I get involved with AWS Gateway API Controller?**

We welcome general feedback, questions, feature requests, or bug reports by creating a [Github issue](https://github.com/aws/aws-application-networking-k8s/issues/new).

**Where can I find AWS Gateway API Controller releases?**

AWS Gateway API Controller releases are tags of the Github repository. The [Github releases page](https://github.com/aws/aws-application-networking-k8s/releases) shows all the releases.

**Which EKS CNI versions are supported?**

Your AWS VPC CNI must be v1.8.0 or later to work with VPC Lattice.

**Which versions of Gateway API are supported?**

AWS Gateway API Controller supports Gateway API CRD bundle versions between v0.6.1 and v1.0.0.
The controller does not reject other versions, but will provide "best effort support" to it.
Not all features of Gateway API are supported - for detailed features and limitation, please refer to individual API references.  
By default, Gateway API v0.6.1 CRD bundle is included in the helm chart.