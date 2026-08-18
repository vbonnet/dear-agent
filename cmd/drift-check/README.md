# drift-check

<!-- Last audited at: 2026-06-15 -->

Detects **deployment drift**: deployed artifacts on the host whose source of
truth in this repo no longer matches the copy that was actually installed — a
fix that merged to main but never reached the machine.

[`bead-close-guard`](../bead-close-guard) consumes this tool's `targets.yaml`
as the deployed-file gate: a bead whose merged change touches a target's
`source` cannot close until that target is current on the host.

## Why

"Merged to main" is not "deployed on the host." Claude Code hooks, launchd
plists, and chezmoi-managed files ship by being copied/rendered onto the
machine, not imported as Go packages. When a redeploy step is skipped, the fix
lives in git but the host stays broken — silently, because CI is green and the
bead is closed. PR
[#456](https://github.com/vbonnet/dear-agent/pull/456) merged a gopls reaper
into the stop hook that was never redeployed, leaking processes for two days
(ce-710r).

## How it works

For each configured target it hashes (SHA-256) the deployed file and the
source-of-truth file and reports a status:

| Status             | Meaning                                                        |
|--------------------|----------------------------------------------------------------|
| `ok`               | deployed file matches source                                   |
| `drift`            | both exist, hashes differ — deployed copy is stale             |
| `missing_deployed` | source exists, nothing deployed (drift if required; else skip) |
| `missing_source`   | configured source path not in repo — a config error            |
| `error`            | could not evaluate (read failure)                              |

The check is **just file hashing** — no builds, no network beyond an optional
`git show`. It is cheap enough for an audit or the bead-close gate.

## Usage

```sh
make drift-check                 # build and run against the built-in targets
drift-check                      # human-readable report
drift-check --json               # structured report (schema drift-check/v1)
drift-check --audit              # also append to the JSONL audit log
drift-check --git-ref origin/main # compare against committed main, not the working tree
drift-check --config my.yaml     # use a custom deploy-target config
```

Exit codes: `0` no drift · `2` drift detected · `1` error.

## Configuring deploy targets

Targets are declared in [`targets.yaml`](./targets.yaml) (embedded as the
default; override with `--config`). Each target pairs a `deployed` path on the
host with a repo-relative `source`, plus optional `tokens` (to render templated
sources like plists), a `remediation` command, and an `optional` flag. See the
comments in `targets.yaml`.

## Output

The JSON report (`--json`) is the contract for monitoring and lifecycle gates:
a `summary` tally plus a per-target list carrying the
hashes, a human-readable `diff`, and the `remediation` command. `--audit`
writes one JSONL line per run to `~/.local/state/dear-agent/drift-check.log`.
