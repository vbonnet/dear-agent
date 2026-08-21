# Language Policy Store Specification

<!-- Last audited at: 2026-08-19 -->

## Overview

This directory holds the data contract for the repository's shell-script
language policy: the waiver store (`exceptions.jsonl`) and the waiver ratchet
(`baseline.json`). The enforcing implementation is
[`tools/language-policy`](../../tools/language-policy/SPEC.md); this
specification governs the files it reads.

Both files are text on purpose. Granting a waiver is a policy decision, so it
must be attributable with `git blame` and reviewable in a diff. The store was a
committed SQLite binary until 2026-08-19; see
[`README.md`](README.md) for the format and the rationale.

## EARS Requirements

**LANGPOLICY-STORE-01** The waiver store shall be line-oriented JSON with one waiver object per line, so each waiver is attributable to the commit that granted it.

**LANGPOLICY-STORE-02** The waiver store shall be sorted by rule then path, so adding a waiver produces a single-line diff.

**LANGPOLICY-STORE-03** The waiver store shall record for each waiver a rule, a repository-relative path with no leading `./`, and a status of `active`, `grandfathered`, `revoked`, or `expired`.

**LANGPOLICY-STORE-04** The waiver store shall record for each waiver a reason describing why the script cannot meet the rule.

**LANGPOLICY-STORE-05** Where a waiver is time-bounded, the waiver store shall record a sunset date in `YYYY-MM-DD` form.

**LANGPOLICY-STORE-06** Where a waiver is open-ended, the waiver store shall record a null sunset date.

**LANGPOLICY-STORE-07** The waiver store shall not contain duplicate rule and path pairs.

**LANGPOLICY-STORE-08** The waiver store directory shall not contain a waiver store in a binary database format.

**LANGPOLICY-STORE-09** The baseline file shall declare for each enforced rule a ceiling on the number of waivers that rule may carry.

**LANGPOLICY-STORE-10** Where waivers are removed, the baseline file shall declare a lowered ceiling matching the remaining count, so the waiver backlog cannot silently refill.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
- Contract tests: `tools/language-policy/store_test.go`, `tools/language-policy/verify_test.go`
