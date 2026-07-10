# VROOM gopls Watch Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Harness-neutral gopls process pressure sampling and alarm evaluation.

## EARS Requirements

**VROOM-GOPLS-01** When process output is sampled, the system shall recognize gopls by the basename of its full executable path.

**VROOM-GOPLS-02** When process output contains malformed rows or non-gopls commands, the system shall ignore those rows.

**VROOM-GOPLS-03** When a gopls process has parent process identifier 1, the system shall classify it as an orphan independently of the originating harness.

**VROOM-GOPLS-04** When process totals are calculated, the system shall report count and resident memory for all observed gopls processes.

**VROOM-GOPLS-05** When reclaimable totals are calculated, the system shall include only orphaned gopls processes.

**VROOM-GOPLS-06** When threshold fields are non-positive, the system shall apply the documented default thresholds.

**VROOM-GOPLS-07** When orphan count exceeds its threshold, the system shall emit a count alarm containing the orphan and total process counts.

**VROOM-GOPLS-08** When orphan resident memory exceeds its threshold, the system shall emit a resident-memory alarm containing reclaimable and total memory.

**VROOM-GOPLS-09** When system file-descriptor usage exceeds its threshold, the system shall emit a file-descriptor alarm.

**VROOM-GOPLS-10** When a live resource probe cannot report file-descriptor usage, the system shall retain process count and memory data without raising a probe error.

## Test Traceability

- Package tests: `pkg/vroom/goplswatch/goplswatch_test.go`
- BDD: `agm/test/bdd/features/vroom_runtime_guardrails.feature`
