# Language Policy Command Specification

<!-- Last audited at: 2026-08-19 -->

## Overview

`tools/language-policy` enforces the repository's shell-script language policy
(`bash-20-line-limit`) and validates the waiver store that records exemptions.

It replaced a shell block embedded in
[`.github/workflows/language-policy.yml`](../../.github/workflows/language-policy.yml)
and a drifted second copy in that workflow's nightly sweep. The waiver store is
line-oriented JSON at `.github/language-policy/exceptions.jsonl` rather than a
binary database, so each waiver is attributable with `git blame` and reviewable
in a diff. See
[`.github/language-policy/README.md`](../../.github/language-policy/README.md).

## EARS Requirements

**LANGPOLICY-CMD-01** When no subcommand is provided, the system shall print usage and exit with code 2.

**LANGPOLICY-CMD-02** When an unrecognised subcommand is provided, the system shall print usage and exit with code 2.

**LANGPOLICY-CMD-03** When `check` is invoked without `--files-from` or `--all`, the system shall report an error and exit with code 1.

**LANGPOLICY-CMD-04** When `check --files-from <file>` is provided, the system shall read NUL-delimited pathnames from that file so paths containing spaces or newlines are preserved.

**LANGPOLICY-CMD-05** When a candidate path lies under an excluded directory segment (`.archived`, `node_modules`, `vendor`, `.worktrees`) at any depth, the system shall exclude it from evaluation.

**LANGPOLICY-CMD-06** When a candidate path is not a regular file, the system shall skip it rather than fail the scan.

**LANGPOLICY-CMD-07** The system shall count a shell script's countable lines as those that are neither blank nor comment-only, treating a shebang as a comment.

**LANGPOLICY-CMD-08** When a script's countable line count is at or below the limit, the system shall record it as compliant.

**LANGPOLICY-CMD-09** When a script exceeds the limit and a shell test under `tests/bats/` references it by basename, the system shall record it as tested and shall not require a waiver for it.

**LANGPOLICY-CMD-10** When no shell test suite directory is present, the system shall credit no test coverage and shall not report an error.

**LANGPOLICY-CMD-11** When a script exceeds the limit, is untested, and an unexpired waiver covers it for the rule, the system shall record it as waived and not as a violation.

**LANGPOLICY-CMD-12** When a waiver carries a sunset date that has passed, or a sunset date that cannot be parsed, the system shall treat that waiver as inactive.

**LANGPOLICY-CMD-13** When a script exceeds the limit, is untested, and has no active waiver, the system shall record a violation and exit with code 1.

**LANGPOLICY-CMD-14** When `--github` is provided, the system shall emit violations as GitHub Actions error annotations.

**LANGPOLICY-CMD-15** When `sweep` is invoked, the system shall report waivers whose sunset date has passed and waivers whose target file no longer exists.

**LANGPOLICY-CMD-16** When `sweep` is invoked, the system shall report the counts of compliant, tested, and waived scripts.

**LANGPOLICY-CMD-17** When `sweep` is invoked and the waiver count exceeds the number of passing scripts, the system shall emit a warning and shall not fail the run.

**LANGPOLICY-CMD-18** When `verify-store` is invoked and a file with a binary database extension (`.db`, `.sqlite`, `.sqlite3`, `.db3`) is present in the waiver store directory, the system shall report an error and exit with code 1.

**LANGPOLICY-CMD-19** When `verify-store` is invoked and the waiver store contains NUL bytes, the system shall report an error and exit with code 1.

**LANGPOLICY-CMD-20** When the waiver store cannot be parsed, contains a duplicate rule and path pair, or declares an unrecognised status, the system shall report an error naming the line number and exit with code 1, and shall not treat the store as empty.

**LANGPOLICY-CMD-21** When `verify-store` is invoked and the waiver store is not sorted by rule then path, the system shall report an error and exit with code 1.

**LANGPOLICY-CMD-22** When `verify-store` is invoked and a rule's waiver count exceeds the ceiling declared for it in the baseline file, the system shall report an error and exit with code 1.

**LANGPOLICY-CMD-23** When `verify-store` is invoked and the baseline file declares no ceiling for the rule being enforced, the system shall report an error and exit with code 1.

**LANGPOLICY-CMD-24** When `format` is invoked, the system shall rewrite the waiver store sorted by rule then path, one compact JSON object per line, preserving the file's existing permissions.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
- Command tests: `tools/language-policy/exceptions_test.go`, `tools/language-policy/lines_test.go`, `tools/language-policy/store_test.go`, `tools/language-policy/verify_test.go`
