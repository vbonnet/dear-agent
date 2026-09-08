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

**TMUX-34** When Codex readiness is checked, the system shall require either the initial composer header with its model-change hint and an empty `›` or `»` input cursor or an empty post-turn `›` or `»` input cursor paired with a structured model footer, shall require that signal to own the current pane tail, and shall reject standalone model text in echoed launch commands, working-status footers, typed drafts, unsubmitted paste chips, or stale composers followed by newer process or shell output. A stale post-turn footer shall not suppress a newer tail-owned initial composer rendered after Codex restarts in the same pane.

**TMUX-35** When a transactional caller creates a tmux session, the system shall return a creation-specific identity composed of tmux's server-local session ID and a random token embedded in its provisional creation name before the token is stored on the session, shall preserve the printed server-local ID if a later command in the creation queue fails, and shall compare the ID with either ownership marker in strict kill and existence checks, so compensation can remove a partially initialized creation at every command boundary without selecting a replacement that reused either the same name or the same server-local ID after a server restart. If creation occurred but the client did not capture the server-local ID, cleanup shall target only the exact random provisional creation name.

**TMUX-36** When a transactional rename starts, the system shall claim the exact source session with its server-local ID and a random short-lived option marker, reconciling a lost claim response by that marker. After every forward or compensating rename response, it shall verify that the same ID still carries the marker at the expected name. A missing marker, unexpected name, failed inspection, or server-restart replacement that reused either name or ID shall be ambiguous and shall never authorize metadata mutation or replacement adoption. Marker cleanup shall be conditional on the same ID and random token so it cannot mutate a replacement.

**TMUX-37** When an Enter for prompt delivery is accepted but the following pane capture cannot determine whether submission occurred, the system shall preserve an explicit submission-uncertain outcome across every later retry so transactional callers preserve work that may have started. A paste positively observed in the composer after every retry shall remain a definite not-submitted failure only when no earlier accepted Enter had an indeterminate capture.

**TMUX-38** When AGM waits to send to Pi, the system shall require the latest managed `AGM <mode>/ready` status, reject permission or selection overlays, and not accept stale readiness that precedes a newer working status.

**TMUX-39** When AGM verifies Pi liveness, the system shall scan the live pane process tree with lossless operating-system argv for Pi-specific executable identity, including the canonical npm Node entrypoint when its absolute package path or a supported preload path contains whitespace, and shall not accept tmux existence, retained pane start-command metadata, a generic shell prompt, a generic `node` process, an option value, or an unrelated Node script carrying a later Pi-looking argument as proof.

**TMUX-40** When AGM waits for a newly launched Pi process, the system shall accept only a managed ready marker containing that launch's unique ID, reject stale ready markers from earlier pane history, and fail closed when Pi-specific process liveness cannot be proved.

**TMUX-41** When a command-scoped Pi identity or pane-liveness scan runs, the system shall derive tmux and process-table subprocesses from the caller context so cancellation returns before command delivery, attachment, or metadata mutation.

**TMUX-42** When shared input readiness is checked for delivery, the system shall hold one tmux mutation boundary while resolving an exact active pane, proving process liveness and styled composer ownership, and delivering to that same pane ID; queue identity shall inspect the pane's complete style-preserving logical capture with terminal-wrapped rows joined, while every harness including AGY and Pi shall require its structural idle composer to own the live pane tail, Pi shall require its latest managed state to be ready, Claude dim or grey placeholder text shall count as empty while unstyled human draft text shall not, and a newer composer shall supersede resolved permission, onboarding, model-upgrade, or survey UI while active structured blockers still win; for every supported harness, an explicit delivery policy may replace input only when the latest queued-input marker and a syntactically complete generated AGM message header are bound inside the same current composer after exact foreground-harness and pane ownership are proved, visible pasted-text line or character counts shall bind the complete payload including payload-ending whitespace while excluding only capture framing and later output, prompt-like glyphs inside that measured payload shall remain payload rather than replace its structural composer anchor, and a required managed or idle footer shall be terminal composer chrome rather than stale history. An opaque Codex pasted-content chip without an observable bound header and a post-turn Codex queue whose occupied cursor cannot independently prove idle ownership shall remain protected generic input; only the compact first-turn Codex composer with no intervening work and an exact native pasted-character extent may own a recoverable Codex queue. Before pasting a replacement, the system shall clear the verified queued input without submitting it, then re-prove the expected foreground harness and empty composer on that same exact pane; a failed clear, failed recheck, changed pane, or non-empty composer shall abort without replacement. It shall never accept a historical header, partial header, human draft, generic busy composer, active work, permission, overlay, onboarding, wrong-harness, missing target, or backend-error state, so a concurrent AGM sender, another pane's harness, generic glyphs or borders, stale prompts followed by work, unrelated Node processes, ordinary permission words, resolved blockers, or stale Pi readiness followed by newer work shall not suppress or fabricate readiness or redirect delivery.

**TMUX-43** When shared startup readiness waits, the system shall honor caller cancellation and the total deadline while the launch shell or wrapper remains visible before the expected harness is first observed, fail promptly if an observed harness later stops or an observation fails, and mutate input only for documented trust, Codex model-upgrade, Codex update-selector Skip, or AGY-survey transitions on the exact verified pane; Codex update handling shall select Skip with Down then Enter rather than accepting a package upgrade, and Gemini first-run directory trust shall select option `1` and then submit Enter before composer readiness can succeed.

**TMUX-44** When expected harness liveness is checked for shared input readiness, the system shall identify supported Node-hosted harnesses from the harness-specific Node entry-script argument even when the operating system reports a non-`node` command name such as `MainThread`, including the canonical npm-hosted Pi entrypoint, while rejecting unrelated Node entry scripts, later ordinary arguments, runtime-option values, and shell wrappers that merely mention a harness.

**TMUX-45** When expected harness liveness is checked for input delivery, the system shall require a matching harness process to belong to the terminal's current foreground process group and not be stopped; a suspended or background harness descendant and a stale composer rendered above the foreground shell shall be classified as `WRONG_HARNESS` and shall never authorize input.

**TMUX-46** When shared input readiness or pane liveness checks verify tmux session existence, the system shall classify only an explicit missing-target response as absence and shall return inaccessible-socket, unavailable-server, timeout, permission, and other backend failures instead of reporting `NOT_FOUND` or a dead session.

**TMUX-47** When a multiline prompt is delivered to AGY, including through the atomic exact-pane delivery boundary, the system shall ask tmux to preserve line feeds and emit bracketed-paste delimiters when the application requested them, then send one Enter after the complete paste; other harnesses shall retain the established paste behavior.

**TMUX-48** When Codex startup shows either the structured selector for new or changed executable hooks or the active hooks dashboard with review-required hooks, every direct and shared readiness wait for an expected Codex harness shall inspect the complete captured screen, stop promptly with a typed, actionable error that requires interactive operator review, classify the pane as review-required rather than ready or generically busy, and not send keys, trust hooks, register the session, deliver a startup prompt, attach, or update lifecycle metadata; trailing blank terminal rows and terminal styling within structural controls shall not hide an active review surface, an active dashboard redraw below an older composer shall remain authoritative, retained selector or dashboard text followed by a newer empty or occupied composer or an in-flight turn shall not remain a blocker, and copied Codex dashboard text in another expected harness's scrollback shall not impose a Codex-specific blocker.

**TMUX-49** When Codex renders its current welcome view with instructions between the composer header and cursor, the system shall use a style-preserving logical capture that rejoins terminal-wrapped rows and classify the structured terminal footer paired with suggestion text styled dim or grey from its first visible character as an empty composer while treating identical unstyled text, structurally occupied paste chips, and human drafts with only later styled tokens as occupied input; direct startup and generic prompt waits, idle probes, shared readiness, session and backend state detection, and prompt delivery shall use the same distinction even in narrow panes.

**TMUX-50** When atomic initial delivery of a Codex prompt with no extractable verification keyword is followed by a styled idle composer at the first delayed verification capture, the verifier shall treat the outcome as an ambiguous successful return and shall not resend the prompt, because the turn may already have completed; observable processing, prompt keywords, and prompt disappearance shall remain stronger confirmation signals, while an extractable keyword that remains absent shall retain retry behavior.

**TMUX-51** When a readiness or pane-liveness scan invokes multiple sequential tmux or process-table observations, the system shall bound the complete scan with an internal deadline that accommodates the full observation sequence under loaded-host contention while an earlier caller cancellation or deadline remains authoritative; exhausting either deadline or failing any observation shall return an operational error and shall never fabricate absence, wrong-harness state, or readiness.

**TMUX-52** When exact-harness readiness classifies a non-ready pane, the system shall report `PROCESSING` only for a current-tail native Codex, Claude, or managed Pi active-work signal; an occupied human composer, queued AGM paste, historical or arbitrary working text, unsupported harness output, and unrecognized content shall remain distinct non-processing states that cannot prove a compaction transition.

**TMUX-53** When atomic exact-pane delivery waits for the tmux mutation lock, the system shall honor caller cancellation; after the first ready capture and immediately before submission it shall re-prove the same pane and foreground harness PID, abort without sending when either changed, and, when strict submission confirmation is requested, preserve that exact target plus an explicit may-have-started outcome when the irreversible submission acknowledgement or every post-Enter observation is lost.

**TMUX-54** When atomic delivery is asked to preserve multiline composer input as one submission, the system shall invoke tmux paste-buffer with raw bracketed-paste semantics so embedded line feeds remain paste content until the separately verified Enter boundary.

**TMUX-55** When strict submission confirmation succeeds, the system shall retain the tmux mutation lock while proving from the exact pane's complete logical history that the complete delivery-owned command left its parked shape after an accepted Enter, then re-proving the same pane ID, pane-root PID, tmux session ID, stable AGM binding, foreground harness PID and process birth time, and expected harness. Mere absence, an empty composer, alternate-screen disappearance, exact-command prefix followed by arbitrary output, or generic busy is not positive submission continuity because a concurrent human clear or appended multiline draft can produce those shapes. Only live-ready or native-processing evidence on that unchanged runtime confirms continuity. A partial or different occupied composer, truncated or ambiguous capture, missing or changed identity, queued composer, permission or overlay state, generic busy state, or failed post-submit observation shall return marked may-have-started uncertainty with the original exact receipt, while callers that do not request strict confirmation shall retain legacy delivery without an added post-submit observation.

**TMUX-56** When AGM creates or explicitly adopts a tmux session for a registered stable session, the system shall persist that stable ID as a session-local tmux binding without overwriting a different binding; adoption shall condition one queued claim on the observed session name and ID, pane ID and root PID, and empty binding, and shall write a random adoption identity with the stable ID so a lost acknowledgement can be reconciled only to that exact claim. Exact delivery shall require the expected binding on the resolved tmux session at initial readiness, immediately before submission, and during strict post-submit reproof, treating a missing or changed pre-submit binding as definite non-delivery and a missing or changed post-submit binding as marked uncertainty.

**TMUX-57** When raw multiline delivery depends on bracketed-paste framing, the system shall include `bracket_paste_flag=1` in the same tmux-server conditional command that verifies exact target identity and pastes the uniquely named buffer; a disabled or changed flag shall stop before prompt bytes or Enter and remain definite not sent.

**TMUX-58** When strict delivery is canceled before exact paste mutation it shall remain definite non-delivery. Once the exact paste succeeds, cancellation before Enter or during capture backoff shall preserve marked uncertainty because the parked command can later be submitted; after an accepted Enter, a lost or ambiguous observation shall stop without another Enter, and a retry shall occur only when the complete exact command is positively still parked.

**TMUX-59** When exact delivery has already crossed the irreversible submission boundary but releasing its tmux mutation lock fails, the system shall preserve the exact delivery receipt, mark `MayHaveStarted`, and return uncertainty instead of downgrading the outcome to safe non-delivery or success.

**TMUX-60** When strict post-submit reproof positively observes native processing on the exact stable-bound pane and foreground harness PID, the system shall return that observation separately from submission acknowledgement so a completion verifier can preserve the transition without inferring it from a later idle frame.

**TMUX-61** When strict delivery is attempted, the system shall use one random tmux buffer per attempt, condition both paste-buffer and every Enter retry inside tmux's command queue on the exact pane ID, pane-root PID, tmux session ID, stable AGM session binding, and zero attached tmux clients, and require a structural exact-command occurrence created after the complete pre-paste history baseline before accepting submission proof. A strict compaction request shall refuse with an actionable detach instruction whenever a client is attached at either mutation boundary. A prior identical command echo, pane replacement, server restart, attached-client input, or identity drift between observation and mutation shall not redirect or falsely confirm prompt bytes or Enter; a lost reply after a conditional mutation starts shall remain uncertain and prohibit automatic retry.

### Open terminal-input boundary

TMUX-61 is defense in depth, not an exclusive composer or foreground-harness
lease. Tmux exposes neither a terminal-content fingerprint nor the foreground
child PID and birth marker in the queued format condition: an external writer
can issue `send-keys`, or attach, type, and detach, after AGM's empty-composer
recheck but before the conditional paste, and the foreground harness can also
exit or restart in that interval. The conditioned pane-root and session
identities can remain unchanged while AGM appends to foreign input or targets a
different foreground process. Closing this boundary requires a harness-native
input transaction/lease or an equivalent mutation-time content and process
authority; callers and reviewers must not represent the current terminal
transport as proving composer or foreground-harness immutability across that
interval. On macOS, `ps lstart` is also only second-resolution, so the recorded
birth marker reduces ordinary PID-reuse risk but cannot distinguish the
pathological case of the same PID being recycled within the same second.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/tmux/workdir_test.go`
- Package tests: `agm/internal/tmux/liveness_test.go`
- Package tests: `agm/internal/tmux/enter_reliable_test.go`
- Package tests: `agm/internal/tmux/tmux_test.go`
- Package tests: `agm/internal/tmux/linger_test.go`
- Package tests: `agm/internal/tmux/capture_test.go`
- Package tests: `agm/internal/tmux/agy_prompt_test.go`
- Package tests: `agm/internal/tmux/codex_prompt_test.go`
- Package tests: `agm/internal/tmux/prompt_test.go`
- Package tests: `agm/internal/tmux/verify_delivery_test.go`
- Integration tests: `agm/internal/tmux/agy_lifecycle_integration_test.go`
- Package tests: `agm/internal/tmux/pi_prompt_test.go`
- Package tests: `agm/internal/tmux/readiness_test.go`
- Integration tests: `agm/internal/session/tmux_real_readiness_test.go`
- Related feature: `agm/test/bdd/features/agm_runtime_package_guardrails.feature` (TMUX-52 through TMUX-61)
