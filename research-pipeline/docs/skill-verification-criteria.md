# Verification Criteria Contract

This file is packaged with the standalone research-pipeline plugin. Stage 4
uses it when turning a reviewed plan into independently executable beads.

Every bead must declare at least one concrete, falsifiable acceptance
criterion. Use one or more of these classes:

| Class | Required shape | Example |
|---|---|---|
| Artifact | Names a path and the state it must have | `docs/retro.md exists and is non-empty` |
| Exit code | Names an exact command and result | `` `make preflight` exits 0 `` |
| Observable | Names a measurable property and threshold | `grep finds no TODO markers` |

Reject criteria that only say the work “looks correct,” “is complete,” or
“works well.” Also reject implementation-coupled criteria that merely require
a particular private function call instead of externally visible behavior.

For each bead, record:

1. the artifact, command, or observable being checked;
2. the exact passing condition;
3. any prerequisite needed to run the check; and
4. the evidence location the auditor will inspect.

The execution agent must satisfy these bead-specific criteria in addition to
the target repository’s own definition of done. A criterion that cannot be
checked is a blocking or explicitly deferred finding, never an implicit pass.
