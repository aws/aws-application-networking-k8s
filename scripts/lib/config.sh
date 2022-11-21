#!/usr/bin/env bash

# config.sh contains functions for extracting configuration elements from an ACK
# test config file, providing sane defaults when possible.

set -Eeo pipefail

LIB_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
SCRIPTS_DIR="$LIB_DIR/.."

TEST_CONFIG_PATH=${TEST_CONFIG_PATH:-"$SCRIPTS_DIR/../test_config.yaml"}

ASSUMED_ROLE_ARN="${ASSUMED_ROLE_ARN:-""}"

source "$LIB_DIR/common.sh"
source "$LIB_DIR/logging.sh"

_get_config_field() {
    local __field_path="$1"
    local __default="${2:-""}"

    if [[ "$__default" != "" ]]; then
        yq "$__field_path // \"$__default\"" $TEST_CONFIG_PATH 2>/dev/null
    else
        yq "$__field_path // \"\"" $TEST_CONFIG_PATH 2>/dev/null
    fi
}

_get_config_boolean() {
    # Booleans must be handled differently because the yq "Alternative" feature
    # returns the alternative value if the value of the field is "false" - 
    # instead of simply returning "false" (as we would expect)
    local __field_path="$1"
    local __default="${2:-""}"

    local ret=$(yq "$__field_path" $TEST_CONFIG_PATH 2>/dev/null)
    if [[ "$ret" == "null" ]]; then
        echo $__default
    fi
    echo $ret
}

get_cluster_create() { _get_config_boolean ".cluster.create"; }
get_cluster_name() { _get_config_field ".cluster.name"; }
get_cluster_k8s_version() { _get_config_field ".cluster.k8s_version"; }
get_cluster_configuration_file_name() { _get_config_field ".cluster.configuration.file_name" "kind-two-node-cluster.yaml"; }
get_cluster_additional_controllers() { _get_config_field ".cluster.configuration.additional_controllers"; }
get_aws_profile() { _get_config_field ".aws.profile"; }
get_aws_token_file() { _get_config_field ".aws.token_file"; }
get_aws_region() { _get_config_field ".aws.region" "us-west-2"; }
get_assumed_role_arn() {
    [[ ! -z "${ASSUMED_ROLE_ARN}" ]] && echo "${ASSUMED_ROLE_ARN}" || _get_config_field ".aws.assumed_role_arn";
}
get_test_markers() { _get_config_field ".tests.markers"; }
get_test_methods() { _get_config_field ".tests.methods"; }
get_run_tests_locally() { _get_config_field ".tests.run_locally"; }
get_is_local_build() { _get_config_boolean ".local_build" "false"; }
get_debug_enabled() { _get_config_boolean ".debug.enabled" "false"; }
get_dump_controller_logs() { _get_config_boolean ".debug.dump_controller_logs" "false"; }

get_test_config_path() { echo "$TEST_CONFIG_PATH"; }

ensure_config_file_exists() {
    [[ ! -f "$TEST_CONFIG_PATH" ]] && { error_msg "Config file does not exist at path $TEST_CONFIG_PATH"; exit 1; } || :
}

ensure_required_fields() {
    # Allow if the ASSUMED_ROLE_ARN is substituting
    [[ -z $(_get_config_field ".aws.assumed_role_arn") && -z ${ASSUMED_ROLE_ARN} ]] && { error_msg "Required config path \`.aws.assumed_role_arn\` not provided"; exit 1; } || :

    local required_field_paths=( ".cluster.create" )
    for path in "${required_field_paths[@]}"; do
        [[ -z $(_get_config_boolean $path) ]] && { error_msg "Required config path \`$path\` not provided"; exit 1; } || :
    done
}

ensure_binaries() {
    check_is_installed "yq"
}

ensure_binaries
ensure_config_file_exists
ensure_required_fields