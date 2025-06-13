# Deploy the AWS Gateway API Controller on Amazon EKS

This Deployment Guide provides an end-to-end procedure to install the AWS Gateway API Controller with [Amazon Elastic Kubernetes Service](https://aws.amazon.com/eks/).

Amazon EKS is a simple, recommended way of preparing a cluster for running services with the AWS Gateway API Controller. However, the AWS Gateway API Controller can be used on any Kubernetes cluster on AWS. Check out the [Advanced Configurations](advanced-configurations.md) section below for instructions on how to install and run the controller on self-hosted Kubernetes clusters on AWS.

### Prerequisites

Install these tools before proceeding:

1. [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2-linux.html),
2. `kubectl` – [the Kubernetes CLI](https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/),
3. `helm` – [the package manager for Kubernetes](https://helm.sh/docs/intro/install/),
4. `eksctl` – [the CLI for Amazon EKS](https://docs.aws.amazon.com/eks/latest/userguide/setting-up.html),
5. `jq` – [CLI to manipulate JSON files](https://jqlang.github.io/jq/).

### Setup

Set your AWS Region and Cluster Name as environment variables. See the [Amazon VPC Lattice FAQs](https://aws.amazon.com/vpc/lattice/faqs/) for a list of supported regions.

```bash
export AWS_REGION=<eks_cluster_region>
export EKS_CLUSTER_NAME=<EKS_CLUSTER_NAME>
```

**Install Gateway API CRDs**

The latest Gateway API CRDs are available [here](https://gateway-api.sigs.k8s.io/). Please [follow this installation](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api) process.

**Create a cluster (optional)**

You can easily create a cluster with `eksctl`, the CLI for Amazon EKS:

```bash
eksctl create cluster --name ${EKS_CLUSTER_NAME} --region $AWS_REGION
```

**Allow traffic from Amazon VPC Lattice**

You must set up security groups so that they allow all Pods communicating with VPC Lattice to allow traffic from the VPC Lattice managed prefix lists. See [Control traffic to resources using security groups](https://docs.aws.amazon.com/vpc/latest/userguide/VPC_SecurityGroups.html) for details. Lattice has both IPv4 and IPv6 prefix lists available.

1. Configure the EKS nodes' security group to receive traffic from the VPC Lattice network.

```bash
CLUSTER_SG=<your_node_security_group>
```

!!!Note
    If you have created the cluster with the `eksctl create cluster --name ${EKS_CLUSTER_NAME} --region $AWS_REGION` command, you can use the following command to export the Security Group ID:

```bash
CLUSTER_SG=$(aws eks describe-cluster --name ${EKS_CLUSTER_NAME} --output json | jq -r '.cluster.resourcesVpcConfig.clusterSecurityGroupId')
```

```bash
PREFIX_LIST_ID=$(aws ec2 describe-managed-prefix-lists --query "PrefixLists[?PrefixListName=='com.amazonaws.$AWS_REGION.vpc-lattice'].PrefixListId" | jq -r '.[]')
aws ec2 authorize-security-group-ingress --group-id $CLUSTER_SG --ip-permissions "PrefixListIds=[{PrefixListId=${PREFIX_LIST_ID}}],IpProtocol=-1"
PREFIX_LIST_ID_IPV6=$(aws ec2 describe-managed-prefix-lists --query "PrefixLists[?PrefixListName=='com.amazonaws.$AWS_REGION.ipv6.vpc-lattice'].PrefixListId" | jq -r '.[]')
aws ec2 authorize-security-group-ingress --group-id $CLUSTER_SG --ip-permissions "PrefixListIds=[{PrefixListId=${PREFIX_LIST_ID_IPV6}}],IpProtocol=-1"
```

**Set up IAM permissions**

The AWS Gateway API Controller needs to have the necessary permissions to operate.

1. Create a policy (`recommended-inline-policy.json`) in IAM with the following content that can invoke the Gateway API and copy the policy ARN for later use:

```bash
curl https://raw.githubusercontent.com/aws/aws-application-networking-k8s/main/files/controller-installation/recommended-inline-policy.json -o recommended-inline-policy.json

aws iam create-policy \
    --policy-name VPCLatticeControllerIAMPolicy \
    --policy-document file://recommended-inline-policy.json

export VPCLatticeControllerIAMPolicyArn=$(aws iam list-policies --query 'Policies[?PolicyName==`VPCLatticeControllerIAMPolicy`].Arn' --output text)
```

2. Create the `aws-application-networking-system` namespace:

```bash
curl -o aws-application-networking-system.yaml https://raw.githubusercontent.com/aws/aws-application-networking-k8s/main/files/controller-installation/deploy-namesystem.yaml
kubectl apply -f https://raw.githubusercontent.com/aws/aws-application-networking-k8s/main/files/controller-installation/deploy-namesystem.yaml
```

You can choose from [Pod Identities](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html) (recommended) and [IAM Roles For Service Accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) to set up controller permissions.

=== "Pod Identities (recommended)"

**Set up the Pod Identities Agent**

To use Pod Identities, set up the [Pod Identity Agent](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-agent-setup.html) and configure the controller's Kubernetes Service Account to assume necessary permissions with EKS Pod Identity.

!!!Note "Read if you are using a custom node role"
    The node role needs to have permissions for the Pod Identity Agent to perform the `AssumeRoleForPodIdentity` action in the EKS Auth API. Follow [the documentation](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-agent-setup.html) if you are not using the AWS managed policy [AmazonEKSWorkerNodePolicy](https://docs.aws.amazon.com/eks/latest/userguide/security-iam-awsmanpol.html#security-iam-awsmanpol-AmazonEKSWorkerNodePolicy).

1. Run the following AWS CLI command to create the Pod Identity addon:

```bash
aws eks create-addon --cluster-name ${EKS_CLUSTER_NAME} --addon-name eks-pod-identity-agent --addon-version v1.0.0-eksbuild.1
```

```bash
kubectl get pods -n kube-system | grep 'eks-pod-identity-agent'
```

**Assign role to Service Account**

1. Create a Service Account:

```bash
cat >gateway-api-controller-service-account.yaml <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
    name: gateway-api-controller
    namespace: aws-application-networking-system
EOF
kubectl apply -f gateway-api-controller-service-account.yaml
```

2. Create a trust policy file for the IAM role:

```bash
cat > eks-pod-identity-trust-relationship.json <<EOF
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "AllowEksAuthToAssumeRoleForPodIdentity",
            "Effect": "Allow",
            "Principal": {
                "Service": "pods.eks.amazonaws.com"
            },
            "Action": [
                "sts:AssumeRole",
                "sts:TagSession"
            ]
        }
    ]
}
EOF
```

3. Create the role:

```bash
aws iam create-role --role-name VPCLatticeControllerIAMRole --assume-role-policy-document file://eks-pod-identity-trust-relationship.json --description "IAM Role for AWS Gateway API Controller for VPC Lattice"
aws iam attach-role-policy --role-name VPCLatticeControllerIAMRole --policy-arn=$VPCLatticeControllerIAMPolicyArn
export VPCLatticeControllerIAMRoleArn=$(aws iam list-roles --query 'Roles[?RoleName==`VPCLatticeControllerIAMRole`].Arn' --output text)
```

4. Create the association:

```bash
aws eks create-pod-identity-association --cluster-name ${EKS_CLUSTER_NAME} --role-arn ${VPCLatticeControllerIAMRoleArn} --namespace aws-application-networking-system --service-account gateway-api-controller
```

=== "IRSA"

You can use AWS IAM Roles for Service Accounts (IRSA) to assign the controller necessary permissions via a ServiceAccount.

1. Create an IAM OIDC provider. See [Creating an IAM OIDC provider for your cluster](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html) for details.

```bash
eksctl utils associate-iam-oidc-provider --cluster ${EKS_CLUSTER_NAME} --approve --region $AWS_REGION
```

2. Create an IAM service account for pod-level permission:

```bash
eksctl create iamserviceaccount \
    --cluster=${EKS_CLUSTER_NAME} \
    --namespace=aws-application-networking-system \
    --name=gateway-api-controller \
    --attach-policy-arn=$VPCLatticeControllerIAMPolicyArn \
    --override-existing-serviceaccounts \
    --region $AWS_REGION \
    --approve
```

### Install the Controller

Run **either** `kubectl` or `helm` to deploy the controller. Check [Environment Variables](../guides/environment.md) for a detailed explanation of each configuration option.

=== "Helm"

```bash
# login to ECR
aws ecr-public get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin public.ecr.aws

# Run helm with either install or upgrade
helm install gateway-api-controller \
    oci://public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller-chart \
    --version=v1.1.0 \
    --set=serviceAccount.create=false \
    --namespace aws-application-networking-system \
    --set=log.level=info # use "debug" for debug level logs
```

=== "Kubectl"

```bash
kubectl apply -f https://raw.githubusercontent.com/aws/aws-application-networking-k8s/main/files/controller-installation/deploy-v1.1.2.yaml
```

1. Create the `amazon-vpc-lattice` GatewayClass:

```bash
kubectl apply -f https://raw.githubusercontent.com/aws/aws-application-networking-k8s/main/files/controller-installation/gatewayclass.yaml
```
