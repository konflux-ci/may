# Build the manager binary
FROM registry.access.redhat.com/ubi10/go-toolset@sha256:3712c9b5021efb3d0d92d72d84342fe8dce2783cdd9f286e87dff86165977b16 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG ENABLE_COVERAGE=false

WORKDIR /workspace/may/
COPY may/api api
COPY may/go.mod may/go.sum ./

WORKDIR /workspace/drivers/aws/
# Copy the Go Modules manifests
COPY drivers/aws/go.mod drivers/aws/go.sum ./
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source (relies on .dockerignore to filter)
COPY drivers/aws/ .

# Build with or without coverage instrumentation
RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        echo "Building with coverage instrumentation..."; \
        CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -cover -covermode=atomic -tags=coverage -o /tmp/manager ./cmd; \
    else \
        echo "Building production binary..."; \
        CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o /tmp/manager cmd/main.go; \
    fi

FROM registry.access.redhat.com/ubi10/ubi-micro@sha256:6cc8b042384757a5dc8ee0adfa4ff7a3f37467d43c2ca42b24d928929536d579
WORKDIR /
COPY --from=builder /tmp/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
