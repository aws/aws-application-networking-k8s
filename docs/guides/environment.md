### Environment Variables
AWS Gateway API Controller for VPC Lattice supports a number of configuration options, which are set through environment variables.
The following environment variables are available, and all of them are optional.

---

#### `CLUSTER_NAME`

**Type:** *string*

**Default:** *Inferred from IMDS metadata*

A unique name to identify a cluster. This will be used in AWS resource tags to record ownership.
This variable is required except for EKS cluster. This needs to be specified if IMDS is not available.

---

#### `CLUSTER_VPC_ID`

**Type:** *string*

**Default:** *Inferred from IMDS metadata*

When running AWS Gateway API Controller outside the Kubernetes Cluster, this specifies the VPC of the cluster. This needs to be specified if IMDS is not available.

---

#### `AWS_ACCOUNT_ID`

**Type:** *string*

**Default:** *Inferred from IMDS metadata*

When running AWS Gateway API Controller outside the Kubernetes Cluster, this specifies the AWS account. This needs to be specified if IMDS is not available.

---

#### `REGION`

**Type:** *string*

**Default:** *Inferred from IMDS metadata*

When running AWS Gateway API Controller outside the Kubernetes Cluster, this specifies the AWS Region of VPC Lattice Service endpoint. This needs to be specified if IMDS is not available.

---

#### `LOG_LEVEL`

**Type:** *string*

**Default:** *"info"*

When set as "debug", the AWS Gateway API Controller will emit debug level logs.


---

#### `DEFAULT_SERVICE_NETWORK`

**Type:** *string*

**Default:** ""

When set as a non-empty value, creates a service network with that name.
The created service network will be also associated with cluster VPC.

---

#### `ENABLE_SERVICE_NETWORK_OVERRIDE`

**Type:** *string*

**Default:** ""

When set as "true", the controller will run in "single service network" mode that will override all gateways to point to default service network, instead of searching for service network with the same name. Can be used for small setups and conformance tests.

---

#### `WEBHOOK_ENABLED`

**Type:** *string*

**Default:** ""

When set as "true", the controller will start the webhook listener responsible for pod readiness gate injection 
(see `pod-readiness-gates.md`). This is disabled by default for `deploy.yaml` because the controller will not start 
successfully without the TLS certificate for the webhook in place. While this can be fixed by running 
`scripts/gen-webhook-cert.sh`, it requires manual action. The webhook is enabled by default for the Helm install
as the Helm install will also generate the necessary certificate.
