# docref-lint Command Specification

<!-- Last audited at: 2026-08-18 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `tools/docref-lint`.

## Overview

`docref-lint` fails when a living document names a repository file that does
not exist. It converts an existing written rule — AGENTS.md, "a wrong
living-document claim is a defect" — into a deterministic check that runs
before an agent finishes, rather than one the pull-request reviewer has to
enforce by hand afterwards.

The check earns its place from evidence. Mining every automated-review comment
on this repository's pull requests, "the document claims an artifact that is
not in the tree" recurred 32 times across 12 pull requests, each costing a
review round to discover something a filesystem lookup answers for free.

## EARS Requirements

**DOCREF-01** When invoked without `-all`, the system shall inspect only tracked Markdown changed against the merge base with the configured base ref.

**DOCREF-02** When the merge base or diff cannot be determined, the system shall audit every tracked Markdown file and report that it did so, rather than inspecting an empty set.

**DOCREF-03** When `-all` is requested, the system shall inspect every tracked Markdown file outside `testdata` directories.

**DOCREF-04** When a reference is evaluated, the system shall consider only backticked paths carrying a known repository prefix and a source-file extension.

**DOCREF-05** When a reference is resolved, the system shall accept it if it names a tracked file relative to the repository root or relative to any ancestor directory of the citing document.

**DOCREF-06** When any reference resolves nowhere, the system shall report each finding with its document and line and exit non-zero.

**DOCREF-07** When findings are reported, the system shall state both remediations: land the promised artifact, or correct the claim.

## Test Traceability

- Unit package: `tools/docref-lint`

## Integration

Runs as a `run_step` in `scripts/guardrail-bundle.sh`, which the
`.claude/hooks/stop-guardrail-feedback` Stop/SubagentStop hook executes. A
finding therefore blocks the agent's stop and returns remediation text into the
live session, instead of relying on an agent having loaded a policy document.
