# Skill Projection Command Specification

## Overview

`tools/skill-projections` checks or creates two fixed, generated delegates from
their canonical skill files. This contract owns only deterministic projection,
one-identity filesystem access, and bounded marker discovery. The canonical
skills own their workflow content and authoring policy; a successful projection
does not establish that any harness discovers, loads, or runs a skill.

## EARS Requirements

**SKILL-PROJECTION-01** When the command receives invalid arguments, the system shall print usage and exit with code 2.

**SKILL-PROJECTION-02** When either mode starts filesystem validation on a supported Unix platform, the system shall open the caller-requested repository root once without following a symlink, record its device and inode identity, retain it before any write-mode Git discovery, use any later pathname inspection only to bind reported identity evidence, and derive every content read, directory traversal, or creation from the original retained descriptor.

**SKILL-PROJECTION-03** When check mode runs, the system shall perform no writes and shall require every fixed delegate to be a regular file whose bytes exactly match its canonical projection.

**SKILL-PROJECTION-04** When check mode finds the generated marker outside the fixed delegate paths, the system shall report every unexpected marked path in deterministic order.

**SKILL-PROJECTION-05** When write mode runs, the system shall invoke one canonical regular system Git executable outside caller-controlled PATH with a sanitized non-interactive environment, require the Git-reported worktree path identity and reciprocal regular pointer and gitdir evidence to match the already-retained caller root, report the executable and retained identity as authentication inputs, and refuse inherited repository overrides, an identity-changing same-path replacement, a caller-selected alternate root, or a primary checkout.

**SKILL-PROJECTION-06** When a fixed write path has an existing symlink or non-directory parent, the system shall refuse to traverse that parent.

**SKILL-PROJECTION-07** When any fixed write target already exists as any entry type, the system shall preserve every target and shall refuse to overwrite or delete the existing entry.

**SKILL-PROJECTION-08** When write mode creates a fixed delegate, the system shall derive directory-relative no-symlink traversal from the same authenticated retained root used for both delegate creations, use exclusive final-file creation, write only bounded expected bytes, and set regular-file mode 0644.

**SKILL-PROJECTION-09** When write mode fails after creating a possible partial file, the system shall preserve the file, perform no rollback or deletion, and identify the paths that require maintainer inspection and manual removal before retry.

**SKILL-PROJECTION-10** When either mode succeeds, the system shall read bounded regular canonical and delegate files through retained-root traversal that follows no symlink ancestor or final symlink, require canonical name and description metadata, reject rendered bytes above the delegate limit, and check both fixed delegates against their exact expected bytes before reporting success.

**SKILL-PROJECTION-11** When the platform cannot provide root-relative no-follow file opens, the system shall keep check mode non-mutating and fail closed instead of claiming symlink-race safety, and shall refuse write mode.

**SKILL-PROJECTION-12** When the system searches for generated markers on a supported Unix platform, the system shall recurse in deterministic order through descriptor-relative no-follow directory opens beneath the retained root, scan only descriptor-confirmed regular `SKILL.md` files, skip symlinks, special files, and the root Git metadata directory, enforce explicit file, directory, depth, and byte limits, check an elapsed-time budget before and immediately after every bounded directory-read batch, after every regular marker-file read and sorting stage, and before success, and fail closed at the next cooperative checkpoint without claiming that a blocking filesystem call can be interrupted.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
