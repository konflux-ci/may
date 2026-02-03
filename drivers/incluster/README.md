# In-cluster driver

The **in-cluster** driver implements MAY’s Host contract by running hosts inside the cluster. It reconciles **StaticHost** (fixed set of runners on a host) and **DynamicHost** (one runner per host, created on demand), plus **DynamicHostAutoscaler** for scaling.

> **Warning:** This is a demo driver. It is **NOT** for production. Use it for local development, demo, or tests only.

**See the [repository root README](../../README.md)** for MAY concepts and the [demo README](../../demo/smug/README.md) for guided demos.

## What this component does

- **internal/controller**: Controllers for StaticHost, DynamicHost, and DynamicHostAutoscaler.
- Depends on MAY CRDs (Claim, Runner, StaticHost, DynamicHost, etc.); deploy the MAY controller first.

## Prerequisites

- Go 1.24+
- Docker, kubectl, Kind (for demo)
- MAY CRDs and controller installed (e.g. via `demo/hack/setup-cluster.sh`)

## Build, test, deploy

```sh
# Unit tests
make -C drivers/incluster test

# E2E tests
make -C drivers/incluster test-e2e

# Build and deploy (set IMG to your image)
make -C drivers/incluster docker-build docker-push IMG=<registry>/incluster:tag
make -C drivers/incluster deploy IMG=<registry>/incluster:tag
```

Run `make -C drivers/incluster help` for all targets.

## License

Copyright 2025. Licensed under the Apache License, Version 2.0.
