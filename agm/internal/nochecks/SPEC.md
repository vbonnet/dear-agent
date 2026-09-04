# Missing PR Checks Detection Specification

<!-- Last audited at: 2026-09-03 -->

## Overview

`agm/internal/nochecks` detects open pull requests whose head commit has no
required check runs and can retrigger CI through a safe empty commit. It uses
SafeGit's complete layered branch-policy owner and consumes every check-run
page before classification. Required-check policy is resolved from each
non-draft pull request's actual base branch; an optional branch filter only
limits which pull requests enter that scan. A trigger decision is revalidated
against a fresh provider snapshot immediately before any write. That snapshot
guard narrows but cannot atomically eliminate state changes after the read.

## Requirements

**NCK-01** When a pull request has one or more required check runs, the system shall not classify it as missing checks.

**NCK-02** When a pull request head has zero check runs for all configured required checks, the system shall classify it as needing a retrigger.

**NCK-03** When check-run state cannot be read, the system shall report structured indeterminate evidence and a non-successful command outcome instead of classifying the pull request or claiming healthy CI from incomplete evidence.

**NCK-04** When retriggering CI, the system shall target the pull request branch and surface command failures.

**NCK-05** When displaying a commit identifier, the system shall return a bounded short SHA without failing on short input.

**NCK-06** When effective required-check policy cannot be completely discovered or represented by check-run name, the system shall return a scan error before classifying or retriggering any pull request.

**NCK-07** When effective required-check policy is authoritatively empty, the system shall preserve the any-run fallback without conflating that state with a provider error.

**NCK-08** When reading check runs for a pull request head, the system shall treat the result as complete only after every provider page succeeds.

**NCK-09** When a scan includes non-draft pull requests, the system shall resolve required-check policy from each pull request's actual base branch and fetch each distinct base policy exactly once per scan.

**NCK-10** When the operator supplies `--branch`, the system shall treat it as an optional pull request base filter and reject provider rows outside that explicit filter; when it is omitted, the system shall scan eligible non-draft pull requests across every returned base.

**NCK-11** When a scan includes any non-draft pull request, the system shall validate every non-draft pull request base and completely resolve policy for every distinct base before reading any check runs or retriggering any pull request; any validation or policy failure shall abort the whole scan without partial classification or mutation.

**NCK-12** When a pull request is a draft, the system shall exclude it before required-policy base validation, policy discovery, check-run reads, and retriggering.

**NCK-13** When a scan reports its scope, the system shall include the explicit base filter and represent all-base mode with an empty filter value in structured output.

**NCK-14** When a scan reports a stuck pull request, the system shall include that pull request's actual base identity in text and structured evidence.

**NCK-15** When a scan resolves required-check policy for multiple bases, the system shall bound all policy reads by one shared total deadline.

**NCK-16** When a pull-request listing contains drafts, the system shall distinguish listed pull requests from eligible non-draft pull requests without claiming unread drafts have healthy CI.

**NCK-17** When a stuck pull request is considered for a write-capable retrigger, the system shall resolve non-mutating tree evidence and completely re-read check-runs, then use a fresh read of that numbered pull request as the final provider observation before writes and require complete evidence that it remains open and non-draft on the scanned base ref, head ref, and head SHA, with matching nonzero base/head repository identities belonging to the target repository.

**NCK-18** When retriggering is a dry run, the system shall completely re-read check-runs and current pull-request identity and return before tree, commit, or ref operations; when `--dry-run` is supplied without `--trigger`, the system shall reject the flags before provider access.

**NCK-19** When a pull-request listing omits or nulls Draft state, the system shall reject the complete listing without returning partial pull-request evidence.

**NCK-20** When trigger-time check revalidation shows that CI has appeared under the captured complete base policy, the system shall report the candidate as no longer stuck and perform no commit creation or ref update.

**NCK-21** When the caller context ends during a trigger sequence, every trigger provider call shall stop through that context and the command shall return before attempting later candidate mutations.

**NCK-22** When trigger-time safety is described, the system shall identify current check and pull-request reads as non-atomic snapshots and shall not claim exclusion of CI arrival or pull-request drift after those observations.

**NCK-23** When a scan is requested, the system shall reject a non-positive `--limit` before provider access and shall pass a positive operator limit to the provider listing.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_supervision_recovery_guardrails.feature`
- Package tests: `agm/internal/nochecks/*_test.go`
