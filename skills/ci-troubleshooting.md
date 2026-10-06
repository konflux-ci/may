---
name: ci-troubleshooting
description: >
  Use when a CI check fails on a PR in MAY and you need to
  understand what failed, how to read the logs, and how to fix it.
---

# CI Troubleshooting

## Overview

How to investigate and fix CI failures on MAY PRs.
MAY is a service composed of several Kubernetes controllers, each of them lives at a different path.
CI targets consistently all of them.

## When to Use

- A CI check failed on your PR
- You need to understand what a CI comment or status means
- You want to re-trigger a flaky test

## Prerequisites

Verify `gh` CLI is installed and authenticated:

```bash
gh auth status
```

All CI investigation commands below depend on it.

## Reading CI Logs

### GitHub Actions checks

```bash
gh pr checks <PR-number> --repo konflux-ci/may
```

To investigate a failed check:

```bash
gh run view <run-id> --repo konflux-ci/may
gh run view <run-id> --repo konflux-ci/may --log-failed
```

### Tekton / Konflux pipeline checks

Tekton pipeline runs (pull-request and push pipelines defined in `.tekton/`) execute inside the Konflux platform. These are best investigated manually through the Konflux UI.

If a Tekton pipeline check fails, you can comment `/retest` on the PR to re-trigger, but if the failure persists, escalate to a human who can inspect the logs in the Konflux UI.

## Common Failures

### unit-tests

Ginkgo test failures. The GitHub Actions log shows which specs failed and the failure messages. To reproduce locally:

```bash
make test -C <PATH>
```

### e2e

To reproduce locally:

```bash
make -C <PATH> test-e2e
```

If logs show no relevant errors and the failure looks intermittent, rerun the failed job:

```bash
gh run rerun <run-id> --repo konflux-ci/may --failed
```

### go-tidy

`go.mod` or `go.sum` are not tidy. Fix:

```bash
go mod -C <PATH> tidy
```

Then commit the changes.

### lint-go

golangci-lint violations. The log shows the exact file, line, and linter rule. Fix the reported issues or run locally:

```bash
make -C <PATH> lint
```

to fix the lint errors automatically:

```bash
make -C <PATH> lint-fix
```

### agent-files-detect

This check has two jobs:

**block-vendor-dirs** — Fails if the repository tree contains any vendor-specific agent directories (`.claude/`, `.cursor/`, `.vscode/`, `.agents/`). To fix, remove the offending directory and use `AGENTS.md` and `skills/` instead.

**detect-protected-changes** — Detects changes to protected agent config files (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `skills/`, `.github/workflows/agent-files-detect.yaml`, `.github/workflows/agent-files-enforce.yaml`). In addition to checking which files were modified, the job scans the diff content for references to protected path strings (e.g. a script that writes to `AGENTS.md`, mentions `skills/`, or references `.claude`). A PR that does not modify any protected file but whose diff content mentions one of these paths will also be flagged. This job does not block the PR directly — it passes its result to `agent-files-enforce`.

### agent-files-enforce

Applies the `agent-config-review-required` label when the detect workflow finds protected file changes, and posts the `Agent File Policy` commit status. The PR is blocked while this label is present.

To resolve:

1. A reviewer with write access to the repository must review the agent config changes.
2. Once approved, the reviewer removes the `agent-config-review-required` label.
3. The `Agent File Policy` status updates to success and merge is unblocked.

If the label was applied but you have reverted all protected file changes, push the revert — the enforce workflow removes the label automatically when the PR no longer modifies protected paths.

If the enforce workflow blocks a PR that does not appear to change protected files, the detection artifact may have been missing or invalid — the enforce workflow fails closed in this case. Re-push to trigger a fresh detect run.

### Tekton pipeline failures

The `.tekton/` pipelines run security scans (Clair, Snyk, Coverity, ClamAV, SAST) and multi-arch container builds. These run inside Konflux and their logs are not accessible from the CLI. If they fail:

1. Comment `/retest` on the PR to re-trigger.
2. If the failure persists, these are best investigated manually through the Konflux UI by a human.
