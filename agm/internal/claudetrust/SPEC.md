# AGM Claude Workspace Trust Specification

<!-- Last audited at: 2026-09-01 -->

## Overview

`agm/internal/claudetrust` pre-authorizes the working directory AGM launches
Claude Code in, so an unattended spawn never meets the interactive directory
trust dialog.

Claude Code records per-directory trust in `~/.claude.json` under
`projects.<dir>.hasTrustDialogAccepted`. The key is the working directory
Claude resolves at startup, not the path it was handed: a sandboxed session is
launched under the sandbox's `merged` symlink
(`~/.agm/sandboxes/<id>/merged/repo0`) but Claude resolves that to the
underlying `upper` directory (`~/.agm/sandboxes/<id>/upper/repo0`) before
looking trust up. Seeding the path AGM knows about therefore leaves the path
Claude reads untrusted.

That mattered because the dialog is not a benign pause. It opens with
`No, exit` selected, so a spawn that reaches it either exits or blocks until the
composer wait gives up.

## EARS Requirements

**TRUST-01** When a workspace is seeded, the system shall record trust against the fully resolved working directory, following symbolic links.

**TRUST-02** When the workspace directory cannot be resolved, the system shall return an error and shall not write the configuration file.

**TRUST-03** When the configuration file already contains entries, the system shall preserve every existing field, both at the top level and within the seeded project's own entry.

**TRUST-04** When the configuration file cannot be parsed, the system shall return an error and shall leave the file unchanged.

**TRUST-05** When a workspace is already trusted, the system shall leave the configuration unchanged and report the resolved path.

**TRUST-06** When the configuration file is written, the system shall replace it atomically and shall preserve the existing file's permissions.

**TRUST-07** When several sessions seed concurrently, the system shall serialize the read-modify-write behind an exclusive lock.

**TRUST-08** When seeding fails, the session-creation path shall report trust as not pre-configured, so the trust dialog monitor runs instead of being skipped.

## Project MCP servers

A project that ships a `.mcp.json` triggers a second prompt on first run in each
new directory, asking which of its servers to enable. That prompt owns input the
same way the trust dialog does, so seeding trust alone only moves the stall one
dialog later.

Approving it is a capability grant: MCP servers can execute code. The grant is
therefore limited to sandbox workspaces, which are throwaway clones, and covers
only the servers the cloned project declares itself — alongside the permission
allowlist AGM already pre-approves into the same file.

**MCP-01** When a sandboxed Claude session is prepared, the system shall pre-approve the project's declared MCP servers in the sandbox workspace's `.claude/settings.local.json`.

**MCP-02** When a session is not sandboxed, the system shall not pre-approve project MCP servers, so a real checkout is never widened on the user's behalf.

**MCP-03** When the workspace settings file already exists, the system shall preserve its contents, including the permission allowlist written earlier in preparation.

**MCP-04** When the workspace settings file cannot be parsed, the system shall return an error and leave it unchanged.

## BDD Traceability

- Package tests: `agm/internal/claudetrust/trust_test.go`,
  `agm/internal/claudetrust/mcp_test.go`
- Wiring tests: `agm/cmd/agm/new_session_trust_test.go`
- Dialog fallback: `agm/internal/tmux/trust_option_block_test.go`,
  `agm/internal/tmux/trust_answer_test.go`
