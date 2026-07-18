# Repository Inventory Specification

<!-- Last audited at: 2026-07-17 -->

## Overview

`internal/repoinventory` is the single filesystem discovery seam for repository
governance tools. It returns stable repository-relative files while respecting
Git ignore rules and excluding dependency, generated-output, test-fixture,
nested-worktree, and VCS directories.

## EARS Requirements

**REPO-INV-01** When a scan root belongs to a Git worktree, the system shall include tracked and untracked non-ignored files beneath that root.

**REPO-INV-02** When Git marks a repository path ignored, the system shall exclude that path from inventory results.

**REPO-INV-03** When a scan root is not a Git worktree, the system shall fall back to a filesystem walk using the same excluded-directory policy.

**REPO-INV-04** When repository files are returned, the system shall sort their slash-normalized paths relative to the requested scan root.

**REPO-INV-05** When repository files are returned, the system shall retain file-mode metadata needed to distinguish executable sources.

**REPO-INV-06** When inventory traverses repository content, the system shall exclude VCS, nested-worktree, dependency, generated-output, and test-fixture directories.

**REPO-INV-07** When Git inventory fails for a Git worktree, the system shall return the failure instead of silently falling back to a filesystem walk that could include ignored files.

**REPO-INV-08** When the non-Git fallback encounters an unreadable entry, the system shall skip that entry and continue inventorying other reachable files.

**REPO-INV-09** When Git inventory fails with diagnostic output, the system shall include that output in the returned error without permitting an interactive Git prompt.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_coverage.feature`
