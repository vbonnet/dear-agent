# Stop Session Guard Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`stop-session-guard` validates repository cleanliness and worktree hygiene
before allowing a Claude Code session to exit.

## EARS Requirements

**SSG-01** When stop hook input cannot be parsed, the system shall fail open.

**SSG-02** When the hook cwd is empty or not a git repository, the system shall skip repository checks.

**SSG-03** When the working directory has uncommitted changes, the system shall block session exit and report remediation.

**SSG-04** When commits are unpushed, the system shall warn and report push remediation.

**SSG-05** When any non-bare worktree has uncommitted changes, the system shall block session exit and identify affected worktrees.

**SSG-06** When extra branches exist, the system shall warn and report branch cleanup remediation.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/hooks/cmd/stop-session-guard/main_test.go`

