# PR Size Comment Command Specification

<!-- Last audited at: 2026-08-21 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `tools/pr-size-comment`.

## Overview

`pr-size-comment` composes and upserts the sticky comment
`.github/workflows/pr-size-scope.yml` posts for its deterministic size, scope,
and code-health signals. It replaces bash that had grown past what a workflow
step should carry: reading existing comments, recovering a code-health
finding across a run that could not re-measure it, rendering the body, and
choosing between updating, deleting, or leaving the comment alone.

The command is advisory by construction, matching every signal it reports: a
comment-update failure is never the reason a pull request cannot merge. It is
not, however, silent about that failure to a direct caller: `upsertComment`
and `deleteStaleComments` return a distinct non-zero status when a required
`gh` operation fails, so the advisory posture toward the merge decision is a
property of the calling workflow step (`continue-on-error: true`), not of
this command swallowing the error itself.

## EARS Requirements

**PRSIZECOMMENT-01** When none of the size, mixed-concern, or code-health signals require an update and the code-health step completed successfully with nothing flagged or unknown, the command shall delete any existing marker comment.

**PRSIZECOMMENT-02** When any of the size, mixed-concern, or code-health-flagged signal is true, or the code-health step reports an unknown result, the command shall update the existing marker comment in place or post a new one.

**PRSIZECOMMENT-03** When the code-health step's own outcome was not successful, or it reports an unknown result, and this run has no fresh code-health report, the command shall recover the code-health section from the comment it is about to overwrite rather than erase it.

**PRSIZECOMMENT-04** When there is no fresh code-health report to show and nothing to recover, and the code-health step reports an unknown result with a summary, the command shall render that summary so an unknown-only run is not silently invisible.

**PRSIZECOMMENT-07** When the size detector's own outcome was not successful, the command shall recover the size section from the comment it is about to overwrite, and when the mixed-concern detector's own outcome was not successful, the command shall recover the concern section, each gated on that detector alone. A recovered section shall be labeled as recovered and not from this revision. The two detectors fail independently, so recovering them together lets a fresh result from either erase the other's last known result, or lets a run where both are blank republish a half-stale block as current.

**PRSIZECOMMENT-05** When more than one marker comment exists, the command shall update or delete only the oldest and remove the rest.

**PRSIZECOMMENT-06** When a required `gh` operation fails while upserting or deleting the marker comment, the command shall exit with a distinct non-zero status rather than reporting success.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- No BDD change, with reason: this command is a thin wrapper composing a
  comment body and calling `gh` to upsert it, mirroring the other advisory
  command wrappers this workflow already invokes. Its behavior is proven by
  `tools/pr-size-comment/main_test.go`; a scenario here would restate those
  unit tests without adding evidence, and it has no cross-harness surface of
  its own.

## Test Traceability

- Unit package: `tools/pr-size-comment`
- Consumer: `.github/workflows/pr-size-scope.yml`
