#!/usr/bin/env bash

# logging.sh contains functions for writing log messages to the console for 
# three different severities: debug, info and error.

# Prints out a message with an optional second argument indicating the
# "indentation level" for the message. If the indentation level argument is
# missing, we look for the existence of an environs variable called
# "indent_level" and use that.
_indented_msg() {
    local __msg=${1:-}
    local __indent_level=${2:-}
    local indent=""
    if [ -n "$__indent_level" ]; then
        indent="$( for each in $( seq 0 $__indent_level ); do printf " "; done )"
    fi

    local timestamp=$(date -Is)

    echo "$timestamp $__indent$__msg"
}

# debug_msg prints out a supplied message if the ACK_TEST_DEBUGGING_MODE environ
# variable is set.
debug_msg() {
    local __debug="${ACK_TEST_DEBUGGING_MODE:-""}"
    if [ ! -n "$__debug" ]; then
        return 0
    fi

    local __msg=${1:-}
    local __indent=${2:-}
    local __debug_prefix="${DEBUG_PREFIX:-"[DEBUG] "}"
    _indented_msg "$__debug_prefix$__msg" $__indent
}

# info_msg prints out a supplied message if the DEBUG environs variable is
# set.
info_msg() {
    local __msg=${1:-}
    local __indent=${2:-}
    local __info_prefix="${INFO_PREFIX:-"[INFO] "}"
    _indented_msg "$__info_prefix$__msg" $__indent
}

# debug_msg prints out a supplied message if the DEBUG environs variable is
# set.
error_msg() {
    local __msg=${1:-}
    local __indent=${2:-}
    local __error_prefix="${ERROR_PREFIX:-"[ERROR] "}"
    >&2 _indented_msg "$__error_prefix$__msg" $__indent
}