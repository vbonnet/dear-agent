# DEAR Retrospective — Recurring host disk ballooning

**Date**: 2026-08-07
**Class**: Silent failure of an unattended reaper
**Severity**: P0 — host crashed twice on 2026-07-03 (ce-uxju), 239 GB leaked by 2026-07-31
**Beads**: ce-uxju, ce-lzs0, ce-te29, ce-mjf9, ce-4hk0, ce-93lw.18 (existing) · ce-2a3ma, ce-3ndic, ce-kracc, ce-5byda, ce-t4oid, ce-r63wo, ce-2lrmg (filed today)
**PR**: `fix/sandbox-gc-workspace-resolution`

---

## Detect

Disk free space kept collapsing overnight despite repeated manual reclaim. The
presenting complaint was "reclaimed ~9 GB of worktrees and 332 MB of archived
clones yesterday; back from ~40 GB free to ~16 GB overnight."

The framing itself was a symptom worth noting: the operator had been reclaiming
in the wrong order of magnitude. The two largest consumers on the host were
never in the manual sweep at all.

Measured ranking (2026-08-07, after an intervening week and a partial Codex
reclaim):

| Rank | Path | Size | Nature |
|---|---|---|---|
| 1 | `~/Library/Caches/go-build` | **54 GB** | Derived, unbounded |
| 2 | `~/worktrees` (297 dirs) | **39 GB** | Mixed; some unlanded work |
| 3 | `~/Library/Application Support` | 31 GB | App data |
| 4 | `~/.colima` | 20 GB | VM image |
| 5 | `~/src` (41 golden checkouts) | 16 GB | Protected source |
| 6 | `~/.agm/sandboxes` (17 dirs) | 12 GB | Reapable |
| 7 | `~/.codex` | 10 GB | Session rollout JSONL |
| 8 | `~/go/pkg/mod` | 5.2 GB | Derived |

A week earlier the same measurement had `~/.agm/sandboxes` at **239 GB across
119 dirs** — 60% of all used space on the volume.

## Establish (root cause)

Three findings, in ascending order of how much they matter.

### 1. The proximate cause: the reaper was dead, loudly, into a file nobody read

`agm sandbox gc` is a well-built tool. It has five safety gates, dry-run by
default, path allowlisting, mount-table re-reads, and an age floor. It is
installed as `com.dear-agent.sandbox-gc`, scheduled hourly, and it had been
**exiting non-zero on every single tick since 2026-07-05**.

`~/.local/state/dear-agent/sandbox-gc.err.log`, 187 KB of it, three successive
causes:

| Count | Error |
|---|---|
| 19× | `unknown flag: --reap` — plist passed a flag the installed binary lacked |
| 314× | `failed to connect to Dolt: database not found: personal` |
| 55× | `multiple enabled workspaces require ... dolt.port` |

Each failure was *correct behaviour*. The GC fails **closed**: it refuses to
reap when it cannot prove which sessions are live, because reaping a sandbox
whose owning session lives in an unreadable store is data loss. That is the
right trade. But it means a fail-closed reaper and a healthy idle reaper are
**observationally identical from the outside** — both quietly do nothing.

The registry had two enabled workspaces (`oss`, 1447 sessions; `personal`, 0
sessions, Dolt DB absent) and neither carried a `dolt.port`. Setting
`DOLT_PORT=3307` resolves it, and the already-existing graceful-degradation
path then kicks in: `workspace "personal" skipped: Dolt database does not
exist`, and the reachable workspace is swept normally.

### 2. The real cause: nothing consumed the failure — and we had already learned this

`disk-watchdog` runs every 5 minutes and reported `Status: OK` throughout,
because its only question is "is free space below 20 GiB?"

Read the `disk-watchdog` package doc:

> on 2026-07-18 this watchdog was in ALARM with `agm worktree sweep --execute:
> signal: killed` in every remediation slot — the remediation path was being
> killed by the exhaustion it existed to relieve — and **nothing consumed that
> fact**, so the mesh kept spawning into a wedged host.

That is ce-93lw.18. It was fixed three weeks before this incident, by making
the watchdog latch the admission brake on its own remediation failure.

**The identical bug existed one layer down, in the GC that watchdog depends on,
and the fix was never generalised.** We diagnosed "an unattended maintenance
job fails and no one notices", fixed the single instance in front of us, and
did not ask where else the shape occurs. `launchctl list` shows
`com.dear-agent.token-refresher` at last-exit=1 right now — plausibly a third
live instance.

### 3. The amplifier: the safety mechanism required the resource it protects

From `disk-watchdog.err.log`:

```
could not engage admission brake: writing temp brake file:
  write /Users/vbonnet/.agm/.admission-brake-2868909151.tmp: no space left on device
```

The admission brake halts worker spawning under disk pressure. It engages by
writing a temp file and renaming it. **Disk exhaustion is both the condition it
exists to handle and the condition that prevents it from engaging.** So at the
moment the mesh most needed to stop spawning 2 GB sandboxes, the brake was
unavailable, and the failure was a stderr warning that by design does not change
the exit code.

### Why the leak was so fast

Each sandbox is ~2.0 GB, and the bulk is a full independent git clone:
identical pack hashes (`pack-b4d4765602cd...`, 338 MB; `pack-a200a1c2502b...`,
142 MB) plus a 68 MB `bin/agm`, re-materialised per worker. On 2026-07-29,
94 sandboxes were created on a ~5.8-minute cadence — **~190 GB in one day**.

A broken reaper plus 190 GB/day of byte-identical duplication is not a slow
drift; it is a host-crash timer.

## Act

### Reclaimed (this session)

| Action | Reclaimed | Safety basis |
|---|---|---|
| `go clean -cache` | **54 GB** (54 GB → 12 KB) | Pure derived build artifacts |
| `agm sandbox gc --reap` (manual, then hourly) | **~9 GB**, 17 → 7 sandboxes | The tool's own five gates |

**Free space: 160 GiB → 206 GiB.** Container free 171.4 GB → 230+ GB.

**Not** reclaimed, deliberately: 297 worktrees / 39 GB (ce-3ch7: the sweep's
MERGED oracle misclassifies live worktrees — 39 GB is not worth destroying
unlanded work); `~/.codex` session rollouts / 10 GB (user conversation history);
anything under `~/src`.

### Landed

PR `fix/sandbox-gc-workspace-resolution`:

1. **Reaper liveness in `disk-watchdog`** — alarms when the GC has not
   completed a sweep within `--gc-max-age` (default 6h), independently of free
   space, quoting the last GC error so the alarm is actionable. Free space is a
   *lagging* indicator; reaper staleness is the *leading* one.
2. **A completion heartbeat** — `sandbox_gc_completed` records every successful
   sweep. Previously a sweep that reaped nothing wrote nothing, so "last ran at
   T" was indistinguishable from "dead since T". There was no heartbeat to
   check staleness against, which is why staleness detection did not already
   exist.
3. **`DOLT_PORT` in the GC plist** — the config that unblocks the inventory.

11 new tests. Deliberately *not* changed: the fail-closed guard in
`configuredWorkspaceConfigsFromRegistry`. Loosening it would trade a disk leak
for a data-loss risk, which is the wrong direction.

### Verified live

The plist was restaged and the job reloaded. `launchctl list` went from
last-exit **1** to **0**; a forced run reaped a sandbox and exited clean. The
hourly reaper is alive for the first time since 2026-07-05.

The new `disk-watchdog` binary was built, verified against the live host —
correctly reporting `sandbox GC: NEVER completed a sweep ... last GC error:
multiple enabled workspaces require ...` — and then **rolled back to the
main-branch build**. Shipping it before the matching `agm` heartbeat would
produce a known-false alarm every 5 minutes, which is precisely the
alert-fatigue pattern this retro is about. Both ship together when the PR lands.

## Reflect — harness findings for dear-agent

**F1. Fail-closed is only half a design. (P0, ce-2a3ma)**
Every fail-closed safety gate needs a liveness signal, or it degrades into
fail-silent. "Refuses to act when it cannot prove safety" and "is not running"
must be distinguishable from outside the process. The rule: *any component
whose correct behaviour under failure is to do nothing must publish a heartbeat
when it does something.*

**F2. Fix the class, not the instance. (P0, ce-2a3ma)**
ce-93lw.18 diagnosed this exact shape three weeks earlier and fixed one
instance. The retro produced a fix, not a sweep for other occurrences. When a
retro's root cause is structural ("nothing consumes X's failure"), the DoD
should require enumerating every component with that structure. Concretely:
audit all 26 launchd jobs for last-exit status and heartbeat coverage.

**F3. `launchctl list` exit status is free monitoring nobody reads. (P0)**
The signal was sitting in column 2 for a month. A trivial periodic check —
"any `com.dear-agent.*` or `com.vbonnet.*` job with non-zero last exit" — would
have caught this on day one, and would have caught the `--reap` flag skew on
day one of *that* regression too. It also flags `token-refresher` right now.

**F4. Alarm on rate-of-change, not just absolute thresholds. (P2, ce-r63wo)**
Static floors (20 GiB / 5 GiB on a 460 GiB volume) cannot see a 190 GB/day
leak until ~95% of headroom is gone — by which point the remediation itself
gets killed by the exhaustion (ce-93lw.18). Losing 100 GiB in 24h is actionable
at 300 GiB free.

**F5. A safety mechanism must not depend on the resource it rations. (P1, ce-5byda)**
The admission brake could not engage because the disk was full. Pre-allocate
it, or reserve ballast. Generalise: audit safety mechanisms for dependence on
the exhaustible resource they protect.

**F6. Remediation levers must cover the actual top consumers. (P1, ce-3ndic)**
`disk-watchdog`'s only lever is `agm worktree sweep`. On this host the top
consumer was a 54 GB Go cache and #6 was sandboxes — neither reachable by that
lever. A watchdog whose remediation cannot touch the biggest consumers can
alarm forever without helping.

**F7. "Built but never wired" is a recurring failure mode. (P1, ce-kracc)**
`cmd/log-rotate` exists and is not installed. This matches prior findings
(repo-health-audit had no dispatcher; the May cleanup retro shipped tooling and
never wired the Stop hook). Suggested DoD addition: a tool is not done until
something invokes it on a schedule *and* its non-invocation is detectable.

**F8. Beads filed ≠ risk retired.**
There were already ~10 open P0/P1 beads on this exact class — ce-uxju even
records "2.3T leak crashed host 2x". The backlog knew. Filing is not
mitigation, and a P0 that has been open across two host crashes is evidence the
queue is not being drained by severity.

**F9. Guardrails blocked deploying the fix they protect.**
The `fs-write-guard` correctly refused a direct write to
`~/Library/LaunchAgents`, and `bash-write-guard` refused shell redirection into
`~/.agm`. Both were *right*. The sanctioned path existed
(`make install-sandbox-gc-launchagent`) but had to be discovered, and it pulls
in `install-agm` as a dependency, which mutates far more host state than a
plist restage. Worth splitting deploy targets so config-only changes do not
require a binary reinstall.

---

## Timeline

| When | What |
|---|---|
| 2026-07-03 | `~/.agm/sandboxes` hits 2.3 TB / 541 dirs; host crashes twice (ce-uxju) |
| 2026-07-05 | `sandbox-gc.err.log` opens; the GC begins failing every hour |
| 2026-07-18 | ce-93lw.18: watchdog remediation fails, "nothing consumed that fact"; fixed for the watchdog only |
| 2026-07-29 | 94 sandboxes created in one day (~5.8 min cadence, ~190 GB) |
| 2026-07-31 | Forensics: 239 GB / 119 sandboxes = 60% of used disk; 38 GiB free |
| 2026-08-07 | Root cause found; 54 GB Go cache + ~9 GB sandboxes reclaimed; GC restored; 206 GiB free |
