# MAY — Core controller

MAY is the core controller of [MAY (Multi-Architecture Y?)](../../README.md). It defines the **Claim**, **Runner**, and abstract **Host** contract, and runs the reconcilers that schedule workloads onto runners and bind them.

**See the [repository root README](../../README.md)** for concepts, high-level flow, and quick start.

## What this component does

- **api/v1alpha1**: CRDs — Claim, Runner, Host (shared types), StaticHost, DynamicHost, DynamicHostAutoscaler.
- **internal/controller**: Single controller with multiple reconcilers:
  - **Claimer**: creates Claims for Pods in tenant namespaces.
  - **Gater**: admission/flow control; prevents Pods from being scheduled until their Claims are bound.
  - **Scheduler**: assigns Claims to Runners.
  - **Binder**: provides Runner credentials / binding to workloads.
  - **Provisioner**: Runner lifecycle on Hosts (delegates host implementation to drivers).

Host implementation is delegated to **drivers** (e.g. `drivers/incluster/`).

## Prerequisites

- Go 1.24+
- Docker (or another container runtime)
- kubectl, Kind (for demo)

## Build, test, deploy

```sh
# Unit tests
make -C may test

# E2E tests
make -C may test-e2e

# Install CRDs
make -C may install

# Build and deploy (set IMG to your image)
make -C may docker-build docker-push IMG=<registry>/may:tag
make -C may deploy IMG=<registry>/may:tag
```

Samples: `kubectl apply -k may/config/samples/`.

Run `make -C may help` for all targets. See [Kubebuilder documentation](https://book.kubebuilder.io/introduction.html) for more.

## License

Copyright 2026. Licensed under the Apache License, Version 2.0.
