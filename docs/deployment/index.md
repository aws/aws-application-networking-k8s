# AWS Gateway API Controller User Guide

The AWS Gateway API Controller lets you connect services across multiple Kubernetes clusters, EC2 instances, containers, and serverless functions through a Gateway API interface.
It does this by leveraging AWS VPC Lattice, which handles the connections to the AWS infrastructure, and Kubernetes Gateway API calls to manage Kubernetes objects. 

This document describes how to set up the AWS Gateway API Controller and provides example use cases.
With the controller deployed and running, you will be able to manage services for multiple Kubernetes clusters and other targets on AWS through the following:

* **CLI**: Use aws and eksctl to create clusters and set up VPC Lattice, then kubectl and YAML files to set up Kubernetes objects.
* **AWS Console**: View VPC Lattice assets through the VPC area of the AWS console.

While separating the application developer from the details of the underling infrastructure, the controller also provides a Kubernetes-native experience, rather than creating a lot of new AWS ways of managing services.
It does this by integrating with the Kubernetes Gateway API.
This lets you work with Kubernetes service-related resources using Kubernetes APIs and custom resource definitions (CRDs).

For more information on this technology, see [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/). 


## Deploying the Gateway API Controller


1. TODO: Is any setup needed similar to adding someone to the Mercury beta allow list? See [README](https://code.amazon.com/packages/VpcServiceNetworkPreviewConstructs/blobs/mainline/--/README.md). Something like this?:

   ```bash
   ./vpc-service-network-role.sh -a yourAccountId -p isengard -r Admin
   ```
1. TODO: Steps for deploying the controller need to include the new [helm chart](https://github.com/aws/aws-application-networking-k8s/pull/35).
   TODO: See [IRSA instructions](https://aws-controllers-k8s.github.io/community/docs/user-docs/irsa/).

With the Gateway API Controller deployed, create one or two clusters, depending on which examples you want to try.

### Prepare the first cluster

1. Create an EKS cluster
   ```bash
   eksctl create cluster —name vpc-lattice-eks-test-1 —region us-west-2
   ```
1. TODO: I don't know how to do this step (link?): TODO: Also, Liwen said to say "Lattice-managed prefix" instead of 169.254.0.0/16. Configure security group: To receive traffic from the VPC Lattice fleet, all Pods MUST explicit configure a security group to allow traffic from the 169.254.0.0/16 address range.

   TODO: Jay says the next two steps can be removed since the Gateway API Controller will handle these.

1. Install Kubernetes Gateway API objects in this EKS cluster:
   TODO: What is location of these yaml files (tarball or ECR registry)?
   ```bash
   kubectl apply -f k8s-crd/k8s-gateway-v1alpha2.yaml
   kubectl apply -f k8s-crd/multicluster.x-k8s.io_serviceexports.yaml
   kubectl apply -f k8s-crd/multicluster.x-k8s.io_serviceimports.yaml
   ```

1. Verify that the Kubernetes multi-cluster API objects are installed correctly:
   ```bash

   TODO: Jay says the next two steps can be removed since the Gateway API Controller will handle these.
   kubectl get crd
   ```
   ```
   NAME                                        CREATED AT
   eniconfigs.crd.k8s.amazonaws.com            2021-11-09T03:14:21Z
   gatewayclasses.gateway.networking.k8s.io    2021-11-09T03:26:32Z <------> gatewayclass
   gateways.gateway.networking.k8s.io          2021-11-09T03:26:32Z <------> gateway
   httproutes.gateway.networking.k8s.io        2021-11-09T03:26:32Z <------> httproute
   referencepolicies.gateway.networking.k8s.io 2021-11-09T03:26:33Z
   securitygrouppolicies.vpcresources.k8s.aws  2021-11-09T03:14:24Z
   serviceexports.multicluster.x-k8s.io        2021-11-09T03:57:44Z <------> serviceexport
   serviceimports.multicluster.x-k8s.io        2021-11-09T03:57:52Z <------> serviceimport
   tcproutes.gateway.networking.k8s.io         2021-11-09T03:26:33Z
   tlsroutes.gateway.networking.k8s.io         2021-11-09T03:26:33Z
   udproutes.gateway.networking.k8s.io         2021-11-09T03:26:33Z
   ```
1. Create an IAM OIDC provider: See [Creating an IAM OIDC provider for your cluster](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html) for details.
   ```bash
   eksctl utils associate-iam-oidc-provider --cluster <my-cluster> --approve
   ```
1. Create a policy in IAM that can invoke the gateway API and copy the policy arn for later use:
   ```bash
   aws iam create-policy \
      --policy-name AWSMercuryControllerIAMPolicy \
      --policy-document file://iam-policy.json
   ```
1. Create a namespace called system:
   ```bash
    kubectl apply -f deploy_namespace.yaml
   ```
1. Create an iamserviceaccount for pod level permission:
   ```bash
   eksctl create iamserviceaccount \
      --cluster=<my-cluster-name> \
      --namespace=system \
      --name=gateway-api-controller \
      --attach-policy-arn=<AWSMercuryControllerIAMPolicy ARN CREATED IN CREATE_POLICY STEP> \
      --override-existing-serviceaccounts \
      --region us-west-2 \
      --approve
   ```
1. If your cluster does NOT allow Pods to access instance IMDS, you need to modify ACCOUNT_ID and CLUSTER_VPC_ID in the last DEPLOYMENT Section of the `deploy.yaml` file. Then deploy the pod running the controller to your Kubernetes cluster:
   ```bash
    kubectl apply -f deploy.yaml
   ```
1. Create Kubernetes Gateway Class for VPC Lattice. All of your services, routes, and targets associated with the controller are organized under this gateway class: 
   ```bash
    kubectl apply -f gatewayclass.yaml
   ```
1. Verify that a new gatewayclass is created:
   ```bash
    kubectl get gatewayclass
   ```
   ```
    NAME          CONTROLLER                               AGE
    aws-lattice   lattice.k8s.aws/lattice-k8s-controller   7d12h
   ```

### Prepare the second cluster
To try the second example in this document, you need to create a second EKS cluster as described here:

1. Create a second EKS cluster:
   ```bash
   eksctl create cluster —name vpc-lattice-eks-test-2 —region us-west-2
   ```
1. Make  sure your security groups are configured to allow 169.254.0.0/16 to all pods in second cluster.
1. Install the Kubernetes Gateway APIs objects in the second cluster:
   ```bash
   kubectl apply -f k8s-crd/k8s-gateway-v1alpha2.yaml
   kubectl apply -f k8s-crd/multicluster.x-k8s.io_serviceexports.yaml
   kubectl apply -f k8s-crd/multicluster.x-k8s.io_serviceimports.yaml
   ```
1. Create an IAM OIDC provider (see Creating an IAM OIDC provider for your cluster (http://%20https//docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html)):
   ```bash
   eksctl utils associate-iam-oidc-provider --cluster <my-cluster> --approve
   ```
1. Create a policy in IAM that can invoke the VPC Lattice API and copy the policy arn for later use:
   ```bash
   aws iam create-policy \
       --policy-name AWSMercuryControllerIAMPolicy \
       --policy-document file://iam-policy.json
   ```
1. Create a namespace called system
   ```bash
   kubectl apply -f deploy_namespace.yaml
   ```
1. Create iamserviceaccount for pod level permission
   ```bash
   eksctl create iamserviceaccount \
     --cluster=<my-cluster-name> \
     --namespace=system \
     --name=controller-manager \
     --attach-policy-arn=<AWSMercuryControllerIAMPolicy ARN CREATED IN STEP 2> \
     --override-existing-serviceaccounts \
     --region us-west-2 \
     --approve
   ```
1. If your cluster does NOT allow Pod to access instance IMDS, then you need to modify ACCOUNT_ID and CLUSTER_VPC_ID in the last DEPLOYMENT Section of the deploy.yaml file.
1. Deploy MercuryK8SController into your Kubernetes cluster:
   ```bash
   kubectl apply -f deploy.yaml
   ```
## Using the Gateway API Controller

The test cases in this section cover two examples of service-to-service communications.
The first shows two services inside the same VPC and the other shows two services associated with two different clusters, each in different VPCs.

### Example 1:  Single-cluster/VPC service-to-service communications

This example creates a single cluster in a single VPC, then configures two routes (rates and inventory) and three services (parking, review, and inventory-1). The following figure illustrates this setup:

[Image: example1.png]

**Steps**

1. Create the Kubernetes Gateway `my-hotel`:
   ```bash
   kubectl apply -f my-hotel-gateway.yaml
   ```
1. Verify that `my-hotel` gateway is created:
   ```bash
   kubectl get gateway  
   ```
   ```
   NAME       CLASS         ADDRESS   READY   AGE
   my-hotel   aws-lattice                     7d12h
   ```
1. Find the VPC Lattice service network:
   ```bash
   kubectl get gateway my-hotel -o yaml
   ```
   ```
   apiVersion: gateway.networking.k8s.io/v1alpha2
   kind: Gateway
   ...
   status:
   conditions:
   message: 'aws-gateway-arn: arn:aws:vpc-service-network:us-west-2:694065802095:mesh/mesh-0d01b22a156d2cc2f' 
   reason: Reconciled
   status: "True"
   ```
1. Create the Kubernetes HTTPRoute rates 
   ```
   # create k8s service parking
   ```
   ```bash
   kubectl apply -f parking.yaml
   ```
   ```
   # create k8s service review
   ```
   ```bash
   kubectl apply -f review.yaml
   ```
   ```
   # create K8S HTTPRoute rate
   ```
   ```bash
   kubectl apply -f rate-route-path.yaml
   ```
1. Create the Kubernetes HTTPRoute inventory
   ```
   # create K8S service inventory-ver1
   ```
   ```bash
   kubectl apply -f inventory-ver1.yaml
   ```
   ```
   # create K8S HTTPRoute inventory
   ```
   ```bash
   kubectl apply -f inventory-route.yaml
   ```
1. Find out HTTPRoute's DNS name from HTTPRoute status:
   ```bash
   kubectl get httproute
   ```
   ```
   NAME                     HOSTNAMES   AGE
   aug24-banking                        15h
   aug24-inventory-weight               20d
   aug24-rates                          12d
   ```
1. List the route’s yaml file to see the DNS address:
   ```bash
   kubectl get httproute aug24-rates -o yaml
   ```
   ```
   apiVersion: gateway.networking.k8s.io/v1alpha2 (http://gateway.networking.k8s.io/v1alpha2)
   kind: HTTPRoute
   metadata:
     annotations:    kubectl.kubernetes.io/last-applied-configuration (http://kubectl.kubernetes.io/last-applied-configuration):
   ...
   status:
     parents:
     - conditions:
     - lastTransitionTime: "2022-09-14T03:57:32Z"
       message: 'DNS Name: aug2-rates-...vpc-service-network-svcs.us-west-2.amazonaws.com'
   ...
   ```
1. During preview, you are required to install the VPC Lattice CLI:
   ```bash
   aws configure add-model --service-model file://api-2.json (file://api-2.json/) —service-name ec2-lattice
   ```
1. Use the VPC Lattice CLI to find the DNS name. You can use the `curl` command to get information about each service by adding the service name to the end of the HTTPRoute DNS name. Those names are gathered from AWS Route53 instead of Kubernetes CoreDNS.
   ```bash
   aws ec2-lattice list-services \
     --endpoint-url=https://vpc-service-network.us-west-2.amazonaws.com (https://vpc-service-network.us-west-2.amazonaws.com/)
   ```
   ```
   {
       "items": [
           {
               "status": "ACTIVE", 
               "name": "sept6-rates", 
               "lastUpdatedAt": "2022-09-07T03:58:50.646Z", 
               "arn": "arn:aws:vpc-service-network:us-west-2:694065802095:service/svc-00422586e3362607e", 
               "id": "svc-00422586e3362607e", 
               "createdAt": "2022-09-07T03:58:23.528Z"
           }, 
           {
               "status": "ACTIVE", 
               "name": "sept6-inventory", 
               "lastUpdatedAt": "2022-09-07T04:12:33.518Z", 
               "arn": "arn:aws:vpc-service-network:us-west-2:694065802095:service/svc-0cd1a223d518754f3", 
               "id": "svc-0cd1a223d518754f3", 
               "createdAt": "2022-09-07T04:12:06.857Z"
           }, 
           ...
    }       
   ```
   ```
   # find out the DNS name using service ID
   ```
   ```bash
   aws ec2-lattice get-service --service-identifier svc-0cd1a223d518754f3 \
      --endpoint-url=https://vpc-service-network.us-west-2.amazonaws.com (https://vpc-service-network.us-west-2.amazonaws.com/)
   ```
   ```
   {
       "status": "ACTIVE", 
       "name": "sept6-inventory", 
       "lastUpdatedAt": "2022-09-07T04:12:33.518Z", 
       "createdAt": "2022-09-07T04:12:06.857Z", 
       "dns": {
           "name": "sept6-inventory-0cd1a223d518754f3.7d67968.vpc-service-network-svcs.us-west-2.amazonaws.com"
       }, 
       "id": "svc-0cd1a223d518754f3", 
       "arn": "arn:aws:vpc-service-network:us-west-2:694065802095:service/svc-0cd1a223d518754f3"
   }
    
   ```
   ```bash
   aws ec2-lattice get-service --service-identifier svc-00422586e3362607e \
      --endpoint-url=https://vpc-service-network.us-west-2.amazonaws.com (https://vpc-service-network.us-west-2.amazonaws.com/)
   ```
   ```
   {
       "status": "ACTIVE", 
       "name": "sept6-rates", 
       "lastUpdatedAt": "2022-09-07T03:58:50.646Z", 
       "createdAt": "2022-09-07T03:58:23.528Z", 
       "dns": {
           "name": "sept6-rates-00422586e3362607e.7d67968.vpc-service-network-svcs.us-west-2.amazonaws.com"
       }, 
       "id": "svc-00422586e3362607e", 
       "arn": "arn:aws:vpc-service-network:us-west-2:694065802095:service/svc-00422586e3362607e"
   }
   ```
1. Traffic: Service-Inventory Pod access Service-Rates/parking or Service-Rates/review: Make sure security group is configured to allow 169.254.0.0/16 to receive traffic from lattice fleet.
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
   ```bash
   kubectl exec -ti inventory-ver1-7bb6989d9d-2p2hk sh
   ```
   ```
   sh-4.2#
   ```
   ```bash
   curl sept6-rates-00422586e3362607e.7d67968.vpc-service-network-svcs.us-west-2.amazonaws.com/parking 
   ```
   ```
   Requesting to Pod(parking-6cdcd5b4b4-g8dkb): parking handler pod
   ```
   ```bash
   curl sept6-rates-00422586e3362607e.7d67968.vpc-service-network-svcs.us-west-2.amazonaws.com/review 
   ```
   ```
   Requesting to Pod(review-5888566ff6-89fqk): review handler pod
   ```
1. Traffic: Service-Rates/parking pod access Service-Inventory
   ```bash
   kubectl exec -ti parking-6cdcd5b4b4-bbzvt sh
   ```
   ```
   Requesting to Pod(inventory-ver1-7bb6989d9d-2p2hk): inventory-ver1 handler pod
   ```
### Example 2:  Multi-cluster/VPC service-to-service communications

This example migrates a Kubernetes service (HTTPRoute inventory) from one Kubernetes cluster to a different Kubernetes cluster.
For example, it will:

* Migrate the Kubernetes inventory service from a Kubernetes v1.19 cluster to a different Kubernetes v1.21 cluster.
* Scale up the Kubernetes inventory service to run it in another cluster (and another VPC) in addition to the current cluster.

The following figure illustrates this:

[Image: example2.png]

**Steps**

1. Create a Kubernetes inventory in the second cluster:
   ```bash
   kubectl apply -f inventory-ver2.yaml
   ```
1. Export this Kubernetes inventory-ver2 from the second cluster, so that it can be referenced by HTTPRoute in the other cluster:
   ```bash
   kubectl apply -f inventory-ver2-export.yaml
   ```
1. Import the Kubernetes inventory-ver2 into first cluster (Note: if you only have a single clouddesktop):
   ```
   # switch to cluster1  
   ```
   ```bash
   kubectl config use-context yourcluster2info
   kubectl apply -f inventory-ver2-import.yaml
   ```
1. Update the HTTPRoute inventory to route 90% traffic to the first cluster and 10% traffic to the second cluster:
   ```bash
   kubectl apply -f inventory-route-bluegreen.yaml
   ```
1. Traffic: Service-Rates/parking pod access Service-Inventory
   ```bash
   kubectl exec -ti parking-6cdcd5b4b4-bbzvt sh
   ```
   ```
   sh-4.2# 
   ```
   ```bash
   curl sept6-inventory-0cd1a223d518754f3.7d67968.vpc-service-network-svcs.us-west-2.amazonaws.com
   ```
   ```
   Requsting to Pod(inventory-ver1-7bb6989d9d-2p2hk): inventory-ver1 handler pod <----> in 1st cluster
   ```
   ```bash
   curl sept6-inventory-0cd1a223d518754f3.7d67968.vpc-service-network-svcs.us-west-2.amazonaws.com
   ```
   ```
   Requsting to Pod(inventory-ver2-7bb6989d9d-2p2hk): inventory-ver2 handler pod <----> in 2nd cluster
   ```

## Understanding the Gateway API Controller

For medium and large-scale customers, applications can often spread across multiple areas of a cloud.
For example, information pertaining to a company’s authentication, billing, and inventory may each be stored by services running on different VPCs in AWS.
Someone wanting to run an application that is spread out in this way might find themselves having to work with multiple ways to configure:

* Authentication and authorization
* Observability
* Service discovery
* Network connectivity and traffic routing

This is not a new problem.
A common approach to interconnecting services that span multiple VPCs is to use service meshes, such as Istio or AWS App Mesh. But these require sidecars, which can introduce scaling problems and present their own management challenges.  

If you just want to run an application, you should be shielded from details needed to find assets across what are essentially multiple virtual data centers (represented by multiple VPCs). You should also have consistent ways of working with assets across your VPCs, even if those assets include different combinations of instances, clusters, containers, and serverless. And while making it simpler to run multi-VPC applications easier for users, administrators still need the tools to control and audit their resources to suit their company’s compliance needs.

### Service Directory, Networks, Policies and Gateways

The goal of VPC Lattice is to provide a way to have a single, over-arching services view of all services across multiple VPCs.
The components making up that view include:

* Service Directory: This is an account-level directory for gathering your services in once place.
This can provide a view from the VPC Lattice section of the AWS console into all the services you own, as well as services that are shared with you.
A service might direct traffic to a particular service type (such as HTTP) and port (such as port 80).
However, using different rules, a request for the service could be sent to different targets such as a Kubernetes pod or a Lambda function, based on path or query string parameter.

* Service Network: Because applications might span multiple VPCs and accounts, there is a need to create networks that span those items.
  These networks let you register services to run across accounts and VPCs.
  You can create common authorization rules to simplify connectivity.
* Service Policies: You can build service policies to configure observability, access, and traffic management across any service network or gateway.
  You configure rules for handling traffic and for authorizing access.
  For now, you can assign IAM roles to allow certain requests.
  These are similar to S3 or IAM resource policies.
  Overall, this provides a common way to apply access rules at the service or service network levels.

* Service Gateway: This feature is not yet implemented.
  It is meant to centralize management of ingress and egress gateways.
  The Service Gateway will also let you manage access to external dependencies and clients using a centrally managed VPC.

If all goes well, you should be able to achieve some of the following goals:

* Kubernetes multi-cluster connectivity: Say that you have multiple clusters across multiple VPCs.
  After configuring your services with the AWS Gateway API, you can facilitate communications between services on those clusters without dealing with the underlying infrastructure.
  VPC Lattice handles a lot of the details for you without needing things like sidecars.
* Serverless access: VPC Lattice allows access to serverless features, as well as Kubernetes cluster features.
  This gives you a way to have a consistent interface to multiple types of platforms.

With VPC Lattice you can also avoid some of these common problems:

* Overlapping IP addresses: Even with well-managed IP addresses, overlapping address use can occur by mistake or when organizations or companies merge together.
  IP address conflicts can also occur if you wanted to manage resources across multiple Kubernetes clusters.
* Sidecar management: Changes to sidecars might require those sidecars to be reconfigured or rebooted.
  While this might not be a big issue for a handful of sidecars, it can be disruptive if you have thousands of pods, each with its own sidecar.

### Relationship between VPC Lattice and Kubernetes

As a Kubernetes user, you can have a very Kubernetes-native experience using the VPC Lattice APIs.
The following figure illustrates how VPC Lattice object connect to [Kubernetes Gateway API]((https://gateway-api.sigs.k8s.io/) objects:

TODO: Replace with new figure from end of this slide deck: https://amazon.awsapps.com/workdocs/index.html#/document/6398b63682b6fae1ac462edde9af07acc45014557df1dd92b32ccc2c6a744de5
[Image: VPCLatticeToKubernetesGatewayAPI.png]

As shown in the figure, there are different personas associated with different levels of control in VPC Lattice.
Notice that the Kubernetes Gateway API syntax is used to create the gateway, HTTPRoute and services, but Kubernetes gets the details of those items from VPC Lattice:

* Infrastructure provider: Creates the Kubernetes GatewayClass to identify VPC Lattice as the GatewayClass.
* Cluster operator: Creates the Kubernetes Gateway, which gets information from VPC Lattice related to the Service Gateway and Service Networks, as well as their related Service Policies.
* Application developer: Creates HTTPRoute objects that point to Kubernetes services, which in turn are directed to particular pods, in this case.
  This is all done by checking the related VPC Lattice Services (and related policies), Target Groups, and Targets
  Keep in mind that Target Groups v1 and v2 can be on different clusters in different VPCs.

## Further information

TODO: Add links to other docs, blogs, or software (will any be ready in time for re:Invent?)

