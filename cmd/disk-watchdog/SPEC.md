# Disk Watchdog Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`
- Reaper-liveness evidence: `cmd/disk-watchdog/reaper_liveness_test.go` (DW-17..DW-23),
  `cmd/disk-watchdog/reaper_liveness_review_test.go` (DW-19, DW-24, DW-25),
  `cmd/disk-watchdog/reaper_liveness_bounds_test.go` (DW-26), and
  `cmd/disk-watchdog/reaper_liveness_honesty_test.go` (DW-27 through DW-31:
  widening past the tail, undetermined-is-not-never, self-produced heartbeats
  ignored and liveness read before remediation, a refused reap latching the
  brake, and a negative window rejected as a usage error). The producer half of
  the refusal and producer-tag wire contracts is pinned in
  `agm/internal/ops/sandbox_gc_test.go` and `agm/internal/gclog/gclog_test.go`
  (SGC-17), since `cmd/` cannot import `agm/internal/...`.
- Build-cache reaper evidence: `cmd/disk-watchdog/buildcache_test.go` (DW-32..DW-38).
- E2E-cache reaper evidence: `cmd/disk-watchdog/e2ecache_test.go` (DW-39..DW-42).
- Preflight-scratch reaper evidence: `cmd/disk-watchdog/preflight_scratch_test.go` (DW-43..DW-47).

<!-- Last audited at: 2026-08-14 -->

## Purpose

`cmd/disk-watchdog` is the host-level backstop of the 2026-07-03 disk-full P0
(ce-6fel): a launchd-driven tick that samples disk free space and inode usage,
alarms on the same thresholds the VROOM Overseer classifies in-process
(`supervisor.DiskAlertThresholds.Classify` — one shared classifier, so the two
layers can never disagree), records the alarm to the decision trail, and
remediates through the existing safe hook `agm worktree sweep --execute`
(provably-merged clean worktree husks only). It runs independently of the VROOM
mesh because a full disk is exactly the failure state that takes the mesh down.

Since ce-93lw.18 it also drives the cross-process **admission brake**
(`pkg/vroom/admission`). Alarming was never the gap: on 2026-07-18 this watchdog
was in ALARM with root at 96.2% used and
`agm worktree sweep --execute: signal: killed` in every remediation slot — the
remediation path being killed by the exhaustion it existed to relieve — and
nothing consumed that fact, so the mesh kept spawning until the host had to be
power-cycled. The brake is the consumer: a TTL'd latch every spawn path reads.

Since PR #1160 it also checks **reaper liveness**. Free space is a *lagging*
indicator of a leaked-sandbox problem: by the time it crosses the 20 GiB floor,
hundreds of GB have accumulated and the remediation path itself gets killed by
the exhaustion. A reaper that has stopped completing sweeps is the *leading*
indicator and is independent of current free space. Between 2026-07-05 and
2026-08-07 the hourly `agm sandbox gc` exited non-zero on every tick — 388
failures into an unread log — while `~/.agm/sandboxes` reached 239 GB and every
tick here reported `Status: OK`. That is the same "nothing consumed that fact"
shape as ce-93lw.18, one layer down.

A stale reaper alarms and exits 1 but deliberately does **not** latch the brake
(DW-18): halting every spawn because a GC is behind would be a worse outage than
the leak it warns about. Only proof of a *real* sweep counts (DW-21) — a dry run
reclaims nothing and a sweep whose deletions all failed leaves the sandboxes in
place, so counting either would let a broken reaper suppress its own alarm.

## EARS Requirements

**DW-01** When free disk space on the measured filesystem falls below the critical floor (default 5 GiB), the system shall classify the condition as CRITICAL.

**DW-02** When free disk space on the measured filesystem falls below the warn floor (default 20 GiB) but not the critical floor, the system shall classify the condition as WARN.

**DW-03** When inode usage on the measured filesystem exceeds the critical ceiling (default 95%), the system shall classify the condition as CRITICAL.

**DW-04** When inode usage on the measured filesystem exceeds the warn ceiling (default 90%) but not the critical ceiling, the system shall classify the condition as WARN.

**DW-05** When the probe cannot measure the filesystem (a snapshot with zero free bytes and a zero used fraction), the system shall not raise an alarm.

**DW-06** When any threshold is breached, the system shall append one `watchdog.disk.alarm` record carrying the snapshot, reasons, thresholds, and remediation outcome to the decision trail.

**DW-07** When any threshold is breached and dry-run mode is not set, the system shall remediate by invoking `agm worktree sweep --execute` with JSON output.

**DW-08** When remediation or the trail append fails, the system shall report the failure and still exit with the breach exit code 1.

**DW-09** When no threshold is breached and the sandbox reaper is not stale, the system shall exit 0 without invoking any remediation.

**DW-10** While dry-run mode is set, the system shall detect and log breaches but the system shall not remove any worktree.

**DW-11** When any threshold is breached and remediation returns an error, the system shall engage the admission brake with a reason carrying the remediation error.

**DW-12** When no threshold is breached, the system shall release the admission brake only if it engaged that brake itself.

**DW-13** If the filesystem snapshot cannot be taken, then the system shall engage the admission brake before reporting the error.

**DW-14** When any threshold is breached and remediation succeeds, the system shall leave any existing admission brake in place.

**DW-15** While dry-run mode is set, the system shall not write or remove the admission brake file.

**DW-16** If engaging or releasing the admission brake fails, then the system shall report the failure on stderr and the system shall not change its exit code.

**DW-17** When the sandbox GC has not recorded proof of a completed sweep within the reaper-liveness window (default 6h), the system shall classify the condition as at least WARN and the system shall exit 1.

**DW-18** When the sandbox reaper is stale and no disk threshold is breached, the system shall not invoke remediation and the system shall not engage the admission brake.

**DW-19** When the sandbox reaper is stale and the sandbox GC recorded an error after its last proof of life, the system shall include that error in the alarm reason.

**DW-25** When a sandbox-GC completion record is rejected as proof of a completed sweep, the system shall record why it was rejected — dry run, deletion errors, or safety-probe failures — as the alarm's error state, so a responder can distinguish a dead schedule from one that runs and fails on every tick.

**DW-26** When scanning the sandbox-GC log, the system shall read at most a bounded tail of the file and shall resume at the first whole record inside that tail, so per-tick work does not grow with total log history and a long-lived host cannot starve later disk samples.

**DW-27** When the bounded tail contains no proof of a completed sweep and records older than the tail exist, the system shall widen the scanned window until it finds proof, reaches the start of the file, reads a record older than the reaper-liveness window, or reaches a hard byte cap — because record volume is not bounded by elapsed time and enough unrelated session-GC records can push a heartbeat that is well inside the SLA out of a fixed tail.

**DW-28** When the widening scan reaches its hard byte cap without proof of a completed sweep and without reading back past the reaper-liveness window, the system shall classify the reaper as stale and shall report the condition as undetermined liveness rather than as a reaper that never completed a sweep, because absent and could-not-determine are different findings with different causes.

**DW-29** When evaluating reaper liveness, the system shall ignore every sandbox-GC record this watchdog's own remediation produced, including per-candidate reap receipts (identified by the producer tag it sets on the sweep), and shall evaluate liveness before invoking remediation on the same tick, so that a remediating watchdog cannot accept its own sweep as proof that the scheduled reaper is alive. A record with no producer tag is not attributed to this watchdog.

**DW-30** When remediation invokes `agm sandbox gc --reap` and the sweep reports that the requested reap was refused or downgraded to a scan, the system shall treat the remediation as failed — it deleted nothing and its reap count means would-reap — and shall therefore engage the admission brake under DW-11.

**DW-20** While the reaper-liveness window is zero or the sandbox GC log path is empty, the system shall not evaluate reaper liveness.

**DW-31** When the configured reaper-liveness window is negative, the system shall reject it as a usage error and exit 2 rather than disabling the check, so a typo cannot leave a dead reaper unmonitored while every tick reports OK. Only zero disables the check (DW-20).

**DW-21** When evaluating reaper liveness, the system shall accept only a non-dry-run completion record with zero reap errors and zero probe failures as proof of a completed sweep. It may accept a sandbox reap record as legacy proof only when the log contains no completion record and no sandbox-GC error at or after that reap, so an aborted modern sweep cannot manufacture health through its partial-mutation receipt.

**DW-24** When a sandbox-GC completion record reports probe failures (a safety gate such as lsof, the mount table, or the session store could not be evaluated, as distinct from a gate that positively found a sandbox in use), the system shall treat that record the same as a record with reap errors: not proof of a completed sweep, so a reaper whose probes are systematically broken cannot suppress its own staleness alarm by reporting "kept" with zero errors.

**DW-22** When evaluating reaper liveness, the system shall ignore records that are not sandbox-GC operations and records timestamped beyond the clock-skew tolerance (5 minutes) ahead of the current time.

**DW-23** If the sandbox GC log cannot be read, then the system shall classify the sandbox reaper as stale.

**DW-32** The system shall reap abandoned Go-style build caches on every tick, regardless of whether any disk threshold is breached. A cache older than the age gate has no value — the next run creates its own — and on this host they accrued roughly 9 GB/day, so deferring the reap to a breach would absorb that growth between breaches instead of bounding it.

**DW-33** When identifying a build cache, the system shall require structural proof and shall never rely on the directory's name: every top-level entry must be a two-hex-digit shard directory or known cache furniture, there must be at least 64 shards, and every sampled shard must contain only content-addressed cache files or nothing at all. A directory holding any foreign entry, at either level, shall be kept.

**DW-34** When a build cache's modification time is newer than the configured age gate, when a process holds a file open inside it, or when that liveness probe cannot be evaluated, the system shall keep the cache and shall record the reason it was kept.

**DW-35** While dry-run mode is set, the system shall scan for build caches and report the reclaimable bytes but shall delete nothing.

**DW-36** The system shall not follow symbolic links when scanning for build caches, so a link planted inside a scanned directory cannot direct a deletion outside the configured scan roots.

**DW-37** When the configured build-cache age gate is not positive while the reaper is enabled, the system shall reject it as a usage error and exit 2, because a zero or negative window would make a cache an in-flight build is writing immediately eligible. Passing empty scan roots is the supported way to disable the reaper.

**DW-38** When a configured build-cache scan root does not exist, the system shall treat it as an empty result rather than a failure, so a host without that directory still completes a healthy tick.

**DW-39** The system shall reap abandoned E2E test fixture directories under the configured E2E cache directory on every tick, regardless of whether any disk threshold is breached.

**DW-40** When identifying an E2E test fixture directory, the system shall verify that its name matches the prefix "agm-", that it is owned by the current user, that it is not a symlink, and that its contents contain only expected fixture files ("agm", "agm.lock", or "agm-build-*"). A directory containing any foreign entry shall be kept.

**DW-41** When an E2E test fixture directory is within the configured max-entries bound and newer than the configured age gate, when a process holds a file open inside it, or when its liveness probe cannot be evaluated, the system shall keep the fixture directory and shall record the reason it was kept.

**DW-42** When the configured E2E cache age gate is not positive while the E2E reaper is enabled, the system shall reject it as a usage error and exit 2. Passing an empty E2E cache directory is the supported way to disable the E2E reaper.

**DW-43** The system shall reap abandoned preflight scratch directories across configured preflight scratch roots on every tick, regardless of whether any disk threshold is breached.

**DW-44** When identifying candidate preflight scratch directories, the system shall discover directories under `${XDG_CACHE_HOME:-$HOME/.cache}/dear-agent/preflight-tmp` and `${XDG_CACHE_HOME:-$HOME/.cache}/dear-agent/preflight-runs`, as well as legacy preflight directories matching `.preflight-home-*`, `.preflight-*`, `ce*-preflight*`, `.ce[0-9]*`, and `.tmp` under `$HOME`, and `ce*-preflight*`, `dear-agent-preflight-*`, and `ce-*-host-tmp*` under `~/.cache`.

**DW-45** When identifying a candidate preflight scratch directory, the system shall verify that the candidate is owned by the current user EUID, is a regular directory and not a symbolic link, is older than the configured preflight scratch age gate (default 24h), and has no active processes holding files open inside it. A directory failing any safety check or whose liveness probe cannot be evaluated shall be kept.

**DW-46** While dry-run mode is set, the system shall scan for abandoned preflight scratch directories and report reclaimable bytes but shall delete nothing.

**DW-47** When the configured preflight scratch age gate is not positive while the preflight reaper is enabled, the system shall reject it as a usage error and exit 2. Passing empty preflight scratch roots is the supported way to disable the preflight scratch reaper.
