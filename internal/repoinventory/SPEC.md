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

**REPO-INV-03** When Git inventory is unavailable, the system shall fall back to a filesystem walk using the same excluded-directory policy.

**REPO-INV-04** When repository files are returned, the system shall sort their slash-normalized paths relative to the requested scan root.

**REPO-INV-05** When repository files are returned, the system shall retain file-mode metadata needed to distinguish executable sources.

**REPO-INV-06** When inventory traverses repository content, the system shall exclude VCS, nested-worktree, dependency, generated-output, and test-fixture directories.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_coverage.feature`
