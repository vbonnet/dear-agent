# AGM tmux Delivery Specification

<!-- Last audited at: 2026-07-20 -->

## Purpose

`agm/internal/tmux` owns low-level tmux delivery primitives for AGM-managed
sessions. The package normalizes session names, waits for harness readiness,
serializes send-keys operations, stashes potentially human-authored input, and
keeps prompt delivery behavior consistent across supported CLI harnesses. It
also owns the harness-process liveness scan (ce-axsr): proving a harness
process is actually running in a pane's process tree, because tmux session
existence alone is a false-green liveness signal. It also verifies and repairs
new-session working directories when tmux silently ignores `new-session -c`
because the tmux server's own cwd has been deleted.

## EARS Requirements

**TMUX-01** When a safe send function is called, the system shall wait for a recognized harness prompt before delivering input.

**TMUX-02** When a prompt file is sent, the system shall reject missing files and files larger than the configured prompt-size limit before touching tmux.

**TMUX-03** When multiline prompt delivery observes that the composer disappeared after readiness detection, the system shall abort delivery unless the caller explicitly bypassed the post-submit guard.

**TMUX-04** When human input is detected before prompt delivery, the system shall stash the existing input non-blockingly before sending automation.

**TMUX-05** When sending keys to tmux, the system shall normalize the target session and serialize send-keys operations to prevent cross-session byte interleaving.

**TMUX-06** When harness-process liveness is checked for a session, the system shall scan the full descendant process tree of every pane and shall not treat tmux session existence alone as proof of liveness.

**TMUX-07** When a liveness scan finds no harness process but finds an agm process in a pane's descendant tree, the system shall report the zombie-writer condition alongside the dead verdict.

**TMUX-08** When a liveness scan classifies a session as dead, the system shall include the pane's descendant process names as evidence so callers can report why the session was treated as dead.

**TMUX-09** When a new tmux session is created with a requested work directory, the system shall verify the active pane's `pane_current_path` before launching the harness.

**TMUX-10** When tmux starts the pane outside the requested work directory, the system shall send a corrective quoted `cd` command after a short grace period.

**TMUX-11** When the pane cannot be verified in the requested work directory before the deadline, the system shall return an error that explains tmux may have ignored `new-session -c` because the tmux server cwd was deleted.

**TMUX-12** When work directory comparison runs, the system shall canonicalize paths and tolerate symlink resolution and trailing slash differences.

**TMUX-13** When Claude liveness is checked and tmux reports a shell or `agm` wrapper as the foreground command, the system shall inspect captured pane output for the Claude prompt before declaring Claude not running.

**TMUX-14** When pane output or scrollback is captured for any active harness, the system shall invoke tmux through the canonical AGM socket and normalize the target session name.

**TMUX-15** When pane output or scrollback capture starts, the system shall bound the tmux subprocess with a timeout, isolate its process group, and bound command waiting after cancellation.

**TMUX-20** When the AGY feedback survey is visible with a bare prompt, the system shall classify the session as blocked rather than ready and shall select the survey's Skip option before delivering automated input; after one successful dismissal in a readiness wait, stale survey text retained in captured pane history shall neither trigger another dismissal nor suppress a subsequently visible composer.

**TMUX-21** When the operating-system user database cannot resolve the current process UID, the system shall use the numeric UID and environment username for linger diagnostics.

**TMUX-22** When `CI_SKIP_TMUX=true`, the test suite shall skip tmux-dependent integration tests while continuing to execute pure tmux unit tests.

**TMUX-23** When detecting whether a pasted prompt is still unsubmitted, the system shall recognize both the `[Pasted text` indicator and the codex `[Pasted Content` chip as a stuck paste.

**TMUX-24** When delivering a prompt via paste-buffer, the system shall re-send Enter and re-check the pane on an increasing backoff until the paste is observed submitted, rather than sending Enter a fixed number of times.

**TMUX-25** When the pane is still positively observed as an unsubmitted paste after every backoff attempt, the system shall return a submission-not-confirmed error so the caller reports delivery failure instead of a false success.

**TMUX-27** When an AGY prompt wait is invoked, the system shall derive polling, retry, trust, survey, and ready-stabilization cancellation from the caller-supplied context, shall return cancellation before reporting readiness, and shall not install process-global signal handling inside the tmux helper.

**TMUX-28** When command-scoped multiline prompt delivery waits for a composer or rechecks composer stability, the system shall derive every wait and subprocess timeout from the caller context and shall return before prompt delivery when cancellation is observed during those waits.

**TMUX-29** When command-scoped file or slash-command delivery or prompt-delivery verification runs, the system shall derive composer waits, pane-capture subprocesses, verification backoff, and retry sends from the caller context and shall not write or retry prompt bytes after cancellation.

**TMUX-30** When command-scoped harness liveness validation runs, the system shall derive the tmux, process-table, and Codex Node-wrapper fallback scans from the caller context and return cancellation instead of allowing a later scan completion or attach.

**TMUX-31** When pane liveness is classified for command injection safety, the system shall positively identify a restartable shell only when exactly one pane exists, its process tree is observable, and every process in that tree is a plain interactive shell; any other foreground or descendant process shall fail that proof.

**TMUX-26** When a caller requests liveness for a named harness process, the system shall scan the full pane descendant tree for that exact process and shall return scan failures separately from a proven dead result.

**TMUX-31** When a transactional cleanup kills and verifies a tmux session, the system shall treat only an explicit missing-session response as absence and shall return socket, timeout, permission, and other backend failures instead of reporting cleanup success.

**TMUX-32** When Codex readiness is checked, the system shall require either the initial composer header with its model-change hint and an empty `›` input cursor or an empty post-turn `›` input cursor paired with a structured model footer, shall require that signal to own the current pane tail, and shall reject standalone model text in echoed launch commands, working-status footers, typed drafts, unsubmitted paste chips, or stale composers followed by newer process or shell output. A stale post-turn footer shall not suppress a newer tail-owned initial composer rendered after Codex restarts in the same pane.

**TMUX-33** When a transactional caller creates a tmux session, the system shall return a creation-specific identity composed of tmux's server-local session ID and a random token embedded in its provisional creation name before the token is stored on the session, shall preserve the printed server-local ID if a later command in the creation queue fails, and shall compare the ID with either ownership marker in strict kill and existence checks, so compensation can remove a partially initialized creation at every command boundary without selecting a replacement that reused either the same name or the same server-local ID after a server restart. If creation occurred but the client did not capture the server-local ID, cleanup shall target only the exact random provisional creation name.

**TMUX-34** When a transactional rename loses the tmux client's response, the system shall probe the exact old and new names with the caller-independent strict existence check, treat only old-absent/new-present as completed forward progress and old-present/new-absent as unchanged, and report both-present, both-absent, or probe failure as ambiguous instead of mutating storage from an assumed result. Compensation shall use the same postcondition so a lost rollback response is accepted only when the prior name is confirmed restored.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/tmux/workdir_test.go`
- Package tests: `agm/internal/tmux/liveness_test.go`
- Package tests: `agm/internal/tmux/tmux_test.go`
- Package tests: `agm/internal/tmux/linger_test.go`
- Package tests: `agm/internal/tmux/capture_test.go`
- Package tests: `agm/internal/tmux/agy_prompt_test.go`
