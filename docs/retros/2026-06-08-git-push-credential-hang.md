# DEAR Retro: `git push` Hangs on the macOS Keychain Credential Helper

**Date:** 2026-06-08
**Severity:** High (recurring — multiple code-task workers blocked
indefinitely on `git push`; the canonical symptom of "uncommitted work is
nonexistent work" because a hung push strands a completed commit locally).
**Status:** Fixed. `safe-push` wrapper + shared `internal/safegit` package
ship in this PR; the same hang was also latent in our own `chezmoi-deploy`
wrapper and is fixed there too. Enforcement wiring (deny raw `git push`,
allow-list `safe-push`) is specified below and handed off as a dotfiles-repo
follow-up (it lives in global chezmoi-managed config, outside this repo).

## Define

**What breaks.** A worker runs `git push` in a headless agent session and it
hangs forever. The supervisor sees no output, no error, no progress — the
session looks alive but is wedged. The completed commit never reaches the
remote, so from everyone else's point of view the work does not exist.

**When it breaks.** git resolves credential helpers as a *cumulative,
ordered* chain across system → global → URL-scoped config. On this host:

| Source                          | Entry                                          |
|---------------------------------|------------------------------------------------|
| system `/opt/homebrew/etc/gitconfig` | `credential.helper = osxkeychain`         |
| global `~/.gitconfig`           | `credential.https://github.com.helper =` (empty reset) then `… = !gh auth git-credential` |
| global `~/.gitconfig`           | `credential.helper = cache --timeout=86400`    |

For **github.com** the empty `credential.https://github.com.helper=` reset
drops osxkeychain, so the chain is gh-only and the push works. But that reset
is scoped to two exact hosts (`github.com`, `gist.github.com`). For **any
other credential context** — a mirror remote, an embedded-credential URL, a
submodule, GitHub Enterprise — git falls back to the *generic* chain, and the
generic chain is queried in read order: **`git-credential-osxkeychain` first**,
then `cache`. Proven empirically (`GIT_TRACE=1`, host `example.invalid`):

```
run_command: 'git credential-osxkeychain get'      ← first
run_command: 'git credential-cache --timeout=86400 get'
```

When the keychain item's access-control list does not pre-authorize the
invoking git binary — which happens routinely after a Homebrew `git` upgrade
relocates/re-signs the binary (this host's `git-credential-osxkeychain` is
dated Apr 21) — macOS pops a **GUI authorization dialog**. In a headless agent
session no one can click it, so osxkeychain never returns and the push blocks
indefinitely. This is not a network failure (cf.
[`memory/macos-env-gaps.md`](../../): "push hangs are keychain prompts").

**Why the known workaround is unreliable.** The widely-pasted fix

```
git -c credential.helper='!gh auth git-credential' push
```

only *appends* the gh helper; it never resets osxkeychain off the front of the
chain. So it works for github.com (already reset-protected) and hangs for
every other context — exactly the "works sometimes" report. Worse, our own
`chezmoi-deploy` wrapper used this same append-only form at `main.go:137`, so
the atomic dotfiles-deploy path carried the identical latent hang.

**Aggravating condition observed this session.** `kern.num_files` was at
180119 / 184320 (**97.7%** of the system file-descriptor table; 6-day uptime,
many accumulated sessions). The first tool calls of this very task failed with
`ENFILE: file table overflow`. fd exhaustion makes every git/network operation
fragile and turns marginal hangs into hard ones. It is a *separate* systemic
issue (tracked as a follow-up below), but it compounds this one.

**Impact.** Violates the core delegation rule "uncommitted work is nonexistent
work" from the bottom: the commit *exists* but cannot be published, the worker
cannot detect the hang (no timeout), and a supervisor `stop` cannot rescue the
already-lost push. Every worker that pushes a non-github.com context, or runs
the append-only workaround, is exposed.

## Enforce

**The mechanism: a wrapper that makes the safe push the only push** (Core
Principle #9, atomic action wrappers — `safe-push` was named there as an
example that did not yet exist; it does now).

1. **`internal/safegit`** — single source of the fix. `CredentialResetArgs`
   emits an **empty `-c credential.helper=`** (which clears the *entire*
   accumulated helper list, osxkeychain included, because command-line `-c` is
   read last and an empty value resets the whole list) followed by a single
   gh-only helper. The result is gh-only for **every** host, so no credential
   context can fall through to the osxkeychain GUI prompt. `Push` additionally:
   - sets `GIT_TERMINAL_PROMPT=0` (no interactive terminal fallback),
   - bounds the push with a hard timeout (default 30s) via
     `exec.CommandContext`, converting "hang forever" into "fail in seconds
     with a message that names the cause",
   - rejects `--force` / `-f` / `--force-with-lease` by construction.

2. **`cmd/safe-push`** — the CLI workers call instead of `git push`:
   `safe-push [-C <dir>] [--timeout <dur>] <git push args…>`.

3. **`cmd/chezmoi-deploy`** — its push step now uses
   `safegit.CredentialResetArgs`, retiring the append-only form that carried
   the same hang.

**Enforcement wiring (handed off — see Refine).** Per Principle #9 the raw
form should be denied and the wrapper allow-listed. That config lives in
global chezmoi-managed files (a *different* repo), and deploying settings.json
is agent-hostile (`memory/chezmoi-settings-commit-unfriendly.md`), so it is
specified here for the user to deploy rather than committed in this PR:

- **Deny** raw pushes via a `PreToolUse` Bash hook that exits 2 and teaches the
  redirect:
  > You ran `git push`. On this host the raw push can hang on the macOS
  > keychain helper with no TTY. Use `safe-push` (same arguments) — it resets
  > the credential chain to gh-only, sets a timeout, and never force-pushes.
- **`ALWAYS_ALLOW`** `Bash(safe-push *)` — its safety (no osxkeychain, no
  force, bounded) is guaranteed by construction, so it needs no per-invocation
  approval. This turns the fuzzy question "can this agent push?" into the
  crisp one "can this agent run the binary we vetted?"

## Audit

**How to detect recurrence:**

- **The hang itself.** A push with no output for >30s is now impossible *via
  `safe-push`* — it self-terminates and prints
  `git push exceeded 30s and was killed — this is the credential-helper hang…`.
  If that line appears in worker logs, the keychain chain is mis-set; re-check
  the config below.
- **osxkeychain must never head a push's chain.** Spot-check any host:
  ```
  GIT_TRACE=1 printf 'protocol=https\nhost=<host>\n\n' | \
    git -c credential.helper= -c credential.helper='!gh auth git-credential' \
    credential fill 2>&1 | grep run_command
  ```
  Expect a single `gh auth git-credential get`; **any** `credential-osxkeychain`
  line is a regression.
- **The append-only form must not return.** A grep gate catches reintroduction
  in our own code:
  ```
  ! grep -rn "credential.helper=!gh auth git-credential" cmd/ internal/ \
      | grep -v "CredentialResetArgs"
  ```
  (The only legal place to construct the gh helper string is
  `safegit.CredentialResetArgs`.) The package's unit tests assert the empty
  reset precedes the gh helper and that `push` follows the reset.
- **fd pressure (aggravator).** `sysctl kern.num_files kern.maxfiles` near the
  ceiling predicts hangs and tool flakiness; investigate the session/process
  leak before blaming the push.

## Refine

**Shipped in this PR (scoped to the push hang):**

- `internal/safegit` package + unit tests — the single, tested implementation
  of the credential-reset-and-bounded push.
- `cmd/safe-push` CLI.
- `cmd/chezmoi-deploy` push step migrated to the shared reset form.
- `make build-safe-push` / `make install-safe-push` (installs to
  `~/go/bin/safe-push`, on PATH for every agent session).

**Follow-ups (out of scope here — filed, not fixed, per Principle #1):**

1. **Deploy the enforcement wiring** (deny raw `git push` hook + `ALWAYS_ALLOW
   safe-push`) into the dotfiles/chezmoi repo. Until then `safe-push` is
   *available and reliable* but not *mandatory*; the wrapper's value stands
   alone without the deny-hook.
2. **System fd exhaustion** (`kern.num_files` at 97.7%). A separate leak
   (likely accumulated tmux/agm sessions over 6-day uptime) that aggravates
   every hang; needs its own DEAR retro + scoped worker.
3. **Stale ref-lock** `refs/heads/feat/opus-4.8.lock` in `~/src/dear-agent`
   (0-byte, dated May 28, no live git process) blocks fetches of that ref. Not
   removed here (no writes to read-only `~/src`).
4. **Adopt `safe-push` at the remaining call sites** that still invoke
   `git push` directly (CLAUDE.md §4's documented
   `GIT_TERMINAL_PROMPT=0 gtimeout 30 git push` recipe can be replaced by
   `safe-push`, which subsumes the timeout and prompt-suppression).
