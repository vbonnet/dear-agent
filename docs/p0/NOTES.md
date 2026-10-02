# P0: why AGM harness sessions do not spawn

Investigated 2026-10-02 on Valentin's Mac. Every claim below is backed by a
command run on the host; the receipts are inline.

## Summary

The claude-code failure is not a credential problem. AGM's sandbox reads the
working file-based token store correctly on both Mac and Linux, and it never
touches the macOS login keychain. The real blocker is that **Claude Code keys
workspace trust by absolute path, and every AGM sandbox is a brand-new path**,
so each session opens a blocking trust dialog that a detached session has
nobody to answer. The readiness wait then expires and AGM reports a generic
timeout.

## Receipts: credentials are fine

`~/.claude/.credentials.json` exists, is mode 0600, and was fresh at test time
(expiry 2026-10-02 06:13 PDT, 1.29 h remaining). A direct probe with the token
read out of that file returned **HTTP 200** from `api.anthropic.com`.

`pkg/llm/auth/oauth.go` already does the right thing. `OAuthResolver.Resolve`
prefers the auto-refreshed file token over `CLAUDE_CODE_OAUTH_TOKEN`, falls back
to the env var only when the file token is stale, and reads
`$HOME/.claude/.credentials.json` through `os.UserHomeDir()`. There is no
keychain call anywhere in the resolver, and no `apiKeyHelper` is involved.

`harnessexec.runClaude` resolves the token through that resolver and injects it
via `ClaudeEnvironment`, which also strips `ANTHROPIC_API_KEY` whenever an OAuth
token is present, so a stale API key cannot shadow it.

A fresh AGM session, once past the dialogs, reports `Sonnet 4.6 · Claude API`
and executes shell commands. No 401, no `/login`.

## Receipts: the trust dialog is the blocker

`~/.claude.json` holds **544** project entries under `/.agm/sandboxes/`.
**529 of them have `hasTrustDialogAccepted: false`.** Each is a session that
died at the dialog. The 15 that are true were answered by hand. For contrast,
47 of the 57 non-sandbox entries are trusted, which is why the host works.

## The four stacked faults

1. **`trustPreConfigured` is a hardcoded lie.** `collectExtraAddDirs`
   (`agm/cmd/agm/new_session.go`) ends with `return collectExtraAddDirsForHarness(...), true`.
   It only computes `--add-dir` entries. It never configures trust, yet it
   always claims it did.

2. **So the trust monitor never runs.** `startClaudeHarness`
   (`agm/cmd/agm/new_harness.go:195`) always takes the
   "Skipping trust prompt monitoring since directory was pre-configured" branch.

3. **The ordering makes the handler unreachable anyway.** `waitForClaudeReady`
   runs at line 191, before the trust branch at line 195. The 90 s wait expires
   while the dialog is still up, so even with faults 1 and 2 fixed the handler
   is never reached.

4. **The detector no longer matches the dialog.**
   `agm/internal/tmux/prompt_detector.go:391-401` requires numbered options
   (`^❯ 1. Yes, I trust this folder`). Claude Code v2.1.273 renders the dialog
   **unnumbered**, and puts **`No, exit` first as the default selection**:

   ```
    ❯ No, exit
      Yes, I trust this folder
    Enter to confirm · Esc to cancel
   ```

   Every regex misses. And the auto-answer path deliberately presses Enter only
   when the affirmative option is already selected (ce-wn4qe), so it would still
   decline to answer a dialog that now defaults to "No".

## Second gate

After trust, Claude Code opens `N new MCP servers found in this project`,
driven by the workspace's own `.mcp.json`. AGM has no handling for this at all.
Clearing trust alone is not enough.

## End-to-end verification

Session `p0verify`, sandbox `5af9d651`, both dialogs cleared inside the 90 s
window:

```
[t=12s] answered TRUST
[t=15s] dismissed MCP
[t=18s] PROMPT REACHED
✓ Claude is ready!
✓ Session 'p0verify' created (detached)
```

Then, non-interactively in that session:

```
❯ Run the bash command: git rev-parse --short HEAD && echo AGM_AUTH_OK
⏺ 1e2110c13
  AGM_AUTH_OK
```

`1e2110c13` is this branch's own commit, confirming the sandbox is a real clone
of the worktree and that the session authenticated and executed.

## What this change ships

Only the part that needs no security decision: the readiness timeout now names
the gate that is actually holding the session, instead of sending the operator
to a generic "is claude installed?" checklist for what is really an unanswered
dialog. `tmux.DiagnoseStartupBlock` is read-only. It captures the pane,
classifies it, and never sends a key.

## What still needs Valentin's sign-off

The real fix is to pre-register the sandbox workspace in `~/.claude.json`
(`hasTrustDialogAccepted`, plus an explicit `enabledMcpjsonServers` decision)
before launch, under an advisory lock with an atomic rename, so the dialogs
never appear and `trustPreConfigured` becomes honest. That is strictly narrower
than `--dangerously-skip-permissions`: permission rules still evaluate, the
session just stops re-asking whether AGM's own scratch directory is trustworthy.

An implementation attempt was **blocked by the Claude Code auto-mode security
classifier as "Security Weaken" and was not routed around**. Granting folder
trust and auto-enabling MCP servers in the operator's global config is a real
security boundary, so it is Valentin's call. See bead ce-324.

## Other harnesses

- **agy (ce-d1g)**: `agy --help` exposes exactly one approval control,
  `--dangerously-skip-permissions`. There is no scoped allowlist flag upstream,
  so "autonomous without blanket-disabling safety" is not reachable by flag
  today. The honest path is to mediate agy tool calls through AGM's existing
  `internal/permissionparity` policy engine.
- **codex-cli (ce-vpm)**: the audited hook-trust path already exists and works.
  Documentation gap only; the spawn error should name the override command.
- **AGY log budget (ce-pwt)**: `maxAgyLogFiles = 64` is a compile-time constant
  that does not scale with the number of sibling workspaces. `~/worktrees` holds
  137, so discovery blows the cap before finding the metadata.

## Operational hazard found in passing

A worktree created at 04:57 with no commits on its branch was deleted, directory
and branch both, by about 05:00. A second worktree carrying one commit survived
the same window. Session GC appears to reap commit-less worktrees. Worth a guard
before it eats something a human was mid-way through.
