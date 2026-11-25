# MAY

MAY is a Kubernetes-native system for management of remote multi-architecture runners.

## Project Layout

- `demo` - Scripts and tools to demo MAY
- `drivers/aws` - Kubernetes Controller to manage Instances on AWS Cloud
- `drivers/ibm` - Kubernetes Controller to manage Instances on IBM Cloud
- `drivers/incluster` - Development only Kubernetes Controller to manage Instances in cluster
- `may/` - MAY's core Kubernetes Controller

## Gotchas

- Kubernetes Controllers in this repo have their own AGENTS.md. Refer to them for scoped information.

## Makefile Test Target Conventions

When adding or modifying Makefile test targets (`test-*`), follow these rules:

1. **Framework-specific flags must only target compatible packages.**
   Flags like `-ginkgo.label-filter`, `-ginkgo.focus`, and `-ginkgo.v` are
   registered by the Ginkgo test framework. Passing them via `go test ./...`
   fails on packages whose test binaries do not import
   `github.com/onsi/ginkgo/v2`. Either scope the target to specific
   packages (e.g., `./test/e2e/`) or use the `ginkgo` CLI directly.

2. **Quote `$(ENVTEST)` and `$(LOCALBIN)` inside `$(shell …)` calls.**
   Driver Makefiles use quoted variable references in the test recipe:
   ```makefile
   KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test ...
   ```
   New test targets must follow this pattern. Unquoted references break
   when paths contain spaces.

3. **Include `-covermode atomic` in all test targets that generate
   `cover.out`.** This matches CI behavior. Omitting it defaults to
   `set` mode, which produces incorrect results under concurrent tests.

4. **Make each target's `## …` help comment unique.** The `make help`
   output is generated from these comments. Duplicate descriptions
   make targets indistinguishable in help output.

## Protected Files

Changes to the following paths require a write-access reviewer to approve them. The `agent-config-review-required` label is applied automatically, and merge is blocked until a reviewer removes the label.

- `AGENTS.md` (at any level)
- `CLAUDE.md` (at any level)
- `GEMINI.md` (at any level)
- `skills/` (at any level)
- `.github/workflows/agent-files-detect.yaml`
- `.github/workflows/agent-files-enforce.yaml`

## Skills

- Before opening a PR, writing a PR description, or interpreting CI results, read `skills/pr-workflow.md`.
- When a CI check fails on a PR, read `skills/ci-troubleshooting.md`.
- When working interactively on new features or significant changes, read `skills/brainstorming-workflow.md` before making changes.
