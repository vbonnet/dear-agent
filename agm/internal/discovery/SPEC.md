# AGM Discovery Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/discovery` owns session discovery, Claude-session matching, tmux
mapping, and workspace-contract integration for AGM. It tolerates legacy YAML
manifests while preferring configured workspace and Dolt-backed session data.

## Requirements

**AGM-DISCOVERY-01** When Claude sessions are matched to manifests, the system shall classify matched sessions, orphaned Claude sessions, and orphaned manifests by session UUID.

**AGM-DISCOVERY-02** When creating a manifest for an orphaned Claude session, the system shall require a Dolt adapter and persist the created session through that adapter.

**AGM-DISCOVERY-03** When tmux mapping is requested with a Dolt adapter, the system shall return SessionID-to-tmux-name mappings from Dolt sessions.

**AGM-DISCOVERY-04** When tmux mapping falls back to YAML manifests, the system shall skip unreadable or unparsable manifests.

**AGM-DISCOVERY-05** When the workspace CLI contract is unavailable, the system shall return an error that names the missing `workspace` CLI.

**AGM-DISCOVERY-06** When workspace detection uses the contract, the system shall call `workspace detect --format=json` and include `--pwd=<pwd>` when a working directory is supplied.

**AGM-DISCOVERY-07** When workspace configuration exists, the system shall scan enabled workspace output directories, `.agm/sessions`, and legacy `sessions` directories.

**AGM-DISCOVERY-08** When workspace configuration is missing, invalid, or empty, the system shall fall back to legacy `~/src/ws/*` scanning.

**AGM-DISCOVERY-09** When sessions are discovered across workspaces, the system shall also scan the default `~/.claude/sessions` directory.

## BDD Traceability

- `agm/test/bdd/features/agm_control_surface_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
