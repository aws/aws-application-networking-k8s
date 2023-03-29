# Deploying the AWS Gateway API Controller

Follow these instructions to create a cluster and deploy the AWS Gateway API Controller.
Run through them again for a second cluster to use with the extended example shown later.

1. Set your region and cluster name as environment variables. Nine regions are now supported, including `us-west-2` and `us-east-1`. For example:
   ```bash
   export AWS_REGION=us-west-2
   export CLUSTER_NAME=my-cluster
   ```
1. You can use an existing EKS cluster or create a new one as shown here:
   ```bash
   eksctl create cluster --name $CLUSTER_NAME --region $AWS_REGION
   ```
1. First, configure security group to receive traffic from the VPC Lattice fleet. You must set up security groups so that they allow all Pods communicating with VPC Lattice to allow traffic on all ports from the `169.254.171.0/24` address range. 
    ```bash
    PREFIX_LIST_ID=$(aws ec2 describe-managed-prefix-lists --query "PrefixLists[?PrefixListName=="\'com.amazonaws.$AWS_REGION.vpc-lattice\'"].PrefixListId" | jq --raw-output .[])
    MANAGED_PREFIX=$(aws ec2 get-managed-prefix-list-entries --prefix-list-id $PREFIX_LIST_ID --output json  | jq -r '.Entries[0].Cidr')
    CLUSTER_SG=$(aws eks describe-cluster --name $CLUSTER_NAME --output json| jq -r '.cluster.resourcesVpcConfig.clusterSecurityGroupId')
    aws ec2 authorize-security-group-ingress --group-id $CLUSTER_SG --cidr $MANAGED_PREFIX --protocol -1
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
                   "ec2:DescribeSubnets"
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
1. Create the `system` namespace:
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
      --namespace=system \
      --name=gateway-api-controller \
      --attach-policy-arn=$VPCLatticeControllerIAMPolicyArn \
      --override-existing-serviceaccounts \
      --region $AWS_REGION \
      --approve
   ```
1. Run either `kubectl` or `helm` to deploy the controller:
   ```bash
   kubectl apply -f examples/deploy-v0.0.8.yaml
   ```
   or
   ```bash
   # login to ECR
   aws ecr-public get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin public.ecr.aws
   # Run helm with either install or upgrade
   helm install gateway-api-controller \
      oci://public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller-chart\
      --version=v0.0.8 \
      --set=aws.region=$AWS_REGION --set=serviceAccount.create=false --namespace system
   ```
1. Create the `amazon-vpc-lattice` GatewayClass:
   ```bash
   kubectl apply -f examples/gatewayclass.yaml
   ```
