#!/usr/bin/env bash

declare -a DEPENDENCY_LIST=("go" "awscli" "kubernetes-cli" "eksctl" "helm" "jq" "yq" "make")
CURRENT_CONTROLLER_VERSION="2.0.0"
CURRENT_CRD_VERSION="1.2.0"
GOLANGCI_LINT_VERSION="2.4.0"
EKS_POD_IDENTITY_AGENT_VERSION="1.0.0-eksbuild.1"

main() {
    printf '\nSetting up your environment... 🚀\n'
    credentials
    tools
    cluster
    crds
    printf '\nSetup completed successfully! 🎉\n'
}

installOrUpdatePackage() {
    if brew list "$1" &>/dev/null; then
        read -r -p "${1} is already installed, do you want to update? (Y/N): " update_package
        if [[ $update_package == 'Y' || $update_package == 'y' ]]; then
            echo "Updating ${1}"
            brew upgrade "$1"
        fi
    else
        echo "Installing ${1}"
        brew install "$1"
    fi
}

credentials() {
    read -r -p "Do you want to configure AWS credentials? (Y/N): " configure_creds
    if [[ $configure_creds == 'Y' || $configure_creds == 'y' ]]; then
        echo "Note: Prefer AWS SSO or role-based auth over long lived access keys."
        read -r -p "Persist credentials to ~/.aws/credentials via 'aws configure set'? (Y/N): " persist_creds
        read -r -p "Enter AWS Access Key: " access_key
        read -r -s -p "Enter AWS Secret Access Key: " secret_key
        echo
        read -r -p "Enter AWS Region: " region

        if [[ $persist_creds == 'Y' || $persist_creds == 'y' ]]; then
            aws configure set aws_access_key_id "$access_key"
            aws configure set aws_secret_access_key "$secret_key"
            aws configure set default.region "$region"
        else
            export AWS_ACCESS_KEY_ID="$access_key"
            export AWS_SECRET_ACCESS_KEY="$secret_key"
            export AWS_REGION="$region"
            export AWS_DEFAULT_REGION="$region"
        fi

        echo "AWS credentials configured successfully."
    fi
    echo "---------------------------------"
}

tools() {
    read -r -p "Do you want to install/update tools? (Y/N): " install_tools
    if [[ $install_tools == 'Y' || $install_tools == 'y' ]]; then
        if ! command -v brew &> /dev/null; then
            echo "Installing Homebrew..."
            /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
            export PATH=/opt/homebrew/bin:$PATH
        else
            read -r -p "Homebrew is already installed, do you want to update? (Y/N): " update_package

            if [[ $update_package == 'Y' || $update_package == 'y' ]]; then
                echo "Updating Homebrew"
                brew update
            fi
        fi

        for i in "${DEPENDENCY_LIST[@]}"; do
            installOrUpdatePackage "$i"
        done

        if ! command -v golangci-lint &> /dev/null; then
            echo "Installing golangci-lint"
            curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v$GOLANGCI_LINT_VERSION
        else

            read -r -p "golangci-lint is already installed, do you want to update? (Y/N): " update_package

            if [[ $update_package == 'Y' || $update_package == 'y' ]]; then
                echo "Updating golangci-lint"
                curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v$GOLANGCI_LINT_VERSION
            fi
        fi

        go install github.com/golang/mock/mockgen@v1.6.0

        echo "Tools installed/updated successfully."
    fi
    echo "---------------------------------"
}

cluster() {
    read -r -p "Do you want to create an EKS cluster? (Y/N): " create_cluster
    if [[ $create_cluster == 'Y' || $create_cluster == 'y' ]]; then
        read -r -p "Enter a Cluster Name. The name must satisfy the regular expression pattern [a-zA-Z][-a-zA-Z0-9]: " cluster_name
        read -r -p "Enter AWS Region: " region
        read -r -p "Enter Controller Version. Entering no version will default to $CURRENT_CONTROLLER_VERSION: " controller_version
        if [[ $controller_version == null || $controller_version == '' ]]; then
            echo "Defaulting to $CURRENT_CONTROLLER_VERSION."
            export CONTROLLER_VERSION=$CURRENT_CONTROLLER_VERSION
        else
            export CONTROLLER_VERSION=$controller_version
        fi

        export CLUSTER_NAME=$cluster_name
        export AWS_REGION=$region

        describe_cluster_output=$( aws eks describe-cluster --name "$CLUSTER_NAME" --output text 2>&1 )
        if [[ $describe_cluster_output == *"ResourceNotFoundException"* ]]; then
            echo "Creating cluster with name: $cluster_name"

            eksctl create cluster --name "$CLUSTER_NAME" --region "$AWS_REGION"

            describe_cluster_output=$( aws eks describe-cluster --name "$CLUSTER_NAME" --output text 2>&1 )
            if [[ $describe_cluster_output == *"ResourceNotFoundException"* ]]; then
                echo "Cluster creation failed, please try again."
                echo "---------------------------------"
                return 1
            fi

            echo "Allowing traffic from VPC Lattice to EKS cluster"
            CLUSTER_SG=$(aws eks describe-cluster --name "$CLUSTER_NAME" --output json| jq -r '.cluster.resourcesVpcConfig.clusterSecurityGroupId')

            PREFIX_LIST_ID="$(aws ec2 describe-managed-prefix-lists --output json --query "PrefixLists[?PrefixListName=='com.amazonaws.${AWS_REGION}.vpc-lattice'].PrefixListId" | jq -r '.[]')"
            aws ec2 authorize-security-group-ingress --group-id "$CLUSTER_SG" --ip-permissions "PrefixListIds=[{PrefixListId=${PREFIX_LIST_ID}}],IpProtocol=-1" --no-cli-pager

            PREFIX_LIST_ID_IPV6="$(aws ec2 describe-managed-prefix-lists --output json --query "PrefixLists[?PrefixListName=='com.amazonaws.${AWS_REGION}.ipv6.vpc-lattice'].PrefixListId" | jq -r '.[]')"
            aws ec2 authorize-security-group-ingress --group-id "$CLUSTER_SG" --ip-permissions "PrefixListIds=[{PrefixListId=${PREFIX_LIST_ID_IPV6}}],IpProtocol=-1" --no-cli-pager

            # shellcheck disable=SC2016
            # JMESPath queries intentionally use backticks for literal matching.
            VPCLatticeControllerIAMPolicyArn="$(aws iam list-policies --query 'Policies[?PolicyName==`VPCLatticeControllerIAMPolicy`].Arn' --output text 2>&1)"
            export VPCLatticeControllerIAMPolicyArn
            if [[ $VPCLatticeControllerIAMPolicyArn != *"arn"* ]]; then
                echo "Setting up IAM permissions"
                curl https://raw.githubusercontent.com/aws/aws-application-networking-k8s/main/files/controller-installation/recommended-inline-policy.json -o recommended-inline-policy.json
                aws iam create-policy \
                    --policy-name VPCLatticeControllerIAMPolicy \
                    --policy-document file://recommended-inline-policy.json --no-cli-pager
                # shellcheck disable=SC2016
                VPCLatticeControllerIAMPolicyArn="$(aws iam list-policies --query 'Policies[?PolicyName==`VPCLatticeControllerIAMPolicy`].Arn' --output text)"
                export VPCLatticeControllerIAMPolicyArn
                rm -f recommended-inline-policy.json
                echo "IAM permissions set up successfully"
            else
                echo "Policy already exists, skipping creation"
            fi

            kubectl apply -f https://raw.githubusercontent.com/aws/aws-application-networking-k8s/main/files/controller-installation/deploy-namesystem.yaml

            echo "Setting up the Pod Identities Agent"
            aws eks create-addon --cluster-name "$CLUSTER_NAME" --addon-name eks-pod-identity-agent --addon-version v$EKS_POD_IDENTITY_AGENT_VERSION --no-cli-pager
            kubectl get pods -n kube-system | grep 'eks-pod-identity-agent'
            echo "Pod Identities Agent set up successfully"

            # shellcheck disable=SC2016
            VPCLatticeControllerIAMRoleArn="$(aws iam list-roles --query 'Roles[?RoleName==`VPCLatticeControllerIAMRole`].Arn' --output text 2>&1)"
            export VPCLatticeControllerIAMRoleArn
            if [[ $VPCLatticeControllerIAMRoleArn != *"arn"* ]]; then
                echo "Assigning a role to the service account"

                cat >gateway-api-controller-service-account.yaml <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
    name: gateway-api-controller
    namespace: aws-application-networking-system
EOF
                kubectl apply -f gateway-api-controller-service-account.yaml
                rm -f gateway-api-controller-service-account.yaml

                cat >trust-relationship.json <<EOF
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

                aws iam create-role --role-name VPCLatticeControllerIAMRole --assume-role-policy-document file://trust-relationship.json --description "IAM Role for AWS Gateway API Controller for VPC Lattice" --no-cli-pager
                aws iam attach-role-policy --role-name VPCLatticeControllerIAMRole --policy-arn="$VPCLatticeControllerIAMPolicyArn" --no-cli-pager
                # shellcheck disable=SC2016
                VPCLatticeControllerIAMRoleArn="$(aws iam list-roles --query 'Roles[?RoleName==`VPCLatticeControllerIAMRole`].Arn' --output text)"
                export VPCLatticeControllerIAMRoleArn
                rm -f trust-relationship.json
                echo "Role assigned successfully"
            else
                echo "Role already exists, skipping creation"
            fi

            eksctl create podidentityassociation --cluster "$CLUSTER_NAME" --namespace aws-application-networking-system --service-account-name gateway-api-controller --role-arn "$VPCLatticeControllerIAMRoleArn"

            echo "Installing the controller"
            kubectl apply -f "https://raw.githubusercontent.com/aws/aws-application-networking-k8s/main/files/controller-installation/deploy-v${CONTROLLER_VERSION}.yaml"

            echo "EKS cluster created successfully."
        elif [[ $describe_cluster_output == *"error"* ]]; then
             echo "Error describing cluster: $describe_cluster_output"
             echo "---------------------------------"
             return 1
        else
            echo "Cluster: $cluster_name already exists. Skipping creation."
        fi
    fi
    echo "---------------------------------"
}

crds() {
    read -r -p "Do you want to install the Gateway API CRDs? (Y/N): " install_crds
    if [[ $install_crds == 'Y' || $install_crds == 'y' ]]; then
        read -r -p "Enter Gateway API CRDs Version. Entering no version will default to $CURRENT_CRD_VERSION: " crds_version
        if [[ $crds_version == null || $crds_version == '' ]]; then
            echo "Defaulting to $CURRENT_CRD_VERSION."
            export CRDS_VERSION=$CURRENT_CRD_VERSION
        else
            export CRDS_VERSION=$crds_version
        fi

        kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/v${CRDS_VERSION}/standard-install.yaml" --validate=false
        kubectl apply -f https://raw.githubusercontent.com/aws/aws-application-networking-k8s/main/files/controller-installation/gatewayclass.yaml

        echo "Gateway API CRDs installed successfully."
    fi
    echo "---------------------------------"
}

main "$@"