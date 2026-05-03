# DEAR Retro: Resource Leaks from Orphaned Git Worktrees

**Date:** 2026-05-01  
**Severity:** Medium (disk space, git repo pollution, confused state)  
**Status:** Resolved — tooling added, worktrees cleaned up

---

## Define

**The invariant that was violated:**

Every agent session that creates a git worktree or branch MUST clean it up when the session ends — regardless of success, failure, or merge status. This is a lifecycle invariant, not a best practice.

**What "orphaned" means:**

A worktree is orphaned when:
1. The session that created it has ended (tmux gone, AGM archived/stopped), AND
2. The branch it tracks is no longer being actively worked on, AND
3. No other session claims ownership

**Scope of the problem discovered:**

```
$ git -C ~/src/ai-tools worktree list | grep prunable | wc -l
123

$ ls ~/worktrees/ai-tools/ | wc -l
13   (session-named worktrees, most orphaned)

$ ls ~/worktrees/ai-conversation-logs/ | wc -l
30+  (branch-named worktrees, many orphaned)

$ ls ~/worktrees/dear-agent/ | wc -l
3    (prunable)
```

Total: **170+ orphaned worktrees** across 4+ repositories, accumulated over ~3 months.

---

## Enforce

**Root cause — three compounding failures:**

### 1. Tracking bypass: raw `git worktree add` without AGM registration

The existing `agm admin cleanup-worktrees` command relies on a Dolt database of tracked worktrees. But agents create worktrees via raw shell commands (`git -C ~/src/ai-tools worktree add ...`) that never register in the DB. Result: the tracking layer knows about 0 of the 170+ orphans.

### 2. No Stop hook — cleanup only on archive

`cleanup.SessionResources()` runs during `agm session archive`, but most sessions end by being killed/stopped, not archived. Stopped sessions have no cleanup lifecycle.

### 3. "Agent judgment" cleanup — which means no cleanup

CLAUDE.md said to clean up worktrees "when done." But agents don't reliably self-report completion and never received a deterministic signal to clean up. Without a hook, cleanup depends on the agent remembering — and it doesn't.

**Enforcement mechanisms added (this retro):**

| Mechanism | What it does |
|-----------|-------------|
| `agm session cleanup` | Per-session cleanup: removes tracked (manifest) worktrees + local branch |
| `agm audit resources` | Filesystem-based scan; cross-references against active sessions; flags orphans |
| `agm audit resources --fix` | Removes flagged orphans (with confirmation prompt) |
| Manifest `resources:` field | Agents declare worktrees/branches they create; cleanup uses this |
| Stop lifecycle hook | Triggers `agm session cleanup` when a session ends (not just on archive) |

---

## Audit

**Evidence gathered:**

```bash
# ai-tools: 123 prunable worktrees in git's own accounting
git -C ~/src/ai-tools worktree list --porcelain | grep -c "prunable"

# dear-agent: 3 prunable
git -C ~/src/dear-agent worktree list | grep prunable

# ~/worktrees/: mixture of session-named and branch-named dirs
# session-named pattern: <adjective>-<noun>-<hexid> (Docker-style AGM names)
# branch-named pattern:  <repo>-<branch>
```

**Session-named orphans in ~/worktrees/ai-tools/** (all stale — no active tmux):
- amazing-easley-b5d9f9
- awesome-sutherland-c3db09
- crazy-goldstine-aa3827
- exciting-booth-b51bf9
- fervent-cori-34653d
- great-carson-028a48
- great-jackson-294857
- musing-villani-2105dc
- pensive-mcclintock-14a83a
- recursing-pascal-486cde
- serene-wiles-cd7b65
- trusting-mayer-5404ea

**Branch-named orphans in ~/worktrees/ai-tools/**:
- ai-tools-agm-bus-launchd, ai-tools-agm-fix, ai-tools-batch-api, ai-tools-daemon-cron,
  ai-tools-doctor, ai-tools-doctor-merge, ai-tools-gate-routing, ai-tools-gemini,
  ai-tools-layer1-clean, ai-tools-layer1-opus-trap, ai-tools-layer2-research-go,
  ai-tools-layer3-agm-bus, ... (many more prunable via `git worktree prune`)

**Trigger:** brain-v2 work created 5+ new worktrees; human had to remind agent to clean up. Agent did not.

---

## Resolve & Refine

**Immediate remediation (this session):**

1. `git worktree prune` run for all repos to remove stale refs
2. Orphaned worktree directories removed from ~/worktrees/
3. Associated branches deleted where safely merged

**Tooling changes (this PR):**

### Manifest: `resources` field
```yaml
resources:
  worktrees:
    - path: ~/worktrees/ai-tools/my-feature
      branch: my-feature
      repo: ~/src/ai-tools
      created_at: 2026-05-01T10:00:00Z
  branches:
    - name: my-feature
      repo: ~/src/ai-tools
      created_at: 2026-05-01T10:00:00Z
```

Agents that create worktrees SHOULD update their session manifest. This makes cleanup deterministic for well-behaved sessions.

### `agm session cleanup [session-name]`
- Reads `resources` from manifest → removes worktrees → deletes branches
- Falls back to `git worktree list` cross-reference if manifest has no resources
- Idempotent: safe to run multiple times

### `agm audit resources [--worktrees-dir DIR] [--repos DIR...] [--fix]`
- Filesystem-based: scans `~/worktrees/` recursively
- Gets active sessions from Dolt (with tmux fallback)
- Flags directories with no active session owner as orphans
- `--fix` removes orphans after confirmation

### Stop lifecycle hook
- Registered as a `Stop` hook in Claude Code settings
- Calls `agm session cleanup` when session stops
- Best-effort: hook failure doesn't block session exit

**Process change:**

> Agents creating worktrees MUST add them to their session manifest.
> The manifest is the source of truth for cleanup.
> `agm audit resources` is the safety net for anything that slips through.

**Monitoring:**

Run `agm audit resources` weekly or add it to the `agm session list` output as an "orphaned resources" count.

---

## Prevention

| What | How |
|------|-----|
| Agent forgets to track | `agm audit resources` catches it on next run |
| Agent forgets to clean up | Stop hook runs `agm session cleanup` automatically |
| DB not available | `agm audit resources` is filesystem-based (no DB needed) |
| New repos not scanned | `--repos` flag; default scans `~/src/` and `~/worktrees/` |
| This retro itself orphans a worktree | ecstatic-sinoussi-9a19da cleaned up by stop hook after merge |
