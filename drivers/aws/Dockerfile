# Build the manager binary
FROM registry.access.redhat.com/ubi10/go-toolset@sha256:81f0a8604f87b126a077cd55055fd5cc2b7b6536b3176171001b4dd47c322dfd AS builder
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

FROM registry.access.redhat.com/ubi10/ubi-micro@sha256:37fadb004c6bea628fcdd81376c8fb77bd8d9fd432d90503af4d9e76b1ff7191
WORKDIR /
COPY --from=builder /tmp/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
