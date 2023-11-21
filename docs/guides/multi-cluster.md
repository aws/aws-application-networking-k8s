# Recommended Multi-Cluster Architecture

Here is a recommended multi-cluster architecture if you'd like to setup kubernetes service-to-service communications across multiple clusters.

Suppose your organization would like to have one "service mesh" that spans many clusters in one aws account, this service mesh should include the following components:
- One manually created VPC Lattice service network, (it could create by either AWS Console, CLI, CloudFormation, Terraform or any other tools)
- Create `VpcServiceNetworkAssociations` between VPC Lattice service network and each config cluster's VPC and workload clusters' VPCs
- Multiple workload cluster(s), that are used to run application workload(s). workload cluster(s) should only have following workloads related kubernetes objects:
  - Multiple application workload(s) (Pods, Deployments etc.)
  - Multiple `Service(s)` for application workload(s)
  - Multiple `ServiceExport(s)`, that export kubernetes application Service(s) to the "config cluster"
- One extra dedicated "config cluster", which is act as a "service mesh control plane" and it should include following kubernetes objects:
  - One `Gateway` that has __same name__ as the manually created VPC Lattice service network name
  - Multiple `ServiceImport(s)`, that reference to kubernetes application services that export from workload cluster(s)
  - Multiple `HTTPRoute(s)`,`GRPCRoute(s)`, that have rules backendRefs to `ServiceImport(s)` that referring kubernetes application service(s) in workload cluster(s)


You can see this similar production use case at Airbnb: [airbnb mullti-cluster setup](https://www.youtube.com/watch?v=1D8lg36ZNHs)

![config cluster and multiple workload clusters](../images/multi-cluster.png)

Following steps will show you how to set up this recommended multi-cluster architecture with 1 config cluster and 2 workload clusters.
1. Create 3 k8s clusters: `config cluster`, `workload cluster-1`, `workload cluster-2`. Install aws gateway api controller in each cluster, you could follow this instruction [deploy.md](deploy.md)
1. Create a VPC Lattice `ServiceNetwork` with name `my-gateway`
1. Create `VPCServiceNetworkAssociation(s)` between previous step created service network and each config cluster's VPC and workload clusters' VPCs
1. Setup following resource in the workload cluster1:
    ```
    kubectl apply -f examples/service-1.yaml
    kubectl apply -f examples/service-1-export.yaml
    ```
1. Setup following resource in the workload cluster2:
    ```
    kubectl apply -f examples/service-2.yaml
    kubectl apply -f examples/service-2-export.yaml
    ```
1. Setup following resource in the config cluster:
    ```
    kubectl apply -f examples/my-gateway.yaml
    kubectl apply -f examples/my-httproute.yaml
    kubectl apply -f examples/service-1-import.yaml
    kubectl apply -f examples/service-2-import.yaml
    ```
1. At this point, the connectivity setup finished, pods in workload cluster1 are able to communicate with `service-2` in workload cluster2 (and vice versa) via the `my-httproute` dns name.
1. Furthermore, you could have more workloads clusters to join the `my-gateway` service network by creating the `ServiceNewtorkAssociation(s)`, they will all be able to communicate with `service-1` and `service-2` via the `my-httproute` dns name and path matching.
