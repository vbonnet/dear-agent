# FD Pressure Monitor Specification

<!-- Last audited at: 2026-07-03 -->

## Purpose

`cmd/fd-pressure` samples system resource pressure (FD-table usage, vnodes,
gopls process count, disk, memory, swap) through the shared
`supervisor.SysResourceProbe`, reports it for humans or machines, and signals
threshold breaches through its exit code. With `--trail` it is also the bridge
that writes the Overseer's `overseer.resource.probe` record into the VROOM
decision trail (ce-mbgq), carrying the full snapshot — including the ce-6fel
disk-free-bytes and inode-usage fields.

## EARS Requirements

**FDP-01** When invoked, the system shall sample FD, vnode, gopls, disk, memory, and swap metrics from one `SysResourceProbe` snapshot.

**FDP-02** When any sampled metric crosses its escalation threshold, the system shall exit with code 1.

**FDP-03** When every sampled metric is within its threshold, the system shall exit with code 0.

**FDP-04** When the `--trail` flag is set, the system shall append one `overseer.resource.probe` record carrying the snapshot fields (including `disk_free_bytes` and `inode_used_fraction`) and the breach count to the named decision trail.

**FDP-05** When the trail append fails, the system shall report the failure on stderr and the system shall not change the exit code.

**FDP-06** When the `--json` flag is set, the system shall emit the snapshot, breaches, and OK verdict as JSON instead of the human-readable table.

**FDP-07** When a usage or probe error occurs, the system shall exit with code 2.
