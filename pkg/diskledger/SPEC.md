# Disk Ledger Specification

<!-- Last audited at: 2026-09-08 -->

Attributes disk growth to the agent run that caused it, and decides whether the
reclaim path is actually reclaiming anything.

Free-space thresholds answer the wrong question: they cannot name WHICH run
leaked, and they only fire once the volume is nearly full, by which point the
evidence is gone. On 2026-09-08 the shared Go build cache grew from 1.4 MB to
44 GiB in four days while every disk alarm stayed green, because the host still
had roughly 70 GiB free.

## EARS Requirements

### Measurement

**DL-01** When measuring a path, the system shall report allocated bytes as the
sum of `st_blocks * 512` over distinct inodes.

**DL-02** When a tree contains several links to one inode, the system shall
count that inode exactly once, so the total reflects what deleting the tree
would return rather than the sum of its names.

**DL-03** The system shall not follow symbolic links, so bytes owned by another
tree are never charged to the tree that merely points at them.

**DL-04** When measuring a path, the system shall report apparent bytes
alongside allocated bytes, because sparse files make the two disagree by a
large factor and a caller trusting either alone will be wrong on one shape.

**DL-05** If a path does not exist, the system shall report zero usage marked
missing, and shall not report an error.

**DL-06** If part of a tree is unreadable, the system shall return the partial
total marked partial rather than aborting, and shall not report a truncated
walk as a shrinking tree.

### Ledger

**DL-07** When a run opens an entry, the system shall record the run id, the
owner, the kind, the path, and the usage measured at that moment.

**DL-08** When a run closes an entry, the system shall charge the run the
allocated bytes still present at its path.

**DL-09** When a run closed after removing everything it created, the system
shall record zero leaked bytes.

**DL-10** If a close names a run that was never opened, the system shall report
an error and shall not write a close record.

**DL-11** The system shall write leaked bytes on every close record including
zero, so a reader can distinguish "nothing leaked" from "nothing ran".

### Reconciliation

**DL-12** When an entry was opened and never closed, and its owner is no longer
live, the system shall attribute the remaining bytes to that owner.

**DL-13** While an open entry is younger than the grace period, the system shall
not report it as a leak, because a run that is merely slow must never be
mistaken for one that leaked.

**DL-14** While an open entry's owner is still live, the system shall not report
it as a leak.

**DL-15** If an open entry's path no longer exists, the system shall report no
leak for it even though it was never closed.

### Budgets and reclaim health

**DL-16** When a tracked root exceeds its byte budget, the system shall report
that root as over budget.

**DL-17** If a budgeted root does not exist, the system shall not report it as
over budget.

**DL-18** When at least one tracked root is over budget and nothing was
reclaimed in the window, the system shall report `silent_gc`, because a
collector that reclaims nothing is otherwise indistinguishable from an idle
healthy host.

**DL-19** When at least one tracked root is over budget and bytes were
reclaimed in the window, the system shall report `pressure` rather than
`silent_gc`, so a working collector losing the race is distinguishable from a
broken one.

**DL-20** While no tracked root is over budget, the system shall report `ok`
even when nothing was reclaimed.

**DL-21** The default tracked roots shall include the shared Go build caches,
the module cache, the lint cache, and the preflight scratch root, none of which
were scanned by the pre-existing build-cache reaper.

### Reporting

**DL-22** The system shall report health on the repository's health-binary exit
contract: 0 healthy, 1 degraded, 2 down, 3 usage.

**DL-23** When the reclaim path is silently doing nothing, the system shall exit
2.

**DL-24** When leaks are attributable to dead owners, the system shall exit at
least 1 even when every tracked root is within budget.

**DL-25** If the reclaim log is absent or unreadable, the system shall treat
bytes reclaimed as zero, because assuming success from a missing log is how a
dead collector stays invisible.

**DL-26** The system shall never delete. Remediation belongs to the existing
reapers, so a defect in the detector cannot destroy data.

## BDD Traceability

- Package tests: `pkg/diskledger/measure_test.go`, `ledger_test.go`, `budget_test.go`
- Command tests: `cmd/disk-ledger/main_test.go`
- Consumer: `agm/internal/gclog` (DL-01 through DL-03 back `Entry.BytesReclaimed`)
