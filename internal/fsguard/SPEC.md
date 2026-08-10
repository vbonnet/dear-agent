# internal/fsguard — Requirements Specification (EARS)

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-07-03 -->

**Version**: 1.1
**Last Updated**: 2026-06-12
**Status**: Baseline (derived from tests + code, not design-forward)
**Scope**: Filesystem write-gate for tools that mutate files (Edit, Write, Bash)

---

## Overview

`fsguard` is the pre-tool write-gate enforced by Claude Code hooks. It
classifies write targets as allowed or blocked, and inspects raw Bash command
strings for filesystem-mutating operations. Every block message must follow the
positive-guidance pattern: explain what was attempted, show the correct path,
state why.

---

## EARS Requirements

### Construction

**FSG-01** When `New()` is called, the system shall resolve the home directory path through symlinks at construction time to handle macOS aliases (e.g. `/tmp` → `/private/tmp`).

### Path Classification (`Classify`)

**FSG-02** When a path falls under `~/worktrees/`, the system shall allow the write.

**FSG-03** When a path resolves to any carveout (`/dev/*`, `~/.auto-memory/*`, `/tmp`, `/private/tmp`, `/var/tmp`, `/private/var/tmp`, `/var/folders/*`, `/private/var/folders/*`, `/sessions/*`), the system shall allow the write.

**FSG-04** When a path falls under `~/src/`, the system shall block the write with a message that identifies the attempted action, provides the exact `git -C ~/src/<repo> worktree add ~/worktrees/<repo>/<branch>` command for the specific repository, and explains why writes to `~/src/` are blocked.

**FSG-05** When a path falls under `~/` and the first path component starts with `.` (a dotfile or dotdir), the system shall block the write with a message that identifies the chezmoi source-edit workflow.

**FSG-06** When a path is within `~/` but is neither a worktree, carveout, `~/src/` path, nor dotfile, the system shall block the write with a generic worktree-creation guidance message.

**FSG-07** When a path is outside `~/` and is not a carveout, the system shall block the write with a generic worktree-creation guidance message.

**FSG-08** When path expansion encounters any error (malformed input, unresolvable variable), the system shall return `(allowed=true, "")` — fail open — so a guard bug never disables the Edit/Write tools.

**FSG-09** When a path is specified using `${HOME}/...` or `$HOME/...` environment-variable form, the system shall expand it before classification.

### Bash Command Inspection (`InspectCommand`)

#### Tokenisation

**FSG-10** When a Bash command string contains a backslash-newline line continuation, the system shall merge the continuation before splitting tokens, so `rm \<newline>~/src/f` is not treated as two separate tokens.

**FSG-11** When a Bash command string contains an unterminated quote, the system shall return `(allowed=true, "")` — fail open.

**FSG-12** When processing a multi-line command string, the system shall inject an explicit `;` between physical lines so arguments never bleed across statement boundaries.

#### Command-runner stripping

**FSG-13** When a command is prefixed by any runner (`env`, `sudo`, `doas`, `nohup`, `setsid`, `exec`, `time`, `nice`, `ionice`, `stdbuf`, `command`, `builtin`) including their option values (e.g. `sudo -u root`), the system shall strip the runner prefix before classifying the underlying command.

#### Shell nesting

**FSG-14** When a command invokes a shell with a literal argument (`bash -c SCRIPT`, `sh -c SCRIPT`, `eval …`), the system shall recursively inspect the nested script up to a maximum nesting depth of 8.

**FSG-15** When the maximum shell nesting depth (8) is exceeded, the system shall return `(allowed=true, "")` — fail open.

#### Write-target detection

**FSG-16** When the command is `rm`, `touch`, `mkdir`, `rmdir`, `mv`, `unlink`, `shred`, or `mktemp`, the system shall classify all path-shaped positional arguments as write targets.

**FSG-17** When the command is `tee`, the system shall classify all path-shaped positional arguments as write targets (file arguments, not stdin).

**FSG-18** When the command is `cp`, `rsync`, `install`, `ln`, or `link`, the system shall classify only the last path-shaped positional argument (the destination) as a write target.

**FSG-19** When the command is `chmod`, `chown`, `chgrp`, or `truncate`, the system shall classify all path-shaped positional arguments as write targets.

**FSG-20** When the command is `dd`, the system shall classify only the `of=` argument as a write target.

**FSG-21** When the command is `sed` or `gsed` with the `-i` flag, the system shall classify path-shaped positional arguments as write targets while correctly skipping the substitution expression (e.g. `s/a/b/`).

**FSG-22** When the command is `sed` or `gsed` without the `-i` flag, the system shall not classify any argument as a write target.

**FSG-23** When the command is `perl` with the `-i` flag, the system shall classify path-shaped positional arguments as write targets.

#### Redirection detection

**FSG-24** When a command contains a `>` or `>>` redirection operator, the system shall classify the redirect target as a write target.

**FSG-25** When a command contains a `&>` redirection, the system shall classify the redirect target as a write target.

**FSG-26** When a redirection token is an fd-dup (e.g. `2>&1`) or a bare digit, the system shall not classify it as a write target.

#### `cd` tracking

**FSG-27** When a command sequence contains a `cd` invocation, the system shall update the current working directory used for relative-path resolution for all subsequent commands in the sequence.

#### Git enforcement

**FSG-28** When a git subcommand that is read-only (`log`, `diff`, `status`, `show`, `blame`, `describe`, `rev-parse`, `rev-list`, `cat-file`, `ls-files`, `ls-tree`, `shortlog`, `stash list`, `tag`, `fetch`, `remote`, `submodule`) is invoked within `~/src/`, the system shall allow it.

**FSG-29** When `git push` is invoked within `~/src/` without `--force`, `-f`, or `--force-with-lease`, the system shall allow it.

**FSG-30** When `git push` is invoked with `--force`, `-f`, or `--force-with-lease` to `main`, `master`, or the repository's configured default branch, the system shall block it.

**FSG-30a** When `git push` is invoked with `--force`, `-f`, or `--force-with-lease` to a non-default PR branch, the system shall allow it. `--force-with-lease` is preferred.

**FSG-31** When `git merge`, `git pull`, `git fetch`, `git clone`, or `git worktree` is invoked within `~/src/`, the system shall allow it.

**FSG-32** When any other write git subcommand (`commit`, `checkout`, `reset`, `rebase`, `branch`, `add`, `rm`, etc.) is invoked with a working directory under `~/src/`, the system shall block it with worktree-creation guidance.

**FSG-33** When a git write subcommand is invoked with a working directory under `~/worktrees/`, the system shall allow it.

### Graduated Enforcement

**FSG-34** When `Policy.Enforcement` is `EnforceDeny` (the default), the hook shall exit with code 2 and write the positive-guidance message to stderr.

**FSG-35** When `Policy.Enforcement` is `EnforceWarn`, the hook shall exit 0 and write a JSON hook response `{"permissionDecision":"allow","message":"..."}` to stdout; the write proceeds but the agent receives the guidance message.

**FSG-36** When `Policy.Enforcement` is `EnforceAsk`, the hook shall exit 0 and write a JSON hook response `{"permissionDecision":"ask","message":"..."}` to stdout; Claude Code prompts the user before proceeding.

**FSG-37** When `Policy.Enforcement` is `EnforceDefer`, the hook shall exit 0 and write a JSON hook response `{"permissionDecision":"defer"}` to stdout; the decision falls through to the normal Claude Code permission model.

**FSG-38** When `FSGUARD_ENFORCEMENT` contains a recognized value (`deny`|`warn`|`ask`|`defer`, case-insensitive), the system shall make `LoadConfig` override the configured `Policy.Enforcement` with the parsed level.

**FSG-39** When a violation occurs under a non-Deny enforcement level, the system shall still record the violation so the retro audit captures attempted out-of-policy writes even when the write was ultimately allowed or deferred.

**FSG-40** When a write is allowed through a writable carveout or `WorktreesDir`, the system shall set `Decision.Enforcement` to `EnforceDeny` (the zero value), since enforcement level is irrelevant for allowed writes.

**FSG-41** When a path falls under `~/.agm/vroom/` or `~/.agm/sandboxes/`, the system shall allow the write because these are VROOM supervisor runtime-state directories (heartbeat files, trail logs, dispatch ledger, worker sandbox trees), and the carveout shall remain namespaced so `~/.agm/vroomX/` or `~/.agm/sandboxesX/` prefix lookalikes are still blocked.

---

## Key Invariants

- **Fail open, always.** Parse errors, unterminated quotes, shell-depth
  exhaustion, and unrecognised inputs all return `(allowed=true, "")`. The
  guard is defence-in-depth; `settings.json` deny rules remain the
  authoritative backstop.
- **Positive-guidance messages.** Every block message must tell the agent
  what to do (worktree creation command, chezmoi workflow) and why, not just
  "access denied".
- **Symlink-resistant path expansion.** `New()` resolves symlinks at
  construction; carveouts include both canonical and symlinked forms
  (e.g. `/var/tmp` and `/private/var/tmp`) explicitly.
- **Runner stripping closes the sudo bypass.** `sudo rm ~/src/f`,
  `env rm ~/src/f`, `nohup rm ~/src/f` all reduce to `rm ~/src/f` before
  classification.
