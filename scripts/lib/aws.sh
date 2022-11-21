#!/usr/bin/env bash

# aws.sh contains functions for running common AWS command line methods.

LIB_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

source "$LIB_DIR/common.sh"
source "$LIB_DIR/config.sh"
source "$LIB_DIR/logging.sh"

AWS_CLI_VERSION=${DEFAULT_AWS_CLI_VERSION:-"2.0.52"}

# daws() executes the AWS Python CLI tool from a Docker container.
#
# Instead of relying on developers having a particular version of the AWS
# Python CLI tool, this method allows a specific version of the CLI tool to be
# executed within a Docker container.
#
# You call the daws function just like you were calling the `aws` CLI tool.
#
# Usage:
#
#   daws SERVICE COMMAND [OPTIONS]
#
# Example:
#
#   daws ecr describe-repositories --repository-name my-repo
#
# To use a specific version of the AWS CLI, set the ACK_AWS_CLI_IMAGE_VERSION
# environment variable, otherwise the value of DEFAULT_AWS_CLI_VERSION is used.
daws() {
    local profile="$(get_aws_profile)"
    local identity_file="$(get_aws_token_file)"
    local default_region="$(get_aws_region)"

    aws_cli_profile_env=$([ ! -z "$profile" ] && echo "--env AWS_PROFILE=$profile")
    aws_cli_web_identity_env="$([ ! -z "$identity_file" ] && \
        echo "--env AWS_WEB_IDENTITY_TOKEN_FILE=/root/aws_token --env AWS_ROLE_ARN -v $identity_file:/root/aws_token:ro" )"
    aws_cli_img="amazon/aws-cli:$AWS_CLI_VERSION"

    docker run --rm -v ~/.aws:/root/.aws:z $(echo $aws_cli_profile_env) $(echo $aws_cli_web_identity_env) --env AWS_DEFAULT_REGION=$default_region -v $(pwd):/aws "$aws_cli_img" "$@"
}

# ensure_aws_credentials() calls the STS::GetCallerIdentity API call and
# verifies that there is a local identity for running AWS commands
ensure_aws_credentials() {
    daws sts get-caller-identity --query "Account" >/dev/null ||
        ( error_msg "No AWS credentials found. Please run \`aws configure\` to set up the CLI for your credentials" && exit 1)
}

# generate_aws_temp_creds function will generate temporary AWS credentials which
# are valid for 3600 seconds
aws_generate_temp_creds() {
    local __uuid=$(uuidgen | cut -d'-' -f1 | tr '[:upper:]' '[:lower:]')

    local assumed_role=$(get_assumed_role_arn)

    local json=$(daws sts assume-role \
           --role-arn "$assumed_role"  \
           --role-session-name tmp-role-"$__uuid" \
           --duration-seconds 3600 \
           --output json || exit 1)

    AWS_ACCESS_KEY_ID=$(echo "${json}" | jq --raw-output ".Credentials[\"AccessKeyId\"]")
    AWS_SECRET_ACCESS_KEY=$(echo "${json}" | jq --raw-output ".Credentials[\"SecretAccessKey\"]")
    AWS_SESSION_TOKEN=$(echo "${json}" | jq --raw-output ".Credentials[\"SessionToken\"]")
}

aws_account_id() {
    local json=$(daws sts get-caller-identity --output json || exit 1)
    echo "${json}" | jq --raw-output ".Account"
}

ensure_binaries() {
    check_is_installed "docker"
    check_is_installed "jq"
    check_is_installed "uuidgen"
}

ensure_binaries