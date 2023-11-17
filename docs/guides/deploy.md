# Deploying the AWS Gateway API Controller

Follow these instructions to create a cluster and deploy the AWS Gateway API Controller.
Run through them again for a second cluster to use with the extended example shown later.

**NOTE**: You can get the yaml files used on this page by cloning the [AWS Gateway API Controller](https://github.com/aws/aws-application-networking-k8s) repository.

## Cluster Setup

### Using EKS Cluster

EKS is a simple, recommended way of preparing a cluster for running services with AWS Gateway API Controller.

1. Set your region and cluster name as environment variables. See the [Amazon VPC Lattice FAQs](https://aws.amazon.com/vpc/lattice/faqs/) for a list of supported regions. For this example, we use `us-west-2`:
   ```bash
   export AWS_REGION=us-west-2
   export CLUSTER_NAME=my-cluster
   ```
1. You can use an existing EKS cluster or create a new one as shown here:
   ```bash
   eksctl create cluster --name $CLUSTER_NAME --region $AWS_REGION
   ```

1. Configure security group to receive traffic from the VPC Lattice network. You must set up security groups so that they allow all Pods communicating with VPC Lattice to allow traffic from the VPC Lattice managed prefix lists.  See [Control traffic to resources using security groups](https://docs.aws.amazon.com/vpc/latest/userguide/VPC_SecurityGroups.html) for details. Lattice has both an IPv4 and IPv6 prefix lists available.
    ```bash
    CLUSTER_SG=$(aws eks describe-cluster --name $CLUSTER_NAME --output json| jq -r '.cluster.resourcesVpcConfig.clusterSecurityGroupId')
    PREFIX_LIST_ID=$(aws ec2 describe-managed-prefix-lists --query "PrefixLists[?PrefixListName=="\'com.amazonaws.$AWS_REGION.vpc-lattice\'"].PrefixListId" | jq -r '.[]')
    aws ec2 authorize-security-group-ingress --group-id $CLUSTER_SG --ip-permissions "PrefixListIds=[{PrefixListId=${PREFIX_LIST_ID}}],IpProtocol=-1"
    PREFIX_LIST_ID_IPV6=$(aws ec2 describe-managed-prefix-lists --query "PrefixLists[?PrefixListName=="\'com.amazonaws.$AWS_REGION.ipv6.vpc-lattice\'"].PrefixListId" | jq -r '.[]')
    aws ec2 authorize-security-group-ingress --group-id $CLUSTER_SG --ip-permissions "PrefixListIds=[{PrefixListId=${PREFIX_LIST_ID_IPV6}}],IpProtocol=-1"
    ```
1. Create an IAM OIDC provider: See [Creating an IAM OIDC provider for your cluster](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html) for details.
   ```bash
   eksctl utils associate-iam-oidc-provider --cluster $CLUSTER_NAME --approve --region $AWS_REGION
   ```
1. Create a policy (`recommended-inline-policy.json`) in IAM with the following content that can invoke the gateway API and copy the policy arn for later use:

   ```bash
   {
       "Version": "2012-10-17",
       "Statement": [
           {
               "Effect": "Allow",
               "Action": [
                   "vpc-lattice:*",
                   "iam:CreateServiceLinkedRole",
                   "ec2:DescribeVpcs",
                   "ec2:DescribeSubnets",
                   "ec2:DescribeTags",
                   "ec2:DescribeSecurityGroups",
                   "logs:CreateLogDelivery",
                   "logs:GetLogDelivery",
                   "logs:UpdateLogDelivery",
                   "logs:DeleteLogDelivery",
                   "logs:ListLogDeliveries"
               ],
               "Resource": "*"
           }
       ]
   }
   ```
   ```bash
   aws iam create-policy \
      --policy-name VPCLatticeControllerIAMPolicy \
      --policy-document file://examples/recommended-inline-policy.json
   ```

1. Create the `aws-application-networking-system` namespace:
   ```bash
   kubectl apply -f examples/deploy-namesystem.yaml
   ```
1. Retrieve the policy ARN:
   ```bash
   export VPCLatticeControllerIAMPolicyArn=$(aws iam list-policies --query 'Policies[?PolicyName==`VPCLatticeControllerIAMPolicy`].Arn' --output text)
   ```
1. Create an iamserviceaccount for pod level permission:

   ```bash
   eksctl create iamserviceaccount \
      --cluster=$CLUSTER_NAME \
      --namespace=aws-application-networking-system \
      --name=gateway-api-controller \
      --attach-policy-arn=$VPCLatticeControllerIAMPolicyArn \
      --override-existing-serviceaccounts \
      --region $AWS_REGION \
      --approve
   ```

#### IPv6 support

IPv6 address type is automatically used for your services and pods if
[your cluster is configured to use IPv6 addresses](https://docs.aws.amazon.com/eks/latest/userguide/cni-ipv6.html).

```bash
# To create an IPv6 cluster
kubectl apply -f examples/ipv6-cluster.yaml
```

If your cluster is configured to be dual-stack, you can set the IP address type
of your service using the `ipFamilies` field. For example:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: ipv4-target-in-dual-stack-cluster
spec:
  ipFamilies:
    - "IPv4"
  selector:
    app: parking
  ports:
    - protocol: TCP
      port: 80
      targetPort: 8090
```


### Using a self-managed Kubernetes cluster

You can install AWS Gateway API Controller to a self-managed Kubernetes cluster in AWS.

The controller utilizes [IMDS](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html) to get necessary information from instance metadata, such as AWS account ID and VPC ID.
If your cluster is using IMDSv2, ensure the hop limit is 2 or higher to allow the access from the controller:

```bash
aws ec2 modify-instance-metadata-options --http-put-response-hop-limit 2 --region <region> --instance-id <instance-id>
```

Alternatively, you can manually provide configuration variables when installing the controller, as described in the next section.

## Controller Installation

1. Run either `kubectl` or `helm` to deploy the controller. Check [Environment Variables](../concepts/environment.md) for detailed explanation of each configuration option.

   ```bash
   kubectl apply -f examples/deploy-v0.0.18.yaml
   ```

   or

   ```bash
   # login to ECR
   aws ecr-public get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin public.ecr.aws
   # Run helm with either install or upgrade
   helm install gateway-api-controller \
      oci://public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller-chart\
      --version=v0.0.18 \
      --set=serviceAccount.create=false --namespace aws-application-networking-system \
      # awsRegion, clusterVpcId, awsAccountId are required for case IMDS is not available.
      --set=awsRegion= \
      --set=clusterVpcId= \
      --set=awsAccountId= \
      # clusterName is required except for EKS cluster.
      --set=clusterName= \
      # When specified, the controller will automatically create a service network with the name.
      --set=defaultServiceNetwork=my-hotel
   ```
1. Create the `amazon-vpc-lattice` GatewayClass:
   ```bash
   kubectl apply -f examples/gatewayclass.yaml
   ```
1. You are all set! Check our [Getting Started Guide](getstarted.md) to try setting up service-to-service communication.

