# AGM tmux Delivery Specification

<!-- Last audited at: 2026-07-21 -->

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

**TMUX-26** When a caller requests liveness for a named harness process, the system shall scan the full pane descendant tree for that exact process and shall return scan failures separately from a proven dead result.

**TMUX-27** When an AGY prompt wait is invoked, the system shall derive polling, retry, trust, survey, and ready-stabilization cancellation from the caller-supplied context, shall return cancellation before reporting readiness, and shall not install process-global signal handling inside the tmux helper.

**TMUX-28** When command-scoped multiline prompt delivery waits for a composer or rechecks composer stability, the system shall derive every wait and subprocess timeout from the caller context and shall return before prompt delivery when cancellation is observed during those waits.

**TMUX-29** When command-scoped file or slash-command delivery or prompt-delivery verification runs, the system shall derive composer waits, pane-capture subprocesses, verification backoff, and retry sends from the caller context and shall not write or retry prompt bytes after cancellation.

**TMUX-30** When command-scoped harness liveness validation runs, the system shall derive the tmux, process-table, and Codex Node-wrapper fallback scans from the caller context and return cancellation instead of allowing a later scan completion or attach.

**TMUX-31** When pane liveness is classified for command injection safety, the system shall positively identify a restartable shell only when exactly one pane exists, its process tree is observable, and every process in that tree is a plain interactive shell; any other foreground or descendant process shall fail that proof.

**TMUX-32** When pre-input AGY readiness encounters an active first-run color-theme or Terms of Service/Data Use screen, the system shall stop promptly with explicit interactive-onboarding guidance and shall not send keys that accept preferences, legal terms, or data-use choices on the operator's behalf; every create and resume entry point, including CLI and adapter paths, shall propagate this failure before prompt delivery or attachment. Detection shall tolerate pane line wrapping. Every resume entry point shall use a resume wait that requires the onboarding screen to persist across a bounded confirmation window so restored transcript content can settle before classification, while post-input transcript text and onboarding markers present only before the latest composer shall not be classified as an active onboarding screen.

**TMUX-33** When a transactional cleanup kills and verifies a tmux session, the system shall treat only an explicit missing-session response as absence and shall return socket, timeout, permission, and other backend failures instead of reporting cleanup success.

**TMUX-34** When Codex readiness is checked, the system shall require either the initial composer header with its model-change hint and an empty `›` input cursor or an empty post-turn `›` input cursor paired with a structured model footer, shall require that signal to own the current pane tail, and shall reject standalone model text in echoed launch commands, working-status footers, typed drafts, unsubmitted paste chips, or stale composers followed by newer process or shell output. A stale post-turn footer shall not suppress a newer tail-owned initial composer rendered after Codex restarts in the same pane.

**TMUX-35** When a transactional caller creates a tmux session, the system shall return a creation-specific identity composed of tmux's server-local session ID and a random token embedded in its provisional creation name before the token is stored on the session, shall preserve the printed server-local ID if a later command in the creation queue fails, and shall compare the ID with either ownership marker in strict kill and existence checks, so compensation can remove a partially initialized creation at every command boundary without selecting a replacement that reused either the same name or the same server-local ID after a server restart. If creation occurred but the client did not capture the server-local ID, cleanup shall target only the exact random provisional creation name.

**TMUX-36** When a transactional rename starts, the system shall claim the exact source session with its server-local ID and a random short-lived option marker, reconciling a lost claim response by that marker. After every forward or compensating rename response, it shall verify that the same ID still carries the marker at the expected name. A missing marker, unexpected name, failed inspection, or server-restart replacement that reused either name or ID shall be ambiguous and shall never authorize metadata mutation or replacement adoption. Marker cleanup shall be conditional on the same ID and random token so it cannot mutate a replacement.

**TMUX-37** When an Enter for prompt delivery is accepted but the following pane capture cannot determine whether submission occurred, the system shall preserve an explicit submission-uncertain outcome across every later retry so transactional callers preserve work that may have started. A paste positively observed in the composer after every retry shall remain a definite not-submitted failure only when no earlier accepted Enter had an indeterminate capture.

**TMUX-38** When AGM waits to send to Pi, the system shall require the latest managed `AGM <mode>/ready` status, reject permission or selection overlays, and not accept stale readiness that precedes a newer working status.

**TMUX-39** When AGM verifies Pi liveness, the system shall scan the live pane process tree with lossless operating-system argv for Pi-specific executable identity, including the canonical npm Node entrypoint when its absolute package path or a supported preload path contains whitespace, and shall not accept tmux existence, retained pane start-command metadata, a generic shell prompt, a generic `node` process, an option value, or an unrelated Node script carrying a later Pi-looking argument as proof.

**TMUX-40** When AGM waits for a newly launched Pi process, the system shall accept only a managed ready marker containing that launch's unique ID, reject stale ready markers from earlier pane history, and fail closed when Pi-specific process liveness cannot be proved.

**TMUX-41** When a command-scoped Pi identity or pane-liveness scan runs, the system shall derive tmux and process-table subprocesses from the caller context so cancellation returns before command delivery, attachment, or metadata mutation.

**TMUX-42** When shared input readiness is checked for delivery, the system shall hold one tmux mutation boundary while resolving an exact active pane, proving process liveness and styled composer ownership, and delivering to that same pane ID; every harness including AGY and Pi shall require its structural idle composer to own that pane tail, Pi shall require its latest managed state to be ready, Claude dim or grey placeholder text shall count as empty while unstyled human draft text shall not, and a newer composer shall supersede resolved permission, onboarding, model-upgrade, or survey UI while active structured blockers still win; an explicit delivery policy may accept `QUEUE` only after exact foreground-harness and pane ownership are proved, and shall never accept permission, overlay, onboarding, wrong-harness, missing-target, or backend-error states, so a concurrent AGM sender, another pane's harness, generic glyphs or borders, stale prompts followed by work, unrelated Node processes, ordinary permission words, resolved blockers, or stale Pi readiness followed by newer work shall not suppress or fabricate readiness or redirect delivery.

**TMUX-43** When shared startup readiness waits, the system shall honor caller cancellation and the total deadline while the launch shell or wrapper remains visible before the expected harness is first observed, fail promptly if an observed harness later stops or an observation fails, and mutate input only for documented trust, model-upgrade, or AGY-survey transitions on the exact verified pane; Gemini first-run directory trust shall select option `1` and then submit Enter before composer readiness can succeed.

**TMUX-44** When expected harness liveness is checked for shared input readiness, the system shall identify supported Node-hosted harnesses from the harness-specific Node entry-script argument even when the operating system reports a non-`node` command name such as `MainThread`, including the canonical npm-hosted Pi entrypoint, while rejecting unrelated Node entry scripts, later ordinary arguments, runtime-option values, and shell wrappers that merely mention a harness.

**TMUX-45** When expected harness liveness is checked for input delivery, the system shall require a matching harness process to belong to the terminal's current foreground process group and not be stopped; a suspended or background harness descendant and a stale composer rendered above the foreground shell shall be classified as `WRONG_HARNESS` and shall never authorize input.

**TMUX-46** When shared input readiness or pane liveness checks verify tmux session existence, the system shall classify only an explicit missing-target response as absence and shall return inaccessible-socket, unavailable-server, timeout, permission, and other backend failures instead of reporting `NOT_FOUND` or a dead session.

**TMUX-47** When a multiline prompt is delivered to AGY, including through the atomic exact-pane delivery boundary, the system shall ask tmux to preserve line feeds and emit bracketed-paste delimiters when the application requested them, then send one Enter after the complete paste; other harnesses shall retain the established paste behavior.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/tmux/workdir_test.go`
- Package tests: `agm/internal/tmux/liveness_test.go`
- Package tests: `agm/internal/tmux/enter_reliable_test.go`
- Package tests: `agm/internal/tmux/tmux_test.go`
- Package tests: `agm/internal/tmux/linger_test.go`
- Package tests: `agm/internal/tmux/capture_test.go`
- Package tests: `agm/internal/tmux/agy_prompt_test.go`
- Package tests: `agm/internal/tmux/prompt_test.go`
- Integration tests: `agm/internal/tmux/agy_lifecycle_integration_test.go`
- Package tests: `agm/internal/tmux/pi_prompt_test.go`
- Package tests: `agm/internal/tmux/readiness_test.go`
- Integration tests: `agm/internal/session/tmux_real_readiness_test.go`
