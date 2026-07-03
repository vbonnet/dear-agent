# AGM tmux Delivery Specification

<!-- Last audited at: 2026-07-03 -->

## Purpose

`agm/internal/tmux` owns low-level tmux delivery primitives for AGM-managed
sessions. The package normalizes session names, waits for harness readiness,
serializes send-keys operations, stashes potentially human-authored input, and
keeps prompt delivery behavior consistent across supported CLI harnesses.

## EARS Requirements

**TMUX-01** When a safe send function is called, the system shall wait for a recognized harness prompt before delivering input.

**TMUX-02** When a prompt file is sent, the system shall reject missing files and files larger than the configured prompt-size limit before touching tmux.

**TMUX-03** When multiline prompt delivery observes that the composer disappeared after readiness detection, the system shall abort delivery unless the caller explicitly bypassed the post-submit guard.

**TMUX-04** When human input is detected before prompt delivery, the system shall stash the existing input non-blockingly before sending automation.

**TMUX-05** When sending keys to tmux, the system shall normalize the target session and serialize send-keys operations to prevent cross-session byte interleaving.
