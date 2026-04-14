# Build the manager binary
FROM registry.access.redhat.com/ubi10/go-toolset@sha256:3712c9b5021efb3d0d92d72d84342fe8dce2783cdd9f286e87dff86165977b16 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG ENABLE_COVERAGE=false

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

# Build with or without coverage instrumentation
RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        echo "Building with coverage instrumentation..."; \
        CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -cover -covermode=atomic -tags=coverage -o /tmp/manager ./cmd; \
    else \
        echo "Building production binary..."; \
        CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o /tmp/manager cmd/main.go; \
    fi

FROM registry.access.redhat.com/ubi10/ubi-micro@sha256:f86852f349dcd2b9ebccef4c8a46fdb75ff2fef9fde8581cef1feddb706be7ba
WORKDIR /
COPY --from=builder /tmp/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
