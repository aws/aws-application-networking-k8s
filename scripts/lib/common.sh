#!/usr/bin/env bash

# common.sh contains commonly used functions, meant to be imported by other
# bash scripts.

LIB_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

source "$LIB_DIR/logging.sh"

# check_is_installed checks to see if the supplied executable is installed and
# exits if not. An optional second argument is an extra message to display when
# the supplied executable is not installed.
#
# Usage:
#
#   check_is_installed PROGRAM [ MSG ]
#
# Example:
#
#   check_is_installed kind "You can install kind with the helper scripts/install-kind.sh"
check_is_installed() {
    local __name="$1"
    local __extra_msg="$2"
    if ! is_installed "$__name"; then
        error_msg "Missing required binary in PATH: '$__name'"
        error_msg "Please install $__name before running this script."
        if [[ -n $__extra_msg ]]; then
            error_msg ""
            error_msg "$__extra_msg"
            error_msg ""
        fi
        exit 1
    fi
}

is_installed() {
    local __name="$1"
    if $(which $__name >/dev/null 2>&1); then
        return 0
    else
        return 1
    fi
}