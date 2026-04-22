# ADR-0003: InitSequence Prompt Verification Flag

## Status
Accepted

## Context
The `InitSequence` was calling `WaitForClaudePrompt()` redundantly. The caller (`new.go:834`) already verified the prompt was ready before starting the init sequence. However, `sendRename()` and `sendAssociation()` both called `WaitForClaudePrompt()` again with 30s timeouts.

By the time these methods executed (after manifest creation, Dolt registration, git commit), the prompt had often scrolled off the 50-line capture buffer, causing 30s timeouts and preventing `/rename` from being sent.

This regression was exacerbated by the sandbox-by-default feature, which added latency between prompt detection and init sequence execution.

## Decision
Add a `PromptVerified bool` field to the `InitSequence` struct. When set to `true` by the caller, `sendRename()` and `sendAssociation()` skip their `WaitForClaudePrompt()` calls.

## Consequences
**Positive**:
- Eliminates redundant 30s timeouts
- `/rename` is sent reliably even when prompt scrolls off buffer
- Backward compatible (default `false` preserves old behavior)

**Negative**:
- Callers must ensure prompt is actually verified before setting flag
- Adds slight complexity to the API

**Mitigations**:
- Tests verify both `PromptVerified=true` and `false` paths
- Documentation explains when to use the flag

## Update (2026-03-26): Trust Dialog Root Cause

The `PromptVerified` flag was the initial fix for a secondary symptom. The actual root cause was the trust dialog: new sandbox UUIDs trigger Claude Code's trust prompt ("Do you trust the files in this folder?"), and `WaitForClaudePrompt` false-positives on the prompt character in the dialog's option list.

The real fix (also in `init_sequence.go`) detects trust dialogs via `capture-pane` and sends `Enter` to confirm before sending `/rename`. This prevents the trust dialog from consuming `/rename` keystrokes.

`PromptVerified` remains as an optimization to avoid costly timeouts when the caller has already confirmed the prompt is ready.
