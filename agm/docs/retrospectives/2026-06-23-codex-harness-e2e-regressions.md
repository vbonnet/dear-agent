# Retrospective Notes: Codex Harness E2E Regressions

**Date**: 2026-06-23
**Status**: Draft notes from live E2E verification
**Scope**: AGM `codex-cli` harness parity after PR #716

## Define

The intended acceptance criteria were broader than unit-level parity:

- the locally installed `agm` binary must contain the merged Codex harness work;
- `agm session new --harness codex-cli` must create a usable Codex TUI session and a Dolt record;
- `agm send msg`, `agm session state detect`, `agm session kill`, `agm session resume`, and `agm session archive` must work against that session;
- Codex must not depend on Claude-only slash-command association; Dolt registration during `session new` is the Codex equivalent of association.

## Execute Findings

1. **Invalid Codex flag reached production path.**
   `codex --skip-git-repo-check` is not accepted by the installed Codex CLI. AGM still registered the session after the launch failed.

2. **Readiness detection was too weak.**
   Generic rounded box-drawing characters could be mistaken for a TUI-ready signal. A shell prompt or non-Codex TUI chrome must not count as Codex readiness.

3. **Codex startup has non-composer interstitials.**
   The live Codex CLI showed both a trust prompt and a model-upgrade prompt. AGM answered trust but previously did not handle model selection, so detached session creation timed out and still registered.

4. **Codex readiness failure was non-fatal.**
   `startCodexHarness` logged a readiness timeout as non-fatal and printed `Codex adapter ready`. That let broken sessions enter Dolt as active.

5. **`agm send msg` had a Claude-only safety assumption.**
   The uninitialized guard required a running Claude process, so a healthy Codex composer was blocked with `session_uninitialized`.

6. **Direct `send msg --workspace ...` path is broken.**
   A direct-recipient send with `--workspace oss` returned `workspace-only filtering not yet implemented`. Retrying without `--workspace` succeeded after the Codex safety fix.

7. **Global tmux socket overrides can corrupt status reconciliation.**
   Running broad list/status reconciliation with `AGM_TMUX_SOCKET` pointed at an isolated socket can mark unrelated production sessions stopped. E2E tests should isolate Dolt as well as tmux, or status reconciliation must scope updates to the selected test environment.

8. **`agm -C ... session new` did not drive the session workdir.**
   The attempted `-C` run still used process `$PWD` in the new-session path. The live test had to set the command working directory directly.

9. **Archive cleanup warns on sandbox merged dirs.**
   `agm session archive` tried `git worktree prune` inside an AGM sandbox merged dir and warned because it was not a git repository. Archive still completed.

10. **Kill confirmation hints can ladder.**
    A stuck active/recently-active session alternated between asking for `--force` and `--confirmed-stuck`; passing both worked.

11. **Hard worker-count cap blocked valid harness testing.**
    `session new` refused when three `role:worker` sessions existed, even for unrelated validation work. A hard-coded worker cap is the wrong control for supervisor runaway prevention.

## Audit Implemented

This branch adds or updates unit-level regression coverage for:

- Codex command construction omitting `--skip-git-repo-check`;
- Codex readiness requiring Codex-specific composer text;
- Codex model-upgrade prompt handling;
- fatal Codex startup readiness failure;
- Codex-specific `session_uninitialized` safety detection;
- removal of the `max_workers` gate from the circuit breaker.

Live E2E verification covered:

- `agm session new --harness codex-cli`;
- Dolt registration and status updates;
- Codex trust and model prompt handling;
- `agm send msg` direct tmux delivery and Codex response;
- `agm session state detect`;
- `agm session kill`;
- `agm session resume --detached`;
- `agm session archive`.

## Tests To Add Next

### Unit

- `send.ParseRecipients` or `runSend` should cover direct recipient + `--workspace` so that single-recipient sends do not take the unsupported workspace-only path.
- `session new -C` should have a command-level test proving the global directory flag is used as the session workdir.
- Kill confirmation logic should have table tests for active, recently-active, and active+recently-active sessions with expected flags.
- Archive cleanup should detect sandbox merged dirs and avoid git-worktree pruning when the path is not a git repository.

### Integration

- Add a fake `codex` binary fixture that emits trust, model-upgrade, composer, and response text. Use it to test `session new`, `send msg`, `state detect`, `kill`, `resume`, and `archive` without burning real Codex usage.
- Add an isolated Dolt+tmux test environment for `session list` reconciliation so broad listing cannot update production session status.
- Add an integration test that creates more than three worker-tagged records and proves `session new` is not blocked by worker count.

### BDD / E2E

- Create a Codex harness feature file mirroring the Claude lifecycle BDD: new, Dolt registration, send, state detect, kill, resume, archive.
- Include a scenario for Codex startup interstitials: trust prompt, model-upgrade prompt, then composer.
- Include a scenario for launch failure: fake Codex exits with an unknown flag/error and AGM must not register an active session.

### CI / Enforce

- Add a lightweight harness-contract CI job that runs fake CLI harnesses for Claude/Gemini/Codex/OpenCode lifecycle paths.
- Add a static audit that fails if harness-specific startup code includes unsupported CLI flags not present in a versioned fixture contract.
- Add an audit check that flags Claude-only assumptions in shared send/safety paths when the target harness is not Claude.
- Add a CI test mode that uses isolated storage and tmux sockets; prohibit broad status reconciliation against production storage in tests.

## Retro Actions

- **Definition**: Codex parity means live lifecycle behavior, not just command construction and docs. Future harness parity tasks should list the lifecycle commands that must pass.
- **Enforce**: CI should include fake-harness lifecycle tests for every CLI harness before merge.
- **Audit**: Periodic AGM audit should replay a cheap fake-harness E2E and verify Dolt/tmux/session state transitions.
- **Retro**: Any live E2E-only bug found after merge should become either a fake-harness integration test or an explicit documented manual gate if automation is impractical.
