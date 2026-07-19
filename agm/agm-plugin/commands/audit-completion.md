---
model: haiku
effort: low
content-hash: a089d7a29d1a3aeae0fe890b2fa1a94f22b7b8784c57bdb71a5c57159d596f3b
description: Audit delivery evidence for an AGM worker before archival. Use when a reviewer needs to distinguish committed, merged, deployed, and verified state without changing anything.
argument-hint: "<session-name>"
allowed-tools: Bash(agm session get *), Bash(agm acceptance show *), Bash(git -C *), Bash(gh repo view *), Bash(gh pr view *)
---

# Audit session completion

1. Require a session name and run `agm session get <session-name> --output json`.
   Extract the project/worktree path and stated purpose; stop if the path is
   missing or is not a git worktree.
2. Run `agm acceptance show -C <project-path> --output json` for the repository's
   declared acceptance criteria. The global `-C` is the lookup root; do not use
   the reviewer's current directory as a substitute.
3. Read only delivery evidence:
   - `git -C <project-path> status --porcelain`
   - `git -C <project-path> branch --show-current`
   - `git -C <project-path> rev-parse HEAD`
   - Resolve the current branch with `git -C <project-path> branch --show-current`.
   - Resolve the provider repository with
     `gh repo view <origin-url> --json nameWithOwner --jq .nameWithOwner`, where
     `<origin-url>` comes from `git -C <project-path> remote get-url origin`.
   - Query that exact branch and repository with
     `gh pr view <branch> --repo <owner/repo> --json state,mergedAt,mergeCommit,url`.
4. Report each gate separately:
   - committed and clean;
   - merged, proven by the provider rather than local branch ancestry;
   - deployed when the component has a deploy target;
   - verified against the declared acceptance criteria.
5. Mark a gate `unknown`, not `pass`, when the evidence cannot prove it. Do not
   infer test quality from filenames, scan commit messages for intent, or treat
   a PR/branch as merged.
6. Conclude `complete` only when every applicable gate has current evidence.
   Otherwise list the exact missing evidence and leave archival blocked.
