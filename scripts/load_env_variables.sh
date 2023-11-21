#!/bin/sh

# When Running the k8s Controller by GoLand IDE locally, it is required to set the environment variables, this script will generate a file called `envFile`
# that contains all needed environment variables, this `envFile` file can be use by GoLand "EnvFile" plugin
# When config the "green arrow"(how we `run` or `debug` this project) in  GoLand, we could set a "before launch" task to run this script `load_env_variables.sh`, and set the "EnvFile" plugin to load the `envFile` file
# "EnvFile" is a plugin, you can install it from GoLand plugin marketplace

echo "Setting environment variables"

> envFile

# Set KUBEBUILDER_ASSETS if not set
if [ -z "$KUBEBUILDER_ASSETS" ]; then
  KUBEBUILDER_ASSETS=${HOME}/.kubebuilder/bin
fi
echo "KUBEBUILDER_ASSETS=$KUBEBUILDER_ASSETS" >> envFile

# Set CLUSTER_NAME if not set
if [ -z "$CLUSTER_NAME" ]; then
  CLUSTER_NAME=$(kubectl config view --minify -o jsonpath='{.clusters[].name}' | rev | cut -d"/" -f1 | rev | cut -d"." -f1)
fi
echo "CLUSTER_NAME=$CLUSTER_NAME" >> envFile

# Set CLUSTER_VPC_ID if not set
if [ -z "$CLUSTER_VPC_ID" ]; then
  CLUSTER_VPC_ID=$(aws eks describe-cluster --name ${CLUSTER_NAME} | jq -r ".cluster.resourcesVpcConfig.vpcId")
fi
echo "CLUSTER_VPC_ID=$CLUSTER_VPC_ID" >> envFile

# Set AWS_ACCOUNT_ID if not set
if [ -z "$AWS_ACCOUNT_ID" ]; then
  AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
fi
echo "AWS_ACCOUNT_ID=$AWS_ACCOUNT_ID" >> envFile

if [ -z "$REGION" ]; then
  REGION=us-west-2
fi
echo "REGION=$REGION" >> envFile

echo "LOG_LEVEL=debug" >> envFile
