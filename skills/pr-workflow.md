---
name: pr-workflow
description: >
  Use when opening, monitoring, or iterating on a pull request in MAY,
  including PR body template, test and coverage requirements, CI interpretation,
  and commit conventions.
---

# PR Workflow

Full life-cycle reference for pull requests in the MAY repository.

## Overview

MAY is a service composed of several Kubernetes controllers. PRs target `main` on the upstream repository.

## When to Use

- About to open a PR or write a PR description
- Iterating on a PR based on review feedback
- Preparing commits for a change

## Definition of Done

A code change is not ready to report as complete, push, or open a PR until:

- `make -C may test` or `make -C drivers/<name> test` passes for every affected module.
- Existing specs are updated for changed behavior.
- New behavior, branches, and error paths have new specs.
- Coverage is part of the change: inspect `cover.out` and cover the new code. For the core module, `make -C may report-coverage` produces the HTML report.
- Cluster-facing changes have the relevant e2e target run, or the PR states why it could not run.
- Ginkgo/Gomega tests follow `skills/testing-standards.md`.

Do not report coding work as complete from a code read or a compile-only check; run the checks yourself. If a test cannot run, say so explicitly and state what remains unverified.

## Branch Setup

If a dedicated branch already exists for this work, use it. Otherwise, create a branch from the latest main of `konflux-ci/may`.

First, find which remote points to `konflux-ci/may`:

```bash
git remote -v | grep konflux-ci/may
```

Then fetch and branch from it:

```bash
git fetch <remote>
git checkout -b <branch-name> <remote>/main --no-track
```

Common setups: fork-based workflows typically name it `upstream`, while direct clones use `origin`.

Never branch from an old or diverged main.

## PR Body Template

Every PR follows this structure:

```markdown
## What

<concise list of what changed>

[KFLUXINFRA-1234](https://redhat.atlassian.net/browse/KFLUXINFRA-1234)

## Why

<motivation — why this change is needed>

## Testing

- <which tests cover this change and their results>
- <e.g., "Added Ginkgo specs for the new Runner status path — `make -C may test` passes">
- <e.g., "Existing e2e coverage in `may/test/e2e/statichost_test.go` covers this path — `make -C may test-e2e` passes">
```

**Rules:**
- **What** — Concise change list and Jira link. Don't explain why here.
- **Why** — Motivation only. Keep it brief.
- **Testing** — Show that the required tests were run and passed: the exact `make` target or targets, the modules covered, and the result. State which existing specs were updated and which new specs cover new behavior. If an e2e target could not run, say why.

## Commit Conventions

Prefix every commit with the Jira key:

```
KFLUXINFRA-1234 short description of the change
```

Always use `-s` flag (DCO sign-off).

Trailers (at end of commit message body). Use the actual agent/tool identity:
- Interactive sessions (human + agent): `Assisted-by:` trailer
- Agentic workflow (autonomous): `Authored-by:` trailer

## Key CI Checks

| Check | What It Does |
|-------|--------------|
| **unit-tests** | Ginkgo tests with coverage, uploads to Codecov. |
| **e2e** | e2e tests on Kind clusters with e2e coverage. |
| **go-tidy** | Verifies `go.mod` and `go.sum` are tidy. |
| **lint-go** | golangci-lint with version pinned in Makefile. |
| **Tekton pipelines** | Multi-arch container builds and security scans (Clair, Snyk, Coverity, ClamAV). Run inside Konflux — best investigated manually through the Konflux UI. |
| **agent-files-detect** | Blocks PRs that add vendor-specific agent directories (`.claude/`, `.cursor/`, `.vscode/`, `.agents/`) and detects changes to protected agent config files (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `skills/`, workflow files). |
| **agent-files-enforce** | Applies the `agent-config-review-required` label when protected agent config files are changed. Posts the `Agent File Policy` commit status — blocks merge until a write-access reviewer removes the label. |

**CI caveats:**
- E2E tests can be flaky due to Kind cluster setup — if logs show no relevant errors, rerun with `gh run rerun <run-id> --failed`.
- Tekton pipeline failures are best investigated manually through the Konflux UI. Use `/retest` to re-trigger, but don't try to diagnose these yourself.

## Interactive Sessions

In interactive sessions (human + agent), always confirm with the human before pushing and opening the PR. Show them the commit message, PR title, and PR body for approval first. Never push or create a PR without explicit approval.

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Reporting work complete or pushing without running tests | Run `make -C may test` or `make -C drivers/<name> test` and record the result in Testing. |
| Adding code without tests or coverage | Update existing specs and add specs for new behavior; inspect `cover.out`. Follow `skills/testing-standards.md`. |
| Putting explanation in Testing instead of evidence | Testing = which tests ran and passed. Why = explanation. |
| Branching from a stale main | Always fetch and reset from origin before branching. |
| Missing Jira key in commit message | Prefix every commit with `KFLUXINFRA-1234`. |
| Running tests with `go test ./...` | Always use `make test` — it wraps Ginkgo with the project's preferred flags and configuration. |
