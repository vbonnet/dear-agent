# internal/earsbdd — Requirements Specification (EARS)

**Version**: 1.0
**Last Updated**: 2026-06-12
**Status**: Baseline
**Scope**: EARS requirement extractor and Gherkin BDD stub generator

---

## Overview

`earsbdd` is the second stage in the EARS → BDD → code pipeline. It reads
SPEC.md files that have been validated by `ears-lint`, extracts structured
requirement records, and emits Gherkin `.feature` file stubs with
Given/When/Then steps derived from each requirement's condition and action
clauses. Generated stubs are scaffold-only; step implementations are added
by the developer in the same change per ADR-027.

---

## EARS Requirements

### Extraction

**EBD-01** When `Extract()` is called with a reader, the system shall return one `Requirement` record per EARS-formatted line containing "shall" outside a fenced code block.

**EBD-02** When a requirement line contains a bold-markdown ID prefix (e.g. `**FSG-01**`), the system shall populate `Requirement.ID` with the extracted identifier and strip it from the condition clause.

**EBD-03** When a requirement line contains " shall " with surrounding spaces, the system shall split at the first occurrence and assign the left portion to `Condition` and the right to `Action`.

**EBD-04** When the condition clause ends with ", the <actor>" (e.g. ", the system"), the system shall strip the trailing actor phrase so the When step reads as a clean trigger.

**EBD-05** When `Extract()` encounters a fenced code block delimited by ` ``` ` or `~~~`, the system shall skip all lines within the block including any that contain "shall".

**EBD-06** When `ExtractFile()` is called with a non-existent or unreadable path, the system shall return a non-nil error wrapping the OS-level failure.

### Generation

**EBD-07** When `Generate()` is called with a non-empty requirement slice, the system shall emit a Gherkin Feature block with one Scenario per requirement tagged `@ears` and `@<ID>` where an ID is present.

**EBD-08** When generating a Scenario, the system shall emit a `Given the system is configured` step, a `When` step from `Condition`, and a `Then the system shall` step from `Action`.

**EBD-09** When the `Condition` field begins with a recognized EARS keyword (`When`, `While`, `Where`, `If`), the system shall strip the leading keyword so the Gherkin `When` step does not duplicate it.

**EBD-10** When `Generate()` is called with a nil or empty requirement slice, the system shall return a `FeatureFile` with an empty `Content` field and a `FeatureName` derived from the spec path.

### CLI (`ears-to-bdd`)

**EBD-11** When invoked with no arguments, the system shall search the current directory recursively for SPEC.md files and write Gherkin output to stdout.

**EBD-12** When invoked with the `-out <dir>` flag, the system shall write one `.feature` file per discovered SPEC.md into the directory, naming each `<spec-directory-basename>.feature`.

**EBD-13** When invoked with the `-dry-run` flag, the system shall print the output paths and requirement counts that would be written without creating or modifying any files.

**EBD-14** When a SPEC.md file contains no EARS requirements, the system shall skip it and emit a warning to stderr rather than writing an empty feature file.
