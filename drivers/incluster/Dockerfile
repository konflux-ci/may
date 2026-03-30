# Build the manager binary
FROM registry.access.redhat.com/ubi10/go-toolset@sha256:b7f71131f21be923baaaf952309b7940ae7c6a55ca44a7a91460adc6c8afd4de AS builder
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

# Add coverport dependency when coverage is enabled
RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        go get github.com/konflux-ci/coverport/instrumentation/go && \
        go mod tidy; \
    fi

# Build with or without coverage instrumentation
RUN if [ "$ENABLE_COVERAGE" = "true" ]; then \
        echo "Building with coverage instrumentation..."; \
        CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -cover -covermode=atomic -tags=coverage -o /tmp/manager .; \
    else \
        echo "Building production binary..."; \
        CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o /tmp/manager cmd/main.go; \
    fi

FROM registry.access.redhat.com/ubi10/ubi-micro@sha256:551f8ee81be3dbabd45a9c197f3724b9724c1edb05d68d10bfe85a5c9e46a458
WORKDIR /
COPY --from=builder /tmp/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
