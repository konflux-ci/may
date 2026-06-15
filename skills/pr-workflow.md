---
name: pr-workflow
description: >
  Use when opening, monitoring, or iterating on a pull request in MAY,
  including PR body template, CI interpretation, and commit conventions.
---

# PR Workflow

Full life-cycle reference for pull requests in the MAY repository.

## Overview

MAY is a service composed of several Kubernetes controllers. PRs target `main` on the upstream repository.

## When to Use

- About to open a PR or write a PR description
- Iterating on a PR based on review feedback
- Preparing commits for a change

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
- <e.g., "Added Ginkgo specs for the new handler — `make test` passes">
- <e.g., "Existing acceptance tests in dumb-proxy/smart-proxy cover this path — both pass">
```

**Rules:**
- **What** — Concise change list and Jira link. Don't explain why here.
- **Why** — Motivation only. Keep it brief.
- **Testing** — Show that appropriate tests were run and passed. This can be new tests you added or existing tests that cover the change. Reference the specific test type (unit, performance, acceptance) and confirm they pass.

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

**CI caveats:**
- Acceptance tests can be flaky due to Kind cluster setup — if logs show no relevant errors, rerun with `gh run rerun <run-id> --failed`.
- Tekton pipeline failures are best investigated manually through the Konflux UI. Use `/retest` to re-trigger, but don't try to diagnose these yourself.

## Interactive Sessions

In interactive sessions (human + agent), always confirm with the human before pushing and opening the PR. Show them the commit message, PR title, and PR body for approval first. Never push or create a PR without explicit approval.

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Not running tests before pushing | Run `make -C <PATH> lint test test-e2e` locally, mention results in Testing. |
| Putting explanation in Testing instead of evidence | Testing = which tests ran and passed. Why = explanation. |
| Branching from a stale main | Always fetch and reset from origin before branching. |
| Missing Jira key in commit message | Prefix every commit with `KFLUXINFRA-1234`. |
| Running tests with `go test ./...` | Always use `make test` — it wraps Ginkgo with the project's preferred flags and configuration. |
