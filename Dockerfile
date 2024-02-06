# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.20.5 as builder

WORKDIR /workspace
ARG TARGETOS TARGETARCH

# Build
RUN --mount=target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -a -o /output/manager cmd/aws-application-networking-k8s/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM --platform=$TARGETPLATFORM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /output/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
