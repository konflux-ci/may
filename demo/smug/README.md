# MAY demos with Smug

This folder runs guided MAY demos using [Smug](https://github.com/ivaaaan/smug), a session manager for tmux. Each demo opens a tmux session with multiple windows and panes: one to drive the demo (apply/delete resources step by step) and others to watch MAY resources, PipelineRuns, and related objects in real time.


---

## Prerequisites

A cluster must already be running with MAY, the incluster driver, Kueue, Tekton, and Tekton-Kueue installed (e.g. via `demo/hack/setup-cluster.sh` from the repo root).

Required Tools:
* `tmux`
* `smug`: the `demo/smug/demo.sh` script will install it locally via `go` if not found
    * `go`: if `smug` needs to be installed locally

---

## Run the demos

> Use tmux as usual: switch windows with `Ctrl+b` then the window number or arrow keys

To run the StaticHost demo

```bash
./demo/smug/demo.sh static
```

To run the DynamicHost demo

```bash
./demo/smug/demo.sh dynamic
```

### Optional: run in your current terminal

By default, Smug starts a new tmux session. To run inside your current terminal session:

```bash
IN_CURRENT_SESSION=true ./demo.sh static
```

---

## Static demo

Use this to see MAY with a fixed pool of runners (one host, multiple runners).

**Session name:** `may-demo-static`

Uses **StaticHosts**: a fixed set of runners on a single host (e.g. ARM64). Flow:

1. Press Enter to start the demo.
2. Apply tenant namespace and sample PipelineRuns: `config/static/tenant-pipelinerun-may-static`.
3. Apply the static host (ARM64): `config/static/hosts/arm64/`. The session switches to the **hosts** window.
4. Press Enter when ready to tear down.
5. PipelineRuns are deleted, then the StaticHost `aws-host-arm64` and the hosts kustomization.

The session has:

- **demo-run** — Interactive pane: follow the prompts (Enter) to apply/delete resources; it switches to **hosts** for the cleanup step.
- **hosts** — Runner pods (labeled for `aws-host-arm64`), Runners, StaticHosts in `may-system`, and PipelineRuns in `pipelinerun-may-tenant-static`.
- **demo-watch** — Runners, Claims, Pods, TaskRuns, Kueue Workloads, and PipelineRuns in the tenant namespace.

---

## Dynamic demo

Use this to see MAY scale runners up and down with workload.

**Session name:** `may-demo-dynamic`

Uses **DynamicHosts** and **DynamicHostAutoscaler**: runners (and hosts) are created on demand (e.g. AMD64). Flow:

1. Press Enter to start the demo.
2. Apply the dynamic host autoscaler (AMD64): `config/dynamic/hostautoscaler/amd64/`. The session switches to the **hosts** window.
3. Press Enter to install PipelineRuns.
4. Apply tenant namespace and sample PipelineRuns: `config/dynamic/tenant-pipelinerun-may-dynamic`. The session switches to **demo-watch**.
5. Press Enter when ready to tear down PipelineRuns; they are deleted.
6. Press Enter to remove the HostAutoscaler; `config/dynamic/hostautoscaler/amd64/` is deleted.

The session has:

- **demo-run** — Interactive pane: follow the prompts (Enter) to apply the autoscaler and PipelineRuns, then to delete PipelineRuns and the HostAutoscaler; it switches to **hosts** and **demo-watch** at the right times.
- **hosts** — DynamicHostAutoscalers; runner pods (labeled for `aws-host-amd64`), Runners, and DynamicHosts in `may-system`.
- **demo-watch** — Runners, DynamicHosts, Claims, TaskRuns, Pods, Kueue Workloads, and PipelineRuns in `pipelinerun-may-tenant-dynamic`.

