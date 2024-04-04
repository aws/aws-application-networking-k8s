# Getting Started with AWS Gateway API Controller

This guide helps you get started using the controller.

Following this guide, you will:

- Set up service-to-service communications with VPC Lattice on a single cluster.
- Create another service on a second cluster in a different VPC, and route traffic to that service across the two clusters and VPCs.

Using these examples as a foundation, see the [Concepts](../concepts/overview.md) section for ways to further configure service-to-service communications.

## Prerequisites

Before proceeding to the next sections, you need to:

- Create a cluster `gw-api-controller-demo` with the controller following the [AWS Gateway API Controller installation guide on Amazon EKS](deploy.md/#deploy-the-controller-on-amazon-eks).
- Clone the [AWS Gateway API Controller](https://github.com/aws/aws-application-networking-k8s) repository.

    ```bash linenums="1"
    git clone https://github.com/aws/aws-application-networking-k8s.git
    cd aws-application-networking-k8s
    ```
- Set the AWS Region of your cluster.
    ```
    export AWS_REGION=<cluster_region>
    ```

## Single cluster

This example creates a single cluster in a single VPC, then configures two HTTPRoutes (`rates` and `inventory`) and three kubetnetes services (`parking`, `review`, and `inventory-1`). The following figure illustrates this setup:

![Single cluster/VPC service-to-service communications](../images/example1.png)

**Setup in-cluster service-to-service communications**

1. AWS Gateway API Controller needs a VPC Lattice Service Network to operate.

    === "Helm"
        If you installed the controller with `helm`, you can update chart configurations by specifying the `defaultServiceNetwork` variable:

        ```bash linenums="1"
        aws ecr-public get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin public.ecr.aws
        helm upgrade gateway-api-controller \
        oci://public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller-chart \
        --version=v1.0.4 \
        --reuse-values \
        --namespace aws-application-networking-system \
        --set=defaultServiceNetwork=my-hotel 
        ```

    === "AWS CLI"

        You can use AWS CLI to manually create a VPC Lattice Service Network association to the previously created `my-hotel` Service Network:

        ```bash linenums="1"
        aws vpc-lattice create-service-network --name my-hotel
        SERVICE_NETWORK_ID=$(aws vpc-lattice list-service-networks --query "items[?name=="\'my-hotel\'"].id" | jq -r '.[]')
        CLUSTER_VPC_ID=$(aws eks describe-cluster --name $CLUSTER_NAME | jq -r .cluster.resourcesVpcConfig.vpcId)
        aws vpc-lattice create-service-network-vpc-association --service-network-identifier $SERVICE_NETWORK_ID --vpc-identifier $CLUSTER_VPC_ID
        ```

        Ensure the service network created above is ready to accept traffic from the new VPC, by checking if the VPC association status is `ACTIVE`:
    
        ```bash 
        aws vpc-lattice list-service-network-vpc-associations --vpc-id $CLUSTER_VPC_ID
        ```
        ``` json hl_lines="5"
        {
            "items": [
                {
                    ...
                    "status": "ACTIVE",
                    ...
                }
            ]
        }
        ```

1. Create the Kubernetes Gateway `my-hotel`:

    ```bash 
    kubectl apply -f files/examples/my-hotel-gateway.yaml
    ```

    Verify that `my-hotel` Gateway is created with `PROGRAMMED` status equals to `True`:

    ```bash 
    kubectl get gateway
    ```
    ```
    NAME       CLASS                ADDRESS   PROGRAMMED   AGE
    my-hotel   amazon-vpc-lattice               True      7d12h
    ```

1. Create the Kubernetes HTTPRoute `rates` that can has path matches routing to the `parking` service and `review` service:
   ```bash linenums="1"
   kubectl apply -f files/examples/parking.yaml
   kubectl apply -f files/examples/review.yaml
   kubectl apply -f files/examples/rate-route-path.yaml
   ```
1. Create another Kubernetes HTTPRoute `inventory`:
   ```bash linenums="1"
   kubectl apply -f files/examples/inventory-ver1.yaml
   kubectl apply -f files/examples/inventory-route.yaml
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

1. Check VPC Lattice generated DNS address for HTTPRoute `inventory` and `rates` **(this could take up to one minute to populate)**:

    ```bash 
    kubectl get httproute inventory -o yaml 
    ```
    ``` hl_lines="5"
        apiVersion: gateway.networking.k8s.io/v1beta1
        kind: HTTPRoute
        metadata:
            annotations:
            application-networking.k8s.aws/lattice-assigned-domain-name: inventory-default-xxxxxx.xxxxx.vpc-lattice-svcs.us-west-2.on.aws
        ...
    ```

    ```bash
    kubectl get httproute rates -o yaml
    ```
    ``` hl_lines="7"
        apiVersion: gateway.networking.k8s.io/v1beta1
        kind: HTTPRoute
        metadata:
            annotations:
            application-networking.k8s.aws/lattice-assigned-domain-name: rates-default-xxxxxx.xxxxxxxxx.vpc-lattice-svcs.us-west-2.on.aws
        ...
    ```

1. If the previous step returns the expected response, store VPC Lattice assigned DNS names to variables.

    ```bash linenums="1"
    ratesFQDN=$(kubectl get httproute rates -o json | jq -r '.metadata.annotations."application-networking.k8s.aws/lattice-assigned-domain-name"')
    inventoryFQDN=$(kubectl get httproute inventory -o json | jq -r '.metadata.annotations."application-networking.k8s.aws/lattice-assigned-domain-name"')
    ```

    Confirm that the URLs are stored correctly:

    ```bash
    echo "$ratesFQDN \n$inventoryFQDN"
    ```
    ```
    rates-default-xxxxxx.xxxxxxxxx.vpc-lattice-svcs.us-west-2.on.aws 
    inventory-default-xxxxxx.xxxxx.vpc-lattice-svcs.us-west-2.on.aws
    ```

**Verify service-to-service communications**

1. Check connectivity from the `inventory-ver1` service to `parking` and `review` services:

    ```bash 
    kubectl exec deploy/inventory-ver1 -- curl -s $ratesFQDN/parking $ratesFQDN/review
    ```

    ```
    Requsting to Pod(parking-xxxxx): parking handler pod
    Requsting to Pod(review-xxxxxx): review handler pod
    ```

1. Check connectivity from the `parking` service to the `inventory-ver1` service:
   ```bash 
   kubectl exec deploy/parking -- curl -s $inventoryFQDN
   ```
   ```
   Requsting to Pod(inventory-xxx): Inventory-ver1 handler pod
   ```
   Now you could confirm the service-to-service communications within one cluster is working as expected.

## Multi-cluster

This section builds on the previous one. We will be migrating the Kubernetes `inventory` service from a the EKS cluster we previously created to new cluster in a different VPC, located in the same AWS Account.

![Multiple clusters/VPCs service-to-service communications](../images/example2.png)

**Set up `inventory-ver2` service and serviceExport in the second cluster**

!!! warning AWS Region
    VPC Lattice is a regional service, so you will need to create this second cluster in the same AWS Region as `gw-api-controller-demo`. Keep this in mind when setting `AWS_REGION` variable in the following steps.
    ```
    export AWS_REGION=<clusters_region>
    ```

1. Create a second Kubernetes cluster `gw-api-controller-demo-2` with the Controller installed (using the [same instructions to create and install the controller on Amazon EKS](deploy.md/#deploy-the-controller-on-amazon-eks) used to create the first). 

2. For sake of simplicity, lets set some alias for our clusters in the Kubernetes config file.

    ```bash linenums="1"
    aws eks update-kubeconfig --name gw-api-controller-demo --region $AWS_REGION --alias gw-api-controller-demo
    aws eks update-kubeconfig --name gw-api-controller-demo-2 --region $AWS_REGION --alias gw-api-controller-demo-2
    kubectl config get-contexts
    ```


1. Ensure you're using the second cluster's `kubectl` context.
    ```bash 
    kubectl config current-context
    ```
   If your context is set to the first cluster, switch it to use the second cluster one:
    ```bash 
    kubectl config use-context gw-api-controller-demo-2
    ```
1. Create the Service Network association.

    === "Helm"
        If you installed the controller with `helm`, you can update chart configurations by specifying the `defaultServiceNetwork` variable:

        ```bash linenums="1"
        aws ecr-public get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin public.ecr.aws
        helm upgrade gateway-api-controller \
        oci://public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller-chart \
        --version=v1.0.4 \
        --reuse-values \
        --namespace aws-application-networking-system \
        --set=defaultServiceNetwork=my-hotel 
        ```

    === "AWS CLI"

        You can use AWS CLI to manually create a VPC Lattice Service Network association to the previously created `my-hotel` Service Network:

        ```bash linenums="1"
        SERVICE_NETWORK_ID=$(aws vpc-lattice list-service-networks --query "items[?name=="\'my-hotel\'"].id" | jq -r '.[]')
        CLUSTER_VPC_ID=$(aws eks describe-cluster --name $CLUSTER_NAME | jq -r .cluster.resourcesVpcConfig.vpcId)
        aws vpc-lattice create-service-network-vpc-association --service-network-identifier $SERVICE_NETWORK_ID --vpc-identifier $CLUSTER_VPC_ID
        ```

        Ensure the service network created above is ready to accept traffic from the new VPC, by checking if the VPC association status is `ACTIVE`:
    
        ```bash 
        aws vpc-lattice list-service-network-vpc-associations --vpc-id $CLUSTER_VPC_ID
        ```
        ``` json hl_lines="5"
        {
            "items": [
                {
                    ...
                    "status": "ACTIVE",
                    ...
                }
            ]
        }
        ```

1. Create the Kubernetes Gateway `my-hotel`:

    ```bash 
    kubectl apply -f files/examples/my-hotel-gateway.yaml
    ```

    Verify that `my-hotel` Gateway is created with `PROGRAMMED` status equals to `True`:

    ```bash 
    kubectl get gateway
    ```
    ```
    NAME       CLASS                ADDRESS   PROGRAMMED   AGE
    my-hotel   amazon-vpc-lattice               True      7d12h
    ```

1. Create a Kubernetes inventory-ver2 service in the second cluster:
    ```bash 
    kubectl apply -f files/examples/inventory-ver2.yaml
    ```
1. Export this Kubernetes inventory-ver2 from the second cluster, so that it can be referenced by HTTPRoute in the first cluster:
    ```bash 
    kubectl apply -f files/examples/inventory-ver2-export.yaml
    ```

   **Switch back to the first cluster**

1. Switch context back to the first cluster
    ```bash 
    kubectl config use-context gw-api-controller-demo
    ```
1. Create Kubernetes ServiceImport `inventory-ver2` in the first cluster:
    ```bash 
    kubectl apply -f files/examples/inventory-ver2-import.yaml
    ```
1. Update the HTTPRoute `inventory` rules to route 10% traffic to the first cluster and 90% traffic to the second cluster:
    ```bash 
    kubectl apply -f files/examples/inventory-route-bluegreen.yaml
    ```
1. Check the service-to-service connectivity from `parking`(in the first cluster) to `inventory-ver1`(in in the first cluster) and `inventory-ver2`(in in the second cluster):
    ```bash 
    inventoryFQDN=$(kubectl get httproute inventory -o json | jq -r '.metadata.annotations."application-networking.k8s.aws/lattice-assigned-domain-name"')
    kubectl exec deploy/parking -- sh -c 'for ((i=1; i<=30; i++)); do curl -s "$0"; done' "$inventoryFQDN"
    ```

    ```
    Requesting to Pod(inventory-ver2-6dc74b45d8-rlnlt): Inventory-ver2 handler pod <----> in 2nd cluster
    Requesting to Pod(inventory-ver2-6dc74b45d8-rlnlt): Inventory-ver2 handler pod
    Requesting to Pod(inventory-ver2-6dc74b45d8-rlnlt): Inventory-ver2 handler pod
    Requesting to Pod(inventory-ver2-6dc74b45d8-rlnlt): Inventory-ver2 handler pod
    Requesting to Pod(inventory-ver2-6dc74b45d8-95rsr): Inventory-ver1 handler pod <----> in 1st cluster
    Requesting to Pod(inventory-ver2-6dc74b45d8-rlnlt): Inventory-ver2 handler pod
    Requesting to Pod(inventory-ver2-6dc74b45d8-95rsr): Inventory-ver2 handler pod
    Requesting to Pod(inventory-ver2-6dc74b45d8-95rsr): Inventory-ver2 handler pod
    Requesting to Pod(inventory-ver1-74fc59977-wg8br): Inventory-ver1 handler pod....
    ```

    You can see that the traffic is distributed between `inventory-ver1` and `inventory-ver2` as expected.

## Cleanup

To avoid additional charges, remove the demo infrastructure from your AWS account.


**Multi-cluster**

Delete resources in the Multi-cluster walkthrough.

!!! Warning Set your AWS Region
    Remember that you need to have the AWS Region set.
    ```bash
    export AWS_REGION=<cluster_region>
    ```

1. Cleanup VPC Lattice Service and Service Import in `gw-api-controller-demo` cluster:
```bash
kubectl config use-context gw-api-controller-demo
kubectl delete -f files/examples/inventory-route-bluegreen.yaml
kubectl delete -f files/examples/inventory-ver2-import.yaml
```

1. Delete Service Export and applications in `gw-api-controller-demo-2` cluster:
```bash
kubectl config use-context gw-api-controller-demo-2
kubectl delete -f files/examples/inventory-ver2-export.yaml
kubectl delete -f files/examples/inventory-ver2.yaml
```

1. Delete the Service Network Association **(this could take up to one minute)**:
```bash
CLUSTER_NAME=gw-api-controller-demo-2
CLUSTER_VPC_ID=$(aws eks describe-cluster --name $CLUSTER_NAME | jq -r .cluster.resourcesVpcConfig.vpcId)
SERVICE_NETWORK_ASSOCIATION_IDENTIFIER=$(aws vpc-lattice list-service-network-vpc-associations --vpc-id $CLUSTER_VPC_ID --query "items[?serviceNetworkName=="\'my-hotel\'"].id" | jq -r '.[]')
aws vpc-lattice delete-service-network-vpc-association  --service-network-vpc-association-identifier $SERVICE_NETWORK_ASSOCIATION_IDENTIFIER
```


**Single cluster**

Delete resources in the Single cluster walkthrough.

!!! Warning Set your AWS Region
    Remember that you need to have the AWS Region set.
    ```bash
    export AWS_REGION=<cluster_region>
    ```


1. Delete VPC Lattice Services adnd applications in `gw-api-controller-demo` cluster:
```bash
kubectl config use-context gw-api-controller-demo
kubectl delete -f files/examples/inventory-route.yaml
kubectl delete -f files/examples/inventory-ver1.yaml
kubectl delete -f files/examples/rate-route-path.yaml
kubectl delete -f files/examples/parking.yaml
kubectl delete -f files/examples/review.yaml
kubectl delete -f files/examples/my-hotel-gateway.yaml
```

1. Delete the Service Network Association **(this could take up to one minute)**:
```bash
CLUSTER_NAME=gw-api-controller-demo
CLUSTER_VPC_ID=$(aws eks describe-cluster --name $CLUSTER_NAME | jq -r .cluster.resourcesVpcConfig.vpcId)
SERVICE_NETWORK_ASSOCIATION_IDENTIFIER=$(aws vpc-lattice list-service-network-vpc-associations --vpc-id $CLUSTER_VPC_ID --query "items[?serviceNetworkName=="\'my-hotel\'"].id" | jq -r '.[]')
aws vpc-lattice delete-service-network-vpc-association  --service-network-vpc-association-identifier $SERVICE_NETWORK_ASSOCIATION_IDENTIFIER
```

**Cleanup VPC Lattice Resources**

1. Cleanup controllers in `gw-api-controller-demo` and `gw-api-controller-demo-2` clusters:

    === "Helm"

        ```bash 
        kubectl config use-context gw-api-controller-demo
        CLUSTER_NAME=gw-api-controller-demo
        aws ecr-public get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin public.ecr.aws
        helm uninstall gateway-api-controller --namespace aws-application-networking-system 
        kubectl config use-context gw-api-controller-demo-2
        CLUSTER_NAME=gw-api-controller-demo-2
        aws ecr-public get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin public.ecr.aws
        helm uninstall gateway-api-controller --namespace aws-application-networking-system 
        ```
    
    === "Kubectl"
        ```bash 
        kubectl config use-context gw-api-controller-demo
        kubectl delete -f https://raw.githubusercontent.com/aws/aws-application-networking-k8s/main/files/controller-installation/deploy-v1.0.4.yaml
        kubectl config use-context gw-api-controller-demo-2
        kubectl delete -f https://raw.githubusercontent.com/aws/aws-application-networking-k8s/main/files/controller-installation/deploy-v1.0.4.yaml
        ```

1. Delete the Service Network:

    1. Ensure the Service Network Associations have been deleted (do not move forward if the deletion is still `IN PROGRESS`):
    ```bash
    SN_IDENTIFIER=$(aws vpc-lattice list-service-networks --query "items[?name=="\'my-hotel\'"].id" | jq -r '.[]')
    aws vpc-lattice list-service-network-vpc-associations --service-network-identifier $SN_IDENTIFIER
    ```
    1. Delete `my-hotel` Service Network:
    ```bash 
    aws vpc-lattice delete-service-network --service-network-identifier $SN_IDENTIFIER
    ```
    1. Ensure the Service Network `my-hotel` is deleted:
    ```bash 
    aws vpc-lattice list-service-networks
    ```

**Cleanup the clusters**

Finally, remember to delete the clusters you created for this walkthrough:

1. Delete `gw-api-controller-demo` cluster:
```bash 
eksctl delete cluster --name=gw-api-controller-demo
k config delete-context gw-api-controller-demo
```

1. Delete `gw-api-controller-demo-2` cluster:
```bash
eksctl delete cluster --name=gw-api-controller-demo-2
k config delete-context gw-api-controller-demo-2
```
