# vroom-governor — Specification

## Executable EARS Requirements

**VGOVR-01** When VROOM evaluates worker capacity, the governor shall apply the configured limits before authorizing additional work.

**VGOVR-02** If a governed signal is missing or stale, then the governor shall fail conservatively and expose the reason.

**VGOVR-03** If the load probe or the memory probe returns an error, then the governor shall engage the admission brake with a reason naming the failing probe.

**VGOVR-04** When both probes read cleanly and both readings are within thresholds, the governor shall release the admission brake only if it engaged that brake itself.

**VGOVR-05** When a probe reads cleanly and breaches a threshold, the governor shall extend the last-spawn timestamp and the governor shall not modify the admission brake.

**VGOVR-06** When a resource limit is exceeded, the governor shall write a future RFC3339 timestamp to AGM's shared spawn timer so admission remains paused for the configured interval.

**VGOVR-07** When resource limits clear, the governor shall stop extending the shared spawn timer; admission shall remain blocked until the existing hold and the circuit breaker's spawn safety interval have elapsed, and shall still require every other admission gate to pass.

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`

<!-- Last audited at: NEEDS-AUDIT -->

## Overview

vroom-governor is a standalone CLI that polls system load and free memory on
a fixed interval and pauses or resumes AGM worker spawns accordingly. On
critical memory pressure it archives the newest active worker session to
reclaim resources.

## Functional Requirements

- Poll 5-minute load average and free memory percentage on a configurable
  interval
- Pause new AGM worker spawns when load exceeds `max-load-ratio × NumCPU` or
  free memory drops below `min-free-mem-pct`, by extending
  `~/.agm/last-spawn.txt`
- Archive the newest active worker session when free memory drops below
  `min-free-mem-pct-critical`
- Engage the cross-process admission brake (`pkg/vroom/admission`) when either
  probe cannot be read, so every spawn path refuses new work; release it on a
  clean tick that is also within thresholds. Until ce-93lw.18 these errors were
  discarded by `err == nil &&` guards, which made a governor that had gone blind
  indistinguishable from one reporting a healthy host. Threshold breaches keep
  using the `last-spawn.txt` pause: that path works and vroom-dispatch's stagger
  retry is the right response to it, whereas an unreadable probe is the case
  where we do not know what we are waiting for. Releases are scoped to this
  source: this governor ticks every 30 seconds against disk-watchdog's 5
  minutes, so an unconditional release would clear a disk brake almost as fast
  as the watchdog could set one
- Read free memory on macOS via `memory_pressure -Q` (accounts for the
  reclaimable inactive queue, matching Activity Monitor) rather than raw
  `vm_stat` free-page counts, which macOS keeps near zero by design
- Read the 5-minute load average via `sysctl -n vm.loadavg` on macOS and
  `/proc/loadavg` on Linux
- Exit cleanly on SIGTERM/SIGINT

## Non-Functional Requirements

- Memory/load probes are bounded by a timeout so a hung subprocess fails the
  read instead of hanging the governor loop
- No external dependencies beyond the `agm` CLI and platform tools
  (`memory_pressure`, `sysctl`, `/proc`)
