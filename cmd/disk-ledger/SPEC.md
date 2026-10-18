# disk-ledger Command Specification

<!-- Last audited at: 2026-09-08 -->

Reports which agent runs leaked disk and whether the reclaim path is
reclaiming anything, on the repository health-binary exit contract.

It exists because disk-watchdog reported "build caches: 14 found, 0 reaped,
0.0 GiB" and "Status: OK (all metrics within limits)" at the same moment the
shared Go build cache held 44 GiB: its reaper only walks /tmp and TMPDIR, so
the shared roots were never scanned and reclaiming nothing read as healthy.

## EARS Requirements

**CMD-DISK-LEDGER-01** When invoked, the system shall report each tracked root with its allocated bytes, its budget, and whether it is over budget.

**CMD-DISK-LEDGER-02** When invoked, the system shall reconcile open ledger entries and report leaks attributable to owners that are no longer live.

**CMD-DISK-LEDGER-03** While an open ledger entry is younger than the grace period, the system shall not report that entry.

**CMD-DISK-LEDGER-04** When at least one tracked root is over budget and nothing was reclaimed in the window, the system shall exit 2.

**CMD-DISK-LEDGER-05** When a leak is attributable or a root is over budget while reclaim is working, the system shall exit 1.

**CMD-DISK-LEDGER-06** While nothing is over budget and no leak is attributable, the system shall exit 0.

**CMD-DISK-LEDGER-07** If arguments are invalid, then the system shall exit 3 and shall not print a report.

**CMD-DISK-LEDGER-08** If the reclaim log is absent or unreadable, then the system shall treat bytes reclaimed as zero rather than assuming reclaim succeeded.

**CMD-DISK-LEDGER-09** When the json flag is given, the system shall emit the roots, the health verdict, the leaks, and the exit code as JSON.

**CMD-DISK-LEDGER-10** When the heartbeat flag names a path, the system shall write a heartbeat there so its own absence is detectable.

**CMD-DISK-LEDGER-11** If the heartbeat cannot be written, then the system shall report that to stderr and shall not change the health verdict.

**CMD-DISK-LEDGER-12** The system shall not delete anything, because remediation belongs to the existing reapers and a defect in a detector must not destroy data.

## BDD Traceability

- `agm/test/bdd/features/disk_ledger_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
- Command tests: `cmd/disk-ledger/main_test.go`
- Library spec: `pkg/diskledger/SPEC.md`
- Schedule: `deploy/launchd/com.dear-agent.disk-ledger.plist`
