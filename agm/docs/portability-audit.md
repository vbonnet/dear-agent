# Portability Audit: Local-Only State

**Date**: 2026-04-10
**Goal**: Identify all local-only state that the dear-agent setup depends on.
If this machine is wiped, we should be able to redeploy from `ai-tools` + `engram-research` repos only.

---

## Summary

The agent ecosystem has ~6.5 GB of local state in `~/.claude/` and ~3.5 GB in `~/.agm/`.
Most is ephemeral (caches, session data, logs). However, several critical configuration
files exist only locally and would block a clean redeploy if lost.

**Critical gaps identified:**
- `~/.claude/settings.json` (1276 lines) — permission model, hooks, MCP servers
- `~/.claude/hooks/` (55+ compiled hook binaries) — workflow enforcement
- `~/.agm/config.yaml` — workspace topology
- `~/.config/agents/AGENTS.md` — base agent instructions (referenced by 6+ repos, may not exist)
- `~/.claude/.credentials.json` — OAuth/secrets

---

## Complete Inventory

### ~/.claude/ (6.5 GB total)

| Item | Path | Size | Classification | Recommended Action |
|------|------|------|----------------|-------------------|
| **Settings** | `~/.claude/settings.json` | 74 KB | **Project** | Already managed by chezmoi template. Verify template is up-to-date with runtime copy. |
| **Hook binaries** | `~/.claude/hooks/` | 276 MB | **Project** | Source code is in ai-tools. Binaries are compiled artifacts. Add `make install-hooks` target to rebuild from source. |
| **CLAUDE.md** | `~/.claude/CLAUDE.md` | ~15 KB | **Personal** | Managed by chezmoi. Currently bloated with 16 duplicate sandbox sections — needs cleanup. |
| **AGENTS.md** | `~/.claude/AGENTS.md` | 22 KB | **Personal** | Managed by chezmoi. Contains agent role reference. |
| **Config** | `~/.claude/config.json` | 333 B | **Project** | MCP engram server endpoint (GCP Vertex AI). Add to chezmoi if not already. |
| **Hooks manifest** | `~/.claude/hooks.yaml` | 1.1 KB | **Project** | Hook manifest with SHA256 validation. Should be generated from ai-tools source. |
| **Machine config** | `~/.claude/machine-config.yaml` | 5.1 KB | **Project** | Environment-specific settings. Add to chezmoi as template. |
| **Credentials** | `~/.claude/.credentials.json` | 897 B | **Personal** | OAuth/secrets. Must NOT go in any repo. Document in recovery runbook. |
| **Project memories** | `~/.claude/projects/*/memory/` | ~4.8 GB | **Ephemeral** | 802 project dirs. Per-project Claude memory. Rebuilt organically. Can be seeded from key memories if desired. |
| **Skills** | `~/.claude/skills/` | 648 MB | **Project** | Installed skills (agm, wayfinder, engram, agent-browser). Source is in repos. Need `make install-skills` target. |
| **Todos/Tasks** | `~/.claude/todos/`, `tasks/` | 13 MB | **Ephemeral** | Session-scoped task state. Rebuilt per conversation. |
| **Stats cache** | `~/.claude/stats-cache.json` | 12 MB | **Ephemeral** | Token usage telemetry. Leave as-is. |
| **History** | `~/.claude/history.jsonl` | 12 MB | **Ephemeral** | Session transcripts. Leave as-is. |
| **File history** | `~/.claude/file-history/` | 173 MB | **Ephemeral** | Edit audit trail. Leave as-is. |
| **Shell snapshots** | `~/.claude/shell-snapshots/` | 7.9 MB | **Ephemeral** | PWD/env state. Leave as-is. |
| **Session env** | `~/.claude/session-env/` | 8.1 MB | **Ephemeral** | Per-session env cache. Leave as-is. |
| **Telemetry** | `~/.claude/telemetry.jsonl` | 6.3 MB | **Ephemeral** | Usage metrics. Leave as-is. |
| **Context monitor** | `~/.claude/context-monitor.conf` | 227 B | **Personal** | Notification thresholds. Add to chezmoi. |
| **Local settings** | `~/.claude/settings.local.json` | 43 B | **Personal** | Local overrides. Add to chezmoi. |
| **Plugin config** | `~/.claude/plugin.yaml` | 64 B | **Personal** | Plugin registry. Add to chezmoi. |
| **README/docs** | `~/.claude/README.md`, `HIERARCHY.md`, `HOOKS.md` | ~30 KB | **Personal** | Documentation. Managed by chezmoi. |

### ~/.agm/ (3.5 GB total)

| Item | Path | Size | Classification | Recommended Action |
|------|------|------|----------------|-------------------|
| **Workspace config** | `~/.agm/config.yaml` | 294 B | **Project** | Managed by chezmoi template. Contains hardcoded paths (`/home/user/src/ws/`). Verify template parameterizes paths. |
| **Message queue DB** | `~/.agm/agm.db` | 98 KB | **Ephemeral** | Core daemon message queue. Rebuilt on daemon start. |
| **Sessions DB** | `~/.agm/sessions.db` | 98 KB | **Ephemeral** | Session metadata. Rebuilt from session usage. |
| **Orchestrator state** | `~/.agm/orchestrator-state.json` | 29 KB | **Ephemeral** | Tracks managed sessions, test status. Rebuilt on orchestrator start. |
| **Meta-orchestrator** | `~/.agm/meta-orchestrator-state.json` | 2.2 KB | **Ephemeral** | Boot count, lifecycle phase. Rebuilt on start. |
| **Orchestrator DB** | `~/.agm/orchestrator-state.db` | 28 KB | **Ephemeral** | Execution checkpoints. Rebuilt. |
| **Pending queue** | `~/.agm/pending/` | 11 MB | **Ephemeral** | Intake items. Purge on redeploy. |
| **Sandboxes** | `~/.agm/sandboxes/` | 3.4 GB | **Ephemeral** | Overlay mounts. Cannot transfer. Recreated by `agm new`. |
| **Logs** | `~/.agm/logs/` | 74 MB | **Ephemeral** | Daemon/session logs. Archive or discard. |
| **State** | `~/.agm/state/` | 1 MB | **Ephemeral** | Message sequence snapshots. Discard. |
| **Gemini cache** | `~/.agm/gemini/` | 9.3 MB | **Ephemeral** | Gemini interactions cache. Discard. |
| **Heartbeats** | `~/.agm/heartbeats/` | 2.2 MB | **Ephemeral** | Daemon heartbeat logs. Discard. |
| **Mode cache** | `~/.agm/mode-cache/` | 2.2 MB | **Ephemeral** | Session mode state. Rebuilt on demand. |
| **Ready markers** | `~/.agm/claude-ready-*` | ~0 B | **Ephemeral** | Feature gate toggles. Regenerated. |
| **Scripts** | `~/.agm/scripts/` | 3.9 KB | **Ephemeral** | Tmux automation helper. Rebuildable. |

### ~/.engram/ (165 MB total)

| Item | Path | Size | Classification | Recommended Action |
|------|------|------|----------------|-------------------|
| **Core symlink** | `~/.engram/core` | symlink | **Project** | Points to `~/src/ws/oss/repos/engram`. Created by `engram init`. |
| **Usage telemetry** | `~/.engram/usage.jsonl` | 158 MB | **Ephemeral** | 1.4M lines of session usage. Safe to rotate/clear. |
| **Cache** | `~/.engram/cache/` | 1.7 MB | **Ephemeral** | Compiled indexes. Rebuilt by `engram index rebuild`. |
| **Logs** | `~/.engram/logs/` | 5.6 MB | **Ephemeral** | System logs. Discard. |
| **Reflections** | `~/.engram/reflections/` | 260 KB | **Ephemeral** | AI retrospectives. Nice-to-have archive but rebuildable. |
| **Ecphory config** | `~/.engram/user/config.yaml` | ~1 KB | **Personal** | Auto-retrieval settings. Add to chezmoi. |
| **Telemetry DB** | `~/.engram/telemetry.db` | ~5 MB | **Ephemeral** | SQLite telemetry. Discard. |

### ~/.config/ (Agent-related)

| Item | Path | Size | Classification | Recommended Action |
|------|------|------|----------------|-------------------|
| **AGENTS.md** | `~/.config/agents/AGENTS.md` | ~22 KB | **Project** | Managed by chezmoi. Referenced by 6+ repo CLAUDE.md files via `@import`. Verify chezmoi apply succeeds. |
| **AGENTS.why.md** | `~/.config/agents/AGENTS.why.md` | ~5 KB | **Personal** | Design rationale. Managed by chezmoi. |
| **MCP config** | `~/.config/claude-code/mcp.json` | ~1 KB | **Project** | MCP server definitions. Managed by chezmoi. |
| **Systemd services** | `~/.config/systemd/user/agm-daemon.service` | ~1 KB | **Project** | AGM daemon service. Add to chezmoi if not already. |
| **Dolt env** | `~/.config/systemd/user/agm-daemon.service.d/dolt-env.conf` | ~200 B | **Project** | Dolt connection env vars. Add to chezmoi. |

### CLAUDE.md / AGENTS.md Files

| Item | Location | Classification | Status |
|------|----------|----------------|--------|
| `CLAUDE.md` | `~/.claude/CLAUDE.md` | **Personal** | Chezmoi-managed. Bloated (16 dup sandbox sections). |
| `CLAUDE.md` | `~/src/ws/oss/repos/ai-tools/.claude/CLAUDE.md` | **Project** | Committed. |
| `CLAUDE.md` | `~/src/ws/oss/repos/engram-research/CLAUDE.md` | **Project** | Committed. |
| `CLAUDE.md` | `~/src/ws/oss/repos/engram/CLAUDE.md` | **Project** | Committed. |
| `AGENTS.md` | `~/src/ws/oss/repos/ai-tools/AGENTS.md` | **Project** | Committed. Imports 4 sub-files. |
| `AGENTS.md` | `~/.config/agents/AGENTS.md` | **Project** | Chezmoi-managed. Critical import target. |

---

## Chezmoi Coverage

**Tracked (68 files):** `.bashrc`, `.gitconfig`, `.claude/settings.json`, `.claude/AGENTS.md`,
`.agm/config.yaml`, `.config/agents/AGENTS.md`, `.config/claude-code/mcp.json`, systemd services,
chezmoiscripts for install automation.

**Not tracked (expected):** Runtime data, caches, databases, session history, compiled binaries.

**Gaps to close:**
- `~/.engram/user/config.yaml` — ecphory settings, not in chezmoi
- `~/.claude/context-monitor.conf` — notification thresholds
- `~/.claude/machine-config.yaml` — environment-specific settings
- `~/.config/systemd/user/agm-daemon.service.d/dolt-env.conf` — if not already tracked

---

## Critical Items That Would Block Redeploy

If this machine is wiped, these items would prevent a working redeploy:

| # | Item | Risk | Mitigation |
|---|------|------|------------|
| 1 | **Hook binaries** (`~/.claude/hooks/`, 55+ scripts) | Workflow enforcement gone. No bash-blocker, cost-guard, or quality gates. | Source is in ai-tools. Need `make install-hooks` target that compiles and installs all hooks. Currently no single command to rebuild them. |
| 2 | **Skills** (`~/.claude/skills/`, 15+ installed) | AGM, wayfinder, engram skills unavailable. | Source is in repos. Need `make install-skills` target. Currently installed manually. |
| 3 | **Chezmoi template drift** | `settings.json` chezmoi template may be stale vs runtime copy (74 KB, 1276 lines, 150+ sandbox paths). | Run `chezmoi diff` regularly. Add CI check. |
| 4 | **Credentials** (`~/.claude/.credentials.json`) | OAuth tokens lost. | Document manual re-auth steps in recovery runbook. Do NOT commit. |
| 5 | **Dolt/SQLite setup** | AGM daemon expects Dolt @ 127.0.0.1:3307. | Systemd service + dolt-env.conf must be in chezmoi. Add `make setup-dolt` target. |
| 6 | **Go binary installation** | `agm`, `engram` CLI binaries in `~/go/bin/`. | `go install ./...` from ai-tools root. Document in GETTING-STARTED.md. |
| 7 | **engram init** | `~/.engram/` directory structure + core symlink. | `engram init` handles this. Document in recovery runbook. |

---

## Recommended Recovery Runbook (Order of Operations)

```bash
# 1. Clone repos
git clone git@github.com:vbonnet/ai-tools.git ~/src/ws/oss/repos/ai-tools
git clone git@github.com:YOUR_ORG/engram-research.git ~/src/ws/oss/repos/engram-research

# 2. Apply chezmoi (settings.json, AGENTS.md, config.yaml, bashrc, etc.)
chezmoi apply

# 3. Build and install Go binaries
cd ~/src/ws/oss/repos/ai-tools && GOWORK=off go install ./...

# 4. Initialize engram
engram init

# 5. Install hooks (TODO: create this target)
# make -C ~/src/ws/oss/repos/ai-tools install-hooks

# 6. Install skills (TODO: create this target)
# make -C ~/src/ws/oss/repos/ai-tools install-skills

# 7. Set up Dolt/systemd (TODO: create this target)
# make -C ~/src/ws/oss/repos/ai-tools setup-dolt

# 8. Re-authenticate
# Manual: re-create ~/.claude/.credentials.json via OAuth flow

# 9. Start AGM daemon
agm daemon start
```

---

## Action Items

1. **Create `make install-hooks`** — compile all hook source in ai-tools, install to `~/.claude/hooks/`
2. **Create `make install-skills`** — install skill definitions to `~/.claude/skills/`
3. **Create `make setup-dolt`** — initialize Dolt DB + systemd service
4. **Add to chezmoi**: `~/.engram/user/config.yaml`, `~/.claude/machine-config.yaml`
5. **Clean up `~/.claude/CLAUDE.md`** — remove 16 duplicate sandbox instruction blocks
6. **Verify chezmoi template** for `settings.json` matches runtime (likely drifted with 150+ sandbox paths)
7. **Document credential recovery** in `agm/docs/RECOVERY.md`
