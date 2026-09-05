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

**FSG-13** When a command is prefixed by any runner (`env`, `sudo`, `doas`, `nohup`, `setsid`, `exec`, `time`, `nice`, `ionice`, `stdbuf`, `command`, `builtin`) including their option values (e.g. `sudo -u root`), the system shall strip the runner prefix before classifying the underlying command. The runner is matched by the command word's basename, so an absolute or PATH-qualified runner (e.g. `/usr/bin/sudo`) is stripped identically.

**FSG-42** When a command word is an absolute or PATH-qualified path (e.g. `/bin/rm`, `/usr/bin/git`), the system shall classify it by its basename so it cannot bypass the per-command write, git, gh, or runner analysis.

#### Shell nesting

**FSG-14** When a command invokes a shell with a literal argument (`bash -c SCRIPT`, `sh -c SCRIPT`, `eval …`), the system shall recursively inspect the nested script up to a maximum nesting depth of 8.

**FSG-15** When the maximum shell nesting depth (8) is exceeded, the system shall return `(allowed=true, "")` — fail open.

#### Write-target detection

**FSG-43** When a write command's positional argument is a bare relative name (e.g. the `AGENTS.md` in `rm AGENTS.md`) rather than a path-shaped one (absolute, `~`, `$HOME`, `.`/`..`), the system shall resolve it against the current working directory and classify it as a write target.

**FSG-44** The system shall not treat the value of a value-taking option (e.g. the `755` in `mkdir -m 755 d`) or a leading non-path spec operand (the mode/owner/group in `chmod 755 f`, `chown user f`, `chgrp grp f`) as a write target. Because GNU coreutils also accept value-taking options after the operands (e.g. `cp SRC DEST --suffix bak`), the value shall be consumed wherever the option appears, so it never displaces the trailing destination of FSG-18.

**FSG-45** When a write command contains the end-of-options separator `--`, the system shall treat every later token as a positional operand even when it begins with `-`, and shall not classify the `--` token itself as a write target.

**FSG-46** When a simple command contains redirection syntax (the operator, its target, and any leading file-descriptor digit such as the `2` of `2>&1`), the system shall exclude those tokens from the command's operands before selecting write targets, so a redirection cannot displace the destination of FSG-18. The redirect target itself remains classified under FSG-23 through FSG-26.

**FSG-47** When `chmod`, `chown`, or `chgrp` takes its mode/owner/group from `--reference` (e.g. `chmod --reference=RFILE FILE...`), the system shall not drop the leading positional as a spec operand, because in that form the first positional is already a mutation target.

**FSG-48** When `cp`, `mv`, `ln`, or `install` names its destination with `-t`/`--target-directory` (including the `--target-directory=DIR`, `-tDIR`, and clustered `-at DIR`/`-atDIR` forms), the system shall classify that option's value as the write target and treat every positional operand as a read-only source.

**FSG-50** When a word begins with `#`, the system shall treat it as the start of a shell comment and ignore the remainder of the line, so commented words never become operands; a `#` inside a word (e.g. `file#1`) shall remain an ordinary character.

**FSG-51** When a digit is lexically adjacent to a redirection operator (e.g. the `2` of `2>&1`), the system shall treat it as a file descriptor and strip it with the redirection; when whitespace separates them (e.g. `rm 2 > log`), the system shall keep the digit as an ordinary write-target operand.

**FSG-52** When an option value names an auxiliary output location rather than an inert scalar (e.g. `rsync --backup-dir DIR`, `--temp-dir`, `--partial-dir`, `--log-file`, `--write-batch`), the system shall classify that value as a write target *in addition to* the command's positional destination, because such options supplement rather than replace it. Read-only basis directories (`--compare-dest`, `--copy-dest`, `--link-dest`) shall be consumed as ordinary option values and not classified as targets.

**FSG-57** When a short-option cluster contains a letter that consumes a value (e.g. the `-S` of `cp -Stext`), the system shall stop interpreting later letters in that word as options, because they are that option's value; only a `t` reached before any such letter names a target directory.

**FSG-58** When a command contains an input redirection (`<`, `<<`, `<<<`), with or without a file-descriptor prefix (`3<`), the system shall exclude the operator and its target from the command's operands without classifying that target as a write target, since input redirections only read.

**FSG-66** When a short-option cluster ends on a letter that takes a value (e.g. the `-aS` of `cp SRC DEST -aS bak`), the system shall consume the following token as that option's value rather than as an operand, so the value cannot displace the real destination. When the value-taking letter is not last, the remainder of the same word is its value and no further token is consumed.

**FSG-67** When an option's value is optional and therefore only supplied glued with `=` (mktemp's `--tmpdir[=DIR]`), the system shall not consume the following token as its value. A valueless `--tmpdir` shall resolve to `$TMPDIR`, or the platform temporary directory when unset, and shall still replace the positional template as the write target.

**FSG-59** When a `chmod` mode is symbolic and begins with an operator (e.g. the `-w` of `chmod -w FILE`), the system shall treat it as the leading spec operand rather than as an option, so the following positional remains a write target.

**FSG-60** When a redirection target consists only of digits, the system shall treat it as a file descriptor only for a descriptor-duplicating operator (e.g. `2>&1`); for a plain `>` or `>>` it shall classify it as the relative filename it is (`echo x > 2`).

**FSG-61** When `mktemp` selects its directory with `-p`/`--tmpdir`, the system shall classify that directory as the write target and shall not classify the template against the current working directory, because the template is created under the selected directory.

**FSG-68** When a `cd` may not have executed — it follows `&&` or `||`, or it sits on either side of a pipe and therefore runs in a subshell — the system shall keep the previous working directory as an additional candidate and shall classify later relative targets against every candidate, blocking when any candidate resolves inside a protected location. When a `cd` is unconditional the system shall collapse the candidate set, because that `cd` is known to have run.

**FSG-53** When a `cd` occurs inside a subshell (`( … )`), the system shall restore the enclosing working directory once the subshell closes, because the shell does not carry that `cd` past `)`.

**FSG-54** When the command word merely has the basename `cd` but is not the shell builtin (e.g. `/tmp/cd`), the system shall not update the tracked working directory, because an external process cannot change its parent's directory.

**FSG-55** The system shall classify redirect targets against the working directory that `cd` tracking has reached at that point in the command, so `cd ~/src/<repo> && echo x > README.md` resolves the bare target inside the protected checkout.

**FSG-16** When the command is `rm`, `touch`, `mkdir`, `rmdir`, `mv`, `unlink`, `shred`, or `mktemp`, the system shall classify all positional target arguments (see FSG-43, FSG-44) as write targets.

**FSG-17** When the command is `tee`, the system shall classify all positional target arguments (see FSG-43, FSG-44) as write targets (file arguments, not stdin).

**FSG-18** When the command is `cp`, `rsync`, `install`, `ln`, or `link`, the system shall classify only the last positional target argument (see FSG-43, FSG-44; the destination) as a write target.

**FSG-19** When the command is `chmod`, `chown`, `chgrp`, or `truncate`, the system shall classify all positional target arguments (see FSG-43, FSG-44; the leading mode/owner/group spec is not a target) as write targets.

**FSG-20** When the command is `dd`, the system shall classify only the `of=` argument as a write target.

**FSG-21** When the command is `sed` or `gsed` with the `-i` flag, the system shall classify path-shaped positional arguments as write targets while correctly skipping the substitution expression (e.g. `s/a/b/`).

**FSG-22** When the command is `sed` or `gsed` without the `-i` flag, the system shall not classify any argument as a write target.

**FSG-23** When the command is `perl` with the `-i` flag, the system shall classify path-shaped positional arguments as write targets.

#### Redirection detection

**FSG-24** When a command contains a `>` or `>>` redirection operator, the system shall classify the redirect target as a write target.

**FSG-25** When a command contains a `&>` redirection, the system shall classify the redirect target as a write target.

**FSG-26** When a redirection token is an fd-dup (e.g. `2>&1`) or a bare digit, the system shall not classify it as a write target. Any other redirect target, including a bare relative name (e.g. `> README.md`), is classified against the current working directory.

#### `cd` tracking

**FSG-27** When a command sequence contains a `cd` invocation, the system shall update the current working directory used for relative-path resolution for all subsequent commands in the sequence.

#### Git enforcement

**FSG-28** When a git subcommand that is read-only (`log`, `diff`, `status`, `show`, `blame`, `describe`, `rev-parse`, `rev-list`, `cat-file`, `ls-files`, `ls-tree`, `shortlog`, `stash list`, `tag`, `fetch`, `remote`, `submodule`) is invoked within `~/src/`, the system shall allow it.

**FSG-29** When `git push` is invoked within `~/src/` without a force flag, the system shall allow it.

**FSG-30** When `git push` is invoked with a force form and its resolved destination is `main`, `master`, the repository's default branch, or a branch listed in `safegit.protectedbranch`, the system shall block it. Force detection is delegated to `safegit.ForceFlag`, so this guard and `safe-push` share one definition: `--force`, `-f`, any short-option cluster containing `f` (`-uf`), `--force-with-lease[=<ref>]`, `--force-if-includes`, `--mirror`, and a leading-`+` force refspec, with tokens after a bare `--` treated as repository/refspec rather than flags, and an abbreviated long option matched after its `=value` is separated.

**FSG-64** When `git push` is invoked with a force form and its resolved destination is a branch that is neither protected nor the repository's default, the system shall allow the push, because rewriting a pull-request branch affects no shared history.

**FSG-69** When the repository's default branch cannot be established from `refs/remotes/origin/HEAD`, the system shall refuse every force push and shall name `git remote set-head origin --auto` as the remedy, rather than inferring a default from whichever conventional branch happens to exist. A shallow single-branch checkout, such as the one `actions/checkout` produces, has no such ref, so force pushes are refused there until the ref is established.

**FSG-65** When a `git push` inside `~/src/` carries a force form, the system shall block it only when the resolved destination is a protected branch, and shall refuse when the destination cannot be resolved or the repository's default branch cannot be established. Resolution is delegated to `safegit.ForcePushViolation`, so this guard, `safe-push`, and the PreToolUse bypass hooks share one decision.

**FSG-56** When a `git push` short-option cluster contains an `f` (e.g. `-uf`), the system shall treat it as a force push, because `-f` is push's only short option spelled with an `f`. Where a value-taking short option precedes it (e.g. `-ofoo`), the system shall stop scanning at that option, because the remainder of the word is its value.

**FSG-62** When a `git push` long option is an abbreviation of a destructive option (e.g. `--mir` for `--mirror`, `--forc` for `--force`), the system shall treat it as that option, because Git itself accepts unambiguous abbreviations; `--no-`-prefixed forms shall be excluded.

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


### Supervisor Role Policy (`DetectSupervisor`, `CheckSupervisor*`)

This layer answers a different question from the rest of the package. The path
policy asks "may this path be written?"; the role policy asks "may this session
perform implementation work at all?". A VROOM supervisor coordinates workers and
must never do the work itself: a detached supervisor cannot answer a permission
prompt, so a modal raised by its own tool call wedges it and stalls dispatch for
the whole mesh, and implementation detail consumes the context window the
coordination role depends on.

**FSG-70** When `AGM_SUPERVISOR_ID` is non-empty, the system shall classify the session as a supervisor and return that value as the identity, including a value that is not canonical, because that variable is set only for supervisors.

**FSG-71** When `AGM_SESSION_NAME` matches a canonical VROOM supervisor identity or compact alias, case-insensitively and as an exact match rather than a prefix, the system shall classify the session as a supervisor. Both markers are checked because `agm supervisor run` sets the first while `vroom-dispatch` spawns through `agm session new` and sets only the second.

**FSG-72** When neither marker identifies a supervisor, the system shall classify the session as a worker, and no role rule shall apply to it.

**FSG-73** When a supervisor invokes a file-mutation tool, the system shall allow the write only when the resolved target lies under a control-plane root (`/dev`, the temp directories, or `~/.agm`), and shall block every other target with delegation guidance.

**FSG-74** When a supervisor's write target cannot be resolved to an absolute path, or is empty, the system shall block it. The role policy fails CLOSED, unlike path classification, because a supervisor has no legitimate write outside the control-plane set.

**FSG-75** When a supervisor runs a Bash command, the system shall evaluate it through the shared segment walker, so runner stripping, shell nesting, redirections, and `cd` tracking behave exactly as they do for the path policy.

**FSG-76** When a supervisor runs a git subcommand that cannot mutate a repository under any flags, the system shall allow it; when the subcommand has both a read and a write mode (`branch`, `remote`, `worktree`, `tag`, `config`, `stash`), the system shall allow it only when every argument is a recognised read flag, the value of a value-taking read flag, a read verb, or a positional following a read verb; and the system shall block every other git subcommand wherever it runs, including inside a worktree, because producing the commit is the worker's job.

**FSG-77** When a supervisor's Bash command cannot be tokenised, the system shall fail open, matching `InspectCommand`, so a guard defect can never brick a supervisor's read and dispatch path.

**FSG-78** The role policy shall have no graduated enforcement. Every refusal is a hard block, because an `ask` decision would raise the very modal the policy exists to prevent.
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
