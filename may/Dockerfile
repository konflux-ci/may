# Build the manager binary
FROM registry.access.redhat.com/ubi10/go-toolset@sha256:3712c9b5021efb3d0d92d72d84342fe8dce2783cdd9f286e87dff86165977b16 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG ENABLE_COVERAGE=false

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.sum ./
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source (relies on .dockerignore to filter)
COPY . .

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
