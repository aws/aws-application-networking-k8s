# Get Start Using the AWS Gateway API Controller

Once you have [deployed the AWS Gateway API Controller](configure/index.md), this guide helps you get started using the controller.

The first part of this section provides an example of setting up of service-to-service communications on a single cluster.
The second section extends that example by creating another inventory service on a second cluster on a different VPC, and spreading traffic to that service across the two clusters and VPCs.
Both clusters are created using `eksctl`, with both clusters created from the same account by the same cluster admin.

Using these examples as a foundation, see the

## Set up single-cluster/VPC service-to-service communications

This example creates a single cluster in a single VPC, then configures two routes (rates and inventory) and three services (parking, review, and inventory-1). The following figure illustrates this setup:

![Single cluster/VPC service-to-service communications](images/example1.png)

**Steps**

   **Set up Service-to-Service communications**

1. Create the Kubernetes Gateway `my-hotel`:
   ```bash
   kubectl apply -f examples/my-hotel-gateway.yaml
   ```
   ***Note***

   By default, the gateway (lattice service network) is not associated with cluster's VPC.  To associate a gateway (lattice service network) to VPC, `my-hotel-gateway.yaml` includes the following annotation.

      
       apiVersion: gateway.networking.k8s.io/v1beta1
       kind: Gateway
       metadata:
         name: my-hotel
         annotations:
           application-networking.k8s.aws/lattice-vpc-association: "true"
       

1. Verify that `my-hotel` gateway is created (this could take about five minutes):
   ```bash
   kubectl get gateway  
   ```
   ```
   NAME       CLASS                ADDRESS   READY   AGE
   my-hotel   amazon-vpc-lattice                     7d12h
   ```
1. Once the gateway is created, find the VPC Lattice service network:
   ```bash
   kubectl get gateway my-hotel -o yaml
   ```
   ```
   apiVersion: gateway.networking.k8s.io/v1beta1
   kind: Gateway
   ...
   status:
   conditions:
   message: 'aws-gateway-arn: arn:aws:vpc-lattice:us-west-2:694065802095:servicenetwork/sn-0ab6bb70055929edd'
   reason: Reconciled
   status: "True"
   type: Schedules
   ```
1. Create the Kubernetes HTTPRoute rates for the parking service, review service, and HTTPRoute rate:
   ```bash
   kubectl apply -f examples/parking.yaml
   kubectl apply -f examples/review.yaml
   kubectl apply -f examples/rate-route-path.yaml
   ```
1. Create the Kubernetes HTTPRoute inventory (this could take about five minutes):
   ```bash
   kubectl apply -f examples/inventory-ver1.yaml
   kubectl apply -f examples/inventory-route.yaml
   ```
1. Find out HTTPRoute's DNS name from HTTPRoute status:
   ```bash
   kubectl get httproute
   ```
   ```
   NAME        HOSTNAMES   AGE
   inventory               51s
   rates                   6m11s
   ```
1. List the route’s yaml file to see the DNS address (highlighted here on the `message` line):

      ```bash
      kubectl get httproute inventory -o yaml
      ```

      ```
      apiVersion: gateway.networking.k8s.io/v1beta1
      kind: HTTPRoute
      metadata:
        annotations:
          application-networking.k8s.aws/lattice-assigned-domain-name: inventory-default-02fb06f1acdeb5b55.7d67968.vpc-lattice-svcs.us-west-2.on.aws
      ...
      ```
      
      ```bash
      kubectl get httproute rates -o yaml
      ```

      ```
      apiVersion: v1
      items:
      - apiVersion: gateway.networking.k8s.io/v1beta1
        kind: HTTPRoute
        metadata:
          annotations:
            application-networking.k8s.aws/lattice-assigned-domain-name: rates-default-0d38139624f20d213.7d67968.vpc-lattice-svcs.us-west-2.on.aws
      ...
      ```

**Check service connectivity**

1. Check Service-Inventory Pod access for Service-Rates/parking or Service-Rates/review by execing into the pod, then curling each service.
   ```bash
   kubectl get pod
   ```
   ```
   NAME                                    READY   STATUS    RESTARTS   AGE
   inventory-ver1-7bb6989d9d-2p2hk         1/1     Running   0          7d13h
   inventory-ver1-7bb6989d9d-464rk         1/1     Running   0          7d13h
   parking-6cdcd5b4b4-bbzvt                1/1     Running   0          103m
   parking-6cdcd5b4b4-g8dkb                1/1     Running   0          103m
   review-5888566ff6-2plsj                 1/1     Running   0          101m
   review-5888566ff6-89fqk                 1/1     Running   0          101m
   ```
1. Exec into an inventory pod to check connectivity to parking and review services:
   ```bash
   kubectl exec -ti inventory-ver1-7bb6989d9d-2p2hk sh
   ```
1. From inside of the inventory pod, use `curl` to connect to the parking service (using the DNS Name from the previous `kubectl get httproute` command):
   ```bash
   curl rates-default-0d38139624f20d213.7d67968.vpc-lattice-svcs.us-west-2.on.aws/parking
   ```
   ```
   Requesting to Pod(parking-6cdcd5b4b4-g8dkb): parking handler pod
   ```
1. From inside of the pod, use `curl` to connect to the review service:
   ```bash
   curl rates-00422586e3362607e.7d67968.vpc-service-network-svcs.us-west-2.amazonaws.com/review 
   ```
   ```
   Requesting to Pod(review-5888566ff6-89fqk): review handler pod
   ```
1. Exit the pod:
   ```bash
   exit
   ```
1. Exec into a parking pod to check connectivity to the inventory-ver1 service:
   ```bash
   kubectl exec -ti parking-6cdcd5b4b4-bbzvt sh
   ```
1. From inside of the parking pod, use `curl` to connect to the inventory-ver1 service:
   ```bash
   curl inventory-default-02fb06f1acdeb5b55.7d67968.vpc-lattice-svcs.us-west-2.on.aws
   ```
   ```
   Requesting to Pod(inventory-ver1-7bb6989d9d-2p2hk): inventory-ver1 handler pod 
   ```
1. Exit the pod:
   ```bash
   exit
   ```

## Set up multi-cluster/multi-VPC service-to-service communications

This sections builds on the previous section by migrating a Kubernetes service (HTTPRoute inventory) from one Kubernetes cluster to a different Kubernetes cluster.
For example, it will:

* Migrate the Kubernetes inventory service from a Kubernetes v1.21 cluster to a Kubernetes v1.23 cluster in a different VPC.
* Scale up the Kubernetes inventory service to run it in another cluster (and another VPC) in addition to the current cluster.

The following figure illustrates this:

![Multiple clusters/VPCs service-to-service communications](images/example2.png)

**Steps**

   **Set up inventory on a second cluster** 

1. Create a second cluster (using the same instructions used to create the first).

1. Ensure you're using the second cluster profile. 
   ```bash
   kubectl config get-contexts 
   ```
   If your profile is set to the first cluster, switch your credentials to use the second cluster:
   ```bash
   kubectl config use-context <yourcluster2info>
   ```
1. Create a Kubernetes inventory-ver2 service in the second cluster:
   ```bash
   kubectl apply -f examples/inventory-ver2.yaml
   ```
1. Export this Kubernetes inventory-ver2 from the second cluster, so that it can be referenced by HTTPRoute in the other cluster:
   ```bash
   kubectl apply -f examples/inventory-ver2-export.yaml
   ```
   **Switch back to the first cluster**

1. Switch credentials back to the first cluster
   ```bash
   kubectl config use-context <yourcluster1info>
   ```
1. Import the Kubernetes inventory-ver2 into first cluster:
   ```bash
   kubectl apply -f examples/inventory-ver2-import.yaml
   ```
1. Update the HTTPRoute inventory to route 10% traffic to the first cluster and 90% traffic to the second cluster:
   ```bash
   kubectl apply -f examples/inventory-route-bluegreen.yaml
   ```
1. Check the Service-Rates/parking pod access to Service-Inventory by execing into the parking pod:
   ```bash
   kubectl exec -ti parking-6cdcd5b4b4-bbzvt sh
   ```
1. From inside of the pod, use `curl` to connect to the inventory service:
 
      ```bash
      for ((i=1;i<=30;i++)); do   curl   "inventory-default-0f89d8ff5e98400d0.7d67968.vpc-lattice-svcs.us-west-2.on.aws"; done
      ```
      ```
      Requsting to Pod(inventory-ver2-6dc74b45d8-rlnlt): Inventory-ver2 handler pod <----> in 2nd cluster
      Requsting to Pod(inventory-ver2-6dc74b45d8-rlnlt): Inventory-ver2 handler pod
      Requsting to Pod(inventory-ver2-6dc74b45d8-rlnlt): Inventory-ver2 handler pod
      Requsting to Pod(inventory-ver2-6dc74b45d8-rlnlt): Inventory-ver2 handler pod
      Requsting to Pod(inventory-ver2-6dc74b45d8-95rsr): Inventory-ver1 handler pod <----> in 1st cluster
      Requsting to Pod(inventory-ver2-6dc74b45d8-rlnlt): Inventory-ver2 handler pod
      Requsting to Pod(inventory-ver2-6dc74b45d8-95rsr): Inventory-ver2 handler pod
      Requsting to Pod(inventory-ver2-6dc74b45d8-95rsr): Inventory-ver2 handler pod
      Requsting to Pod(inventory-ver1-74fc59977-wg8br): Inventory-ver1 handler pod....
      ```
      You can see that the traffic is distributed between *inventory-ver1* and *inventory-ver2* as expected.
