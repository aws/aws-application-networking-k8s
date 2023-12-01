#!/usr/bin/env bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:="1.22.x"}"
KUBEBUILDER_ASSETS="${KUBEBUILDER_ASSETS:="${HOME}/.kubebuilder/bin"}"

main() {
    tools
    kubebuilder
    install_golangci_lint
}

install_golangci_lint() {
    if { [ -z "${CI+x}" ] || [ "${CI}" != "true" ]; } && ! command -v golangci-lint &> /dev/null; then
        echo "golangci-lint is not installed in local machine. Installing..."
        # https://golangci-lint.run/usage/install/#local-installation
        curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.55.2
    fi
}

tools() {
    if ! echo "$PATH" | grep -q "${GOPATH:-undefined}/bin\|$HOME/go/bin"; then
        echo "Go workspace's \"bin\" directory is not in PATH. Run 'export PATH=\"\$PATH:\${GOPATH:-\$HOME/go}/bin\"'."
        exit 1
    fi

    go install github.com/golang/mock/mockgen@v1.6.0
    go install sigs.k8s.io/kustomize/kustomize/v4@v4.5.7
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20220421205612-c162794a9b12
    go install github.com/mattn/goveralls@b031368
}

kubebuilder() {
    mkdir -p $KUBEBUILDER_ASSETS
    arch=$(go env GOARCH)
    ## Kubebuilder does not support darwin/arm64, so use amd64 through Rosetta instead
    if [[ $(go env GOOS) == "darwin" ]] && [[ $(go env GOARCH) == "arm64" ]]; then
        arch="amd64"
    fi
    ln -sf $(setup-envtest use -p path "${K8S_VERSION}" --arch="${arch}" --bin-dir="${KUBEBUILDER_ASSETS}")/* ${KUBEBUILDER_ASSETS}
    find $KUBEBUILDER_ASSETS
}

main "$@"
