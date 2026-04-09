# Build the manager binary
FROM registry.access.redhat.com/ubi10/go-toolset@sha256:3712c9b5021efb3d0d92d72d84342fe8dce2783cdd9f286e87dff86165977b16 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace/may/
COPY may/api api
COPY may/go.mod may/go.sum ./

WORKDIR /workspace/drivers/ibm/
# Copy the Go Modules manifests
COPY drivers/ibm/go.mod drivers/ibm/go.sum ./
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the Go source (relies on .dockerignore to filter)
COPY drivers/ibm/ .

# Build
# the GOARCH has no default value to allow the binary to be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o /tmp/manager cmd/main.go

FROM registry.access.redhat.com/ubi10/ubi-micro@sha256:f86852f349dcd2b9ebccef4c8a46fdb75ff2fef9fde8581cef1feddb706be7ba
WORKDIR /
COPY --from=builder /tmp/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
