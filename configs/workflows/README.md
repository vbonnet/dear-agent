# Workflow templates

Stateless workflow definitions consumed by `workflow-run`. Each YAML
file defines one workflow. Schedule them externally (cron, launchd,
systemd timer); the engine itself does not own a scheduler.

## Templates

| File                | What it does                                                    |
|---------------------|-----------------------------------------------------------------|
| `audit-daily.yaml`  | Single-repo daily DEAR audit (cwd-scoped).                      |
| `audit-weekly.yaml` | Single-repo weekly DEAR audit.                                  |
| `audit-monthly.yaml`| Single-repo monthly DEAR audit (heavy checks).                  |
| `audit-multi.yaml`  | Same DEAR audit across a list of repos in one invocation.       |
| `signals-collect.yaml` | Periodic `dear-agent-signals collect` across configured repos. |

## Why workflows for periodic jobs

Wrapping `dear-agent-signals collect` (or `workflow-audit run`) in a
workflow instead of calling it directly from cron buys you the engine's
audit log, retry policy, and run history for free. The trade-off is one
extra layer; the win is observability — `workflow status` and
`workflow logs <run-id>` answer "did last night's collection succeed?"
without a separate per-job log file.

## Scheduling

There is no `schedule:` field in the template YAML. Wire one of the
templates into your platform's scheduler:

```cron
# Signal collection — daily at 03:00
0 3 * * *  workflow-run -file configs/workflows/signals-collect.yaml -trigger cron

# DEAR audit — daily at 04:00, weekly on Sunday at 04:30, monthly on the 1st
0  4 * * *  workflow-run -file configs/workflows/audit-multi.yaml -input cadence=daily   -trigger cron
30 4 * * 0  workflow-run -file configs/workflows/audit-multi.yaml -input cadence=weekly  -trigger cron
0  5 1 * *  workflow-run -file configs/workflows/audit-multi.yaml -input cadence=monthly -trigger cron
```

The `-trigger cron` flag is recorded on the runs row so ad-hoc
invocations can be distinguished from scheduled ones in `workflow
list` / `workflow status`.

## Partial-failure semantics

The multi-repo templates (`audit-multi.yaml`, `signals-collect.yaml`)
loop over the configured `repos` input internally. Each repo's exit
code is captured per iteration; the workflow exits non-zero only when
*every* repo failed. Partial success — some OK, some FAILED — exits 0
by design so a single broken repo does not suppress fresh signals from
the others. Cron-style alerting still fires for total breakage.

## Inputs

Override defaults at run time with `-input key=value`:

```sh
workflow-run \
  -file configs/workflows/signals-collect.yaml \
  -input db="$HOME/.local/share/dear-agent/signals.db" \
  -input repos="dear-agent,brain-v2,engram" \
  -input lookback_days=14
```

Inputs are exposed inside `bash` nodes as `$INPUT_<name>` env vars; the
`{{ .Inputs.<name> }}` template form is shell-injectable and avoided
in the shipped templates (see the warning in
`pkg/workflow/runner.go:865`).
