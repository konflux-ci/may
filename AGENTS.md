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

## Protected Files

Changes to the following paths require a write-access reviewer to approve them. The `agent-config-review-required` label is applied automatically, and merge is blocked until a reviewer removes the label.

- `AGENTS.md` (at any level)
- `CLAUDE.md` (at any level)
- `GEMINI.md` (at any level)
- `skills/`
- `.github/workflows/agent-files-detect.yaml`
- `.github/workflows/agent-files-enforce.yaml`

## Skills

- Before opening a PR, writing a PR description, or interpreting CI results, read `skills/pr-workflow.md`.
- When a CI check fails on a PR, read `skills/ci-troubleshooting.md`.
- When working interactively on new features or significant changes, read `skills/brainstorming-workflow.md` before making changes.
