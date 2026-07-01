# vroom-governor — Specification

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
