# Deploying the AWS Gateway API Controller

Follow these instructions to create a cluster and deploy the AWS Gateway API Controller.
Run through them again for a second cluster to use with the extended example shown later.

**NOTE**: You can get the yaml files used on this page by cloning the [AWS Gateway API Controller for VPC Lattice](https://github.com/aws/aws-application-networking-k8s) site. The files are in the `examples/` directory.

1. Set your region and cluster name as environment variables. See the [Amazon VPC Lattice FAQs](https://aws.amazon.com/vpc/lattice/faqs/) for a list of supported regions. For this example, we use `us-west-2`:
   ```bash
   export AWS_REGION=us-west-2
   export CLUSTER_NAME=my-cluster
   ```
2. You can use an existing EKS cluster or create a new one as shown here:
   ```bash
   eksctl create cluster --name $CLUSTER_NAME --region $AWS_REGION
   ```
3. Configure security group to receive traffic from the VPC Lattice network. You must set up security groups so that they allow all Pods communicating with VPC Lattice to allow traffic from the VPC Lattice managed prefix lists.  See [Control traffic to resources using security groups](https://docs.aws.amazon.com/vpc/latest/userguide/VPC_SecurityGroups.html) for details. Lattice has both an IPv4 and IPv6 prefix lists available.

    ```bash
    CLUSTER_SG=$(aws eks describe-cluster --name $CLUSTER_NAME --output json| jq -r '.cluster.resourcesVpcConfig.clusterSecurityGroupId')
    PREFIX_LIST_ID=$(aws ec2 describe-managed-prefix-lists --query "PrefixLists[?PrefixListName=="\'com.amazonaws.$AWS_REGION.vpc-lattice\'"].PrefixListId" | jq -r '.[]')
    aws ec2 authorize-security-group-ingress --group-id $CLUSTER_SG --ip-permissions "PrefixListIds=[{PrefixListId=${PREFIX_LIST_ID}}],IpProtocol=-1"
    PREFIX_LIST_ID_IPV6=$(aws ec2 describe-managed-prefix-lists --query "PrefixLists[?PrefixListName=="\'com.amazonaws.$AWS_REGION.ipv6.vpc-lattice\'"].PrefixListId" | jq -r '.[]')
    aws ec2 authorize-security-group-ingress --group-id $CLUSTER_SG --ip-permissions "PrefixListIds=[{PrefixListId=${PREFIX_LIST_ID_IPV6}}],IpProtocol=-1"
    ```
3. Create an IAM OIDC provider: See [Creating an IAM OIDC provider for your cluster](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html) for details.
   ```bash
   eksctl utils associate-iam-oidc-provider --cluster $CLUSTER_NAME --approve --region $AWS_REGION
   ```
4. Create a policy (`recommended-inline-policy.json`) in IAM with the following content that can invoke the gateway API and copy the policy arn for later use:
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
5. Create the `aws-application-networking-system` namespace:
   ```bash
   kubectl apply -f examples/deploy-namesystem.yaml
   ```
6. Retrieve the policy ARN:
   ```bash
   export VPCLatticeControllerIAMPolicyArn=$(aws iam list-policies --query 'Policies[?PolicyName==`VPCLatticeControllerIAMPolicy`].Arn' --output text)
   ```
7. Create an iamserviceaccount for pod level permission:
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
8. Run either `kubectl` or `helm` to deploy the controller:
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
      # awsRegion, clusterVpcId, clusterName, awsAccountId are required for case where IMDS is NOT AVAILABLE, e.g Fargate, self-managed clusters with IMDS access blocked 
      --set=awsRegion= \
      --set=clusterVpcId= \
      --set=clusterName= \
      --set=awsAccountId= \
      --set=defaultServiceNetwork= \ # check environment.md for more its details
      # latticeEndpoint is required for the case where the VPC Lattice endpoint is being overridden
      --set=latticeEndpoint= \
      
   
   ```
9. Create the `amazon-vpc-lattice` GatewayClass:
   ```bash
   kubectl apply -f examples/gatewayclass.yaml
   ```
