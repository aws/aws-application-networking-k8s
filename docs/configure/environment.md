### Environment Variables
AWS Gateway API Controller for VPC Lattice supports a number of configuration options, which are set through environment variables.
The following environment variables are available, and all of them are optional.

---

#### `CLUSTER_NAME`

Type: string

Default: Inferred from IMDS metadata

A unique name to identify a cluster. This will be used in AWS resource tags to record ownership.

---

#### `CLUSTER_VPC_ID`

Type: string

Default: Inferred from IMDS metadata

When running AWS Gateway API Controller outside the Kubernetes Cluster, this specifies the VPC of the cluster.

---

#### `AWS_ACCOUNT_ID`

Type: string

Default: Inferred from IMDS metadata

When running AWS Gateway API Controller outside the Kubernetes Cluster, this specifies the AWS account.

---

#### `REGION`

Type: string

Default: "us-west-2"

When running AWS Gateway API Controller outside the Kubernetes Cluster, this specifies the region of VPC lattice service endpoint

---

#### `GATEWAY_API_CONTROLLER_LOGLEVEL`

Type: string

Default: "info"

When set as "debug", the AWS Gateway API Controller will emit debug level logs.


---

#### `CLUSTER_LOCAL_GATEWAY`

Type: string

Default: "NO_DEFAULT_SERVICE_NETWORK"

When it is set to something different from "NO_DEFAULT_SERVICE_NETWORK", the AWS Gateway API Controller will associate its correspoding Lattice Service Network to cluster's VPC.
Also, all HTTPRoutes will automatically have an additional parentref to this `CLUSTER_LOCAL_GATEWAY`.

---

#### `TARGET_GROUP_NAME_LEN_MODE`

Type: string

Default:  "short"

When set to "long", the controller will create Lattice TargetGroup as follows.

For Kubernetes Service as BackendRef:

`k8s-(k8s service name)-(k8s service namespace)-(k8s httproute name)-(VPC ID)-(protocol)-(protocol version)`

For ServiceExport of a Kubernetes Service:

`k8s-(k8s service name)-(k8s service namespace)-(VPC-ID)-(protocol)-(protocol version)`

By default, the controller will create Lattice TargetGroup as follows

For Kubernetes Service as BackendRef:

`k8s-(k8s service name)-(k8s service namespace)-(protocol)-(protocol version)`

For ServiceExport of a Kubernetes Service:

`k8s-(k8s service name)-(k8s service namespace)-(protocol)-(protocol version)`


```
# for  examples/inventory-route.yaml 
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: inventory
spec:
  parentRefs:
  - name: my-hotel
    sectionName: http
  rules:
  - backendRefs:
    - name: inventory-ver1
      kind: Service
      port: 8090
      weight: 10

 # by default, lattice target group name 
 k8s-inventory-ver1-default


 # when TARGET_GROUP_NAME_LEN_MODE = "long", lattice target group name, e.g.

 k8s-inventory-ver1-default-inventory-vpc-05c7322a3df3f255a

```

```
# for examples/parking-ver2-export.yaml 
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceExport
metadata:
  name: parking-ver2
  annotations:
    application-networking.k8s.aws/federation: "amazon-vpc-lattice"

# by default, lattice target group name is
k8s-parking-ver2-default

# when TARGET_GROUP_NAME_LEN_MODE = "long", lattice target group name, e.g.
k8s-parking-ver2-default-vpc-05c7322a3df3f255a

```
