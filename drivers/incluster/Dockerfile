# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.25@sha256:31ce6a4b3348874c6a3ba1f7e88371348f96b34b7615e6680f16f4b0c9cf3c6b AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace/may/
COPY may/api api
COPY may/go.mod may/go.sum ./

WORKDIR /workspace/drivers/incluster/
# Copy the Go Modules manifests
COPY drivers/incluster/go.mod drivers/incluster/go.sum ./
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source (relies on .dockerignore to filter)
COPY drivers/incluster/ .

# Build
# the GOARCH has no default value to allow the binary to be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o /tmp/manager cmd/main.go

FROM registry.access.redhat.com/ubi9/ubi-micro@sha256:f2359f63cae4bd0e832a385fdec2c02874a3edc3405f3a278df618a0602c6f5f
WORKDIR /
COPY --from=builder /tmp/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
