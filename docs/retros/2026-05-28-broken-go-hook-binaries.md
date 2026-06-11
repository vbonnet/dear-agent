# DEAR Retro: Broken Go Hook Binaries

Date: 2026-05-28

> Scope: explain why the filesystem-write enforcement hooks
> (`pretool-worktree-enforcer`, `pretool-bash-blocker`) broke and had to be
> replaced with Python stopgaps. Investigation + retro only — no fixes were
> applied. Action items here become the spec for the "Rebuild hooks in Go"
> task (`ce-1ob`).

## Define

### What broke

The two PreToolUse enforcement hooks that gate filesystem writes —
`pretool-worktree-enforcer` (write-destination policy: block `~/src/`,
redirect to worktrees, block non-repo writes) and `pretool-bash-blocker`
(block dangerous/`~/src/`-targeting shell commands) — were deployed to
`~/.claude/hooks/` as **prebuilt Mach-O arm64 binaries with no recoverable
source in the repo**. By 2026-05-28 they were stale/non-functional and could
not be rebuilt, so they were replaced with Python stopgaps.

### What the evidence shows

1. **The deployed binaries are orphaned from their source.** All seven Go
   hook binaries in `~/.claude/hooks/` carry an identical build timestamp of
   **Feb 24 08:18** (`pretool-worktree-enforcer`, `pretool-bash-blocker`,
   `pretool-beads-protection`, `pretool-validate-paired-files`,
   `posttool-auto-commit-beads`, `sessionstart-guardian`,
   `sessionstart-auto-bead-load`). They are 3–4 MB Mach-O executables.

2. **`pretool-worktree-enforcer` source never existed in this repo.**
   `git log --all --diff-filter=A -- '*pretool-worktree-enforcer/main.go'`
   returns nothing. The only traces are *documentation* in
   `engram/hooks-bin/README.md` and `engram/hooks-bin/GO-MIGRATION-PLAN.md`,
   which point to a source path `hooks/cmd/pretool-worktree-enforcer/main.go`
   (and an `IMPLEMENTATION_STATUS.md`) **that does not exist** in the tree.
   The same is true for `sessionstart-guardian`, `posttool-auto-commit-beads`,
   `pretool-validate-paired-files`, and `pretool-beads-protection`.

3. **The build machinery that *is* in the repo builds different binaries.**
   `engram/hooks-bin/Makefile` builds `posttool-error-collector`,
   `prepush-act-validator`, `sessionstart-bead-coverage`, and
   `sessionend-bead-coverage`. **None** of the seven deployed binaries are
   produced by it. There is no `make`/build path in this repo that reproduces
   what is installed.

4. **`pretool-bash-blocker` has a second, divergent source lineage.** A
   `pretool-bash-blocker/main.go` exists at
   `agm/cmd/agm-hooks/pretool-bash-blocker/main.go` (last modified May 3–4),
   plus a 33 KB `bash-anti-patterns.yaml`. But the deployed binary predates
   that source by ~10 weeks (Feb 24 vs May 4) and was never rebuilt or
   redeployed from it. Source and binary drifted apart silently.

5. **The provenance is a lost predecessor repo.** dear-agent received its
   history via `02c784d0a1 chore: squash-export ai-tools 2026-04-25` and
   `570dcbfafd Migrate post-squash work from ai-tools (#36)` (May 2). The
   binaries were built Feb 24 — **two months before** the ai-tools →
   dear-agent export. The hook `cmd/` source trees lived in ai-tools (under
   `hooks/cmd/...` and `swarm/completed/hooks-rewrite/...`, both absent here)
   and were **not carried over** in the squash-export. The README/migration
   docs survived; the code did not.

6. **chezmoi does not track the binaries.** `chezmoi managed` for
   `.claude/hooks` covers only the small Python/shell hooks
   (`pretool-config-guard.py`, the chezmoi-drift scripts, the disabled-hooks
   README). The seven Go binaries are unmanaged on the host — not in git, not
   in chezmoi, not reproducible.

7. **The stopgap.** On **May 28** two Python guards were dropped in at
   `~/.config/claude-code/hooks/pretool-fs-write-guard` (4.7 KB) and
   `~/.config/claude-code/hooks/pretool-bash-write-guard` (12 KB), and
   `settings.json` was rewired to call them. The old Go binaries still sit in
   `~/.claude/hooks/` but are **no longer referenced** by any matcher in
   `settings.json`. The Python guards are explicitly fail-open and lean on the
   `settings.json` deny rules as a backstop.

### Impact

- **Enforcement gap window.** Between the binaries going stale and the May 28
  Python replacement, the worktree-only write policy and bash-blocking were
  running on binaries that could not be inspected, tested, or corrected. The
  guardrail most relied on to keep agents out of golden `~/src/` was in an
  unverifiable state.
- **Unrebuildable security control.** A control whose source is gone cannot be
  patched when policy changes (e.g. the `~/.config/` allow-list, new worktree
  roots). The only available remediation was wholesale replacement.
- **Capability regression.** The Python stopgaps are deliberately minimal
  (fail-open, no LRU git cache, no session-state worktree redirection, no YAML
  config) versus the documented Go behaviour — a functional downgrade accepted
  to restore *some* enforcement quickly.

### Timeline

| Date | Event |
|------|-------|
| 2026-02-11 | SessionStart hook perf crisis; several hooks disabled (`README-HOOKS-DISABLED.md`). |
| 2026-02-24 08:18 | All 7 Go hook binaries built (ai-tools era) and installed to `~/.claude/hooks/`. |
| 2026-04-25 | `ai-tools` squash-exported into dear-agent — hook `cmd/` source trees **not** carried over. |
| 2026-05-02 | Post-squash migration (#36) — still no hook source brought in. |
| 2026-05-03/04 | A *new, divergent* `pretool-bash-blocker` source appears under `agm/cmd/agm-hooks/`; deployed binary never rebuilt from it. |
| 2026-05-28 | Go binaries found broken/stale; replaced with Python guards in `~/.config/claude-code/hooks/`; `settings.json` rewired. |

### Root cause

The deployed enforcement hooks were **binary artifacts with no source of
truth in version control**. Their source lived in a predecessor repo
(ai-tools) and was dropped during the squash-export, leaving documentation
that describes code which no longer exists. With no source and no reproducing
build, normal staleness (policy drift, a path/OS assumption going stale) became
unrecoverable, forcing a stopgap rewrite. A second copy of the bash-blocker
source existing in `agm/` but never wired to the deployment made the drift
invisible — it *looked* like the source was present.

## Enforce

Mechanisms that would have prevented this (not "be more careful"):

1. **No binary may be deployed without its source building in CI.** Every
   binary installed to `~/.claude/hooks/` must be produced by a checked-in
   `make`/`go build` target in this repo. If the source isn't here, the binary
   doesn't ship. This directly prevents the orphaned-binary state.

2. **Single source of truth per hook.** Exactly one `cmd/<hook>/main.go`
   location per deployed hook, referenced by both the Makefile and the install
   script. The current split (`engram/hooks-bin/` docs vs `agm/cmd/agm-hooks/`
   source vs a Makefile that builds neither) is the structural defect.

3. **Install-from-build, never copy-a-prebuilt.** The install step must
   `go build` from repo source into `~/.claude/hooks/`, stamping the binary
   with its source commit. Copying an opaque prebuilt binary (what happened
   Feb 24) is what allowed source and deployment to diverge.

4. **Migration completeness gate.** A repo squash/export must fail if it drops
   a `cmd/` tree that has a corresponding documented/deployed binary. The
   ai-tools → dear-agent export silently lost the hook sources while keeping
   their READMEs; an export manifest check would have caught the orphaned docs.

## Audit

How we detect this happening again:

1. **Hook source↔binary reconciliation check** (CI + `engram doctor` /
   `agm admin doctor`): for every command in `settings.json` (and plugin
   `hooks.json`) that resolves to an executable binary, assert a checked-in
   build target produces a byte-identical (or commit-stamped) artifact. Fail
   on any deployed hook with no buildable source.

2. **Stale-binary detector.** Compare each deployed hook binary's embedded
   source commit / build timestamp against the latest commit touching its
   source dir. The Feb-24-binary-vs-May-4-source drift would have fired
   immediately. (Today's `sessionstart-guardian` only stat-checks *existence*,
   not source correspondence or staleness — that is why it reported healthy
   while the source was gone.)

3. **Orphaned-doc lint.** Flag README/SPEC files that reference a
   `cmd/.../main.go` or `Location:` path that does not exist on disk. This
   catches the "docs survived, code didn't" signature directly.

4. **`settings.json` ↔ repo coverage report.** Periodically diff the hooks
   wired in live `settings.json` against the hooks this repo can build/install,
   surfacing both unmanaged deployed hooks (the Go binaries) and host-only
   stopgaps (`~/.config/claude-code/hooks/*`, currently untracked).

## Refine

Action items — these become the spec for "Rebuild hooks in Go" (`ce-1ob`):

- [ ] **Recover or reimplement `pretool-worktree-enforcer` source in-repo.**
  No source survives; reconstruct from `engram/hooks-bin/README.md` lines
  304–345 (golden-ref block, non-repo block, worktree redirect, the
  `/tmp`,`~/.csm`,`~/.claude`,`~/.wayfinder`,`~/.config`,`~/.local`,`~/.agm`
  allow-list, exit codes 0/1/2) and from the new Python `pretool-fs-write-guard`
  behaviour as the de-facto current spec. Place at one canonical
  `cmd/pretool-worktree-enforcer/`.
- [ ] **Consolidate `pretool-bash-blocker` to one source.** Adopt the surviving
  `agm/cmd/agm-hooks/pretool-bash-blocker/main.go` + `bash-anti-patterns.yaml`
  as canonical (or migrate it), and delete/redirect the divergent lineage so
  there is exactly one buildable source.
- [ ] **Add a Makefile target that builds *all* deployed hooks** (the 7 from
  Feb 24, not the unrelated set the current `engram/hooks-bin/Makefile`
  builds), cross-compiling darwin/arm64 + the CI target.
- [ ] **Install-from-source script** that `go build`s into `~/.claude/hooks/`
  and stamps each binary with its source commit (ldflags), replacing the
  copy-a-prebuilt pattern.
- [ ] **Port the Python stopgaps' behaviour faithfully** so the Go rebuild is
  at least at parity with what's live today (fail-open default, deny-rule
  backstop, `PERMISSION_ESCALATION` guidance, `~/.config/` allow-list).
- [ ] **Wire the audit checks** (source↔binary reconciliation, stale-binary,
  orphaned-doc lint) from the Audit section into CI and `engram doctor`.
- [ ] **Decide the stopgap's fate.** Keep the Python guards at
  `~/.config/claude-code/hooks/` as the documented fail-open backstop, or
  retire them once the Go binaries are rebuilt — but track them somewhere
  (chezmoi or repo) so they are not themselves orphaned.
- [ ] **Reconcile the orphaned docs.** Update `engram/hooks-bin/README.md` and
  `GO-MIGRATION-PLAN.md` to point at real source paths, or mark them
  superseded, so they stop describing code that isn't there.

### Connection to existing process rules

This is the same failure class the project already learned once:
`docs/retros/` and the DEAR enforcement rules in `.claude/CLAUDE.md` exist
because **uncommitted/unversioned work is nonexistent work**. A binary with no
source in git is the artifact-level version of that exact rule — "the code
works on disk" but it is not in version control, so it is not done and not
maintainable.
