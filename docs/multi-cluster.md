# Recommended Multi-Cluster Architecture

If you'd like to setup kubernetes service-to-service communications across multiple clusters, Following is a recommended multi-cluster architecture.

Suppose your organization have one "conceptual service mesh" that spans multiple clusters in one aws account, this service mesh should include the following components:
- One manually created VPC Lattice service network, (it could create by either AWS Console, CLI, CloudFormation, Terraform or any other tools)
- Create VPC Service Network Associations between VPC Lattice service network and each config cluster's VPC and workload clusters' VPCs
- Multiple workload cluster(s), where are used to run application workload(s), workload cluster(s) should only have following workloads related kubernetes objects:
  - Multiple application workload(s) (Pods, Deployments etc.)
  - Multiple `Service(s)` for application workload(s)
  - Multiple `ServiceExport(s)`, that export kubernetes application Service(s) to the "config cluster"
- One extra dedicated "config cluster", which is used for "mesh control plane" and where should include following kubernetes objects in the "config cluster":
  - One `gateway` that has __same name__ as the manually created VPC Lattice service network
  - Multiple `ServiceImport(s)`, that reference to kubernetes application ServiceExport(s) from workload cluster(s)
  - Multiple `HTTPRoute(s)`,`GRPCRoute(s)`, that have rules with backendRefs `ServiceImport(s)` that referring kubernetes application service(s) in workload cluster(s)


You can see this similar production use case at Airbnb [airbnb mullti-cluster](https://www.youtube.com/watch?v=1D8lg36ZNHs)

Here is our example

![Config Cluster and multiple workload cluster](images/multi-sn.png)

Following is the yaml examples steps to setup this multi-cluster architecture

1. Setup resource in the workload cluster1 

```
kubectl apply -f examples/service-1.yaml
kubectl apply -f examples/service-1-export.yaml
```  

1. Setup resource in the workload cluster2 


```
kubectl apply -f examples/service-2.yaml
kubectl apply -f examples/service-2-export.yaml
```

```
kubectl apply -f examples/my-hotel-gateway.yaml
kubectl apply -f examples/my-http-route.yaml
kubectl apply -f examples/service-1-import.yaml
kubectl apply -f examples/service-2-import.yaml
```













