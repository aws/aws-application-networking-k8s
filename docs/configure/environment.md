### Environment Variables
AWS Gateway API Controller for VPC Lattice supports a number of configuration options, which are set through environment variables.
The following environment variables are available, and all of them are optional.

---

#### `CLUSTER_VPC_ID`

Type: string

Default: ""

When running AWS Gateway API Controller outside the Kubernetes Cluster, this specify the VPC of the cluster.

---

#### `AWS_ACCOUNT_ID`

Type: string

Default: ""

When running AWS Gateway API Controller outside the Kubernetes Cluster, this specify the AWS account.

---

#### `REGION`

Type: string

Default: "us-west-2"

When running AWS Gateway API Controller outside the Kubernetes Cluster, this specify the region of VPC lattice service endpoint

---

#### `GATEWAY_API_CONTROLLER_LOGLEVEL`

Type: string

Default: "info"

When it is set as "debug", the AWS Gateway API Controller prints detailed debugging information.  Otherwise, it is only prints the key information


---

#### `CLUSTER_LOCAL_GATEWAY`

Type: string

Default: "NO_DEFAULT_SERVICE_NETWORK"

When it is set to something different than "NO_DEFAULT_SERVICE_NETWORK", the AWS Gateway API Controller will associate its correspoding Lattice Service Network to cluster's VPC.  Also, all HTTPRoutes will automatically have an additional parentref to this `CLUSTER_LOCAL_GATEWAY`

---

#### `TARGET_GROUP_NAME_LEN_MODE`

Type: string

Default:  "short"

***If it is set to "long"*** 
 AWS Gateway API will create Lattice Target Group as follows

for backendRef of a Kubernetes Service:

k8s-(k8s service name)-(k8s service namespace)-(k8s httproute name)-(VPC ID)

for serviceexport of a Kubernetes Servic:

k8s-(k8s service name)-(k8s service namespace)-(VPC-ID)

***By default***
  AWS Gateway API will create Lattice Target Group as follows

for backendRef of a Kubernetes Service:

k8s-(k8s service name)-(k8s service namespace)

for serviceexport of a Kubernetes Service:

k8s-(k8s service name)-(k8s service namespace)


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
apiVersion: multicluster.x-k8s.io/v1alpha1
kind: ServiceExport
metadata:
  name: parking-ver2
  annotations:
    multicluster.x-k8s.io/federation: "amazon-vpc-lattice"

# by default, lattice target group name is
k8s-parking-ver2-default

# when TARGET_GROUP_NAME_LEN_MODE = "long", lattice target group name, e.g.
k8s-parking-ver2-default-vpc-05c7322a3df3f255a

```
