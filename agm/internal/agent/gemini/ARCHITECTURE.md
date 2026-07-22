# Deprecated Gemini CLI compatibility

<!-- Last audited at: 2026-07-17 -->

`gemini-cli` is a deprecated AGM harness retained for compatibility with
existing manifests and explicit selections. It is not part of AGM's active
cross-harness parity set.

There is no Gemini API or Google SDK adapter in this directory. The executable
implementation is the sibling file
[`../gemini_cli_adapter.go`](../gemini_cli_adapter.go) in package `agent`.

## Runtime path

```text
agent.GetHarness("gemini-cli")
    -> NewGeminiCLIAdapter
    -> shared JSON session store
    -> tmux session
    -> local `gemini` CLI
```

The adapter:

- launches the `gemini` executable in tmux with authorized directories;
- records the AGM-to-tmux and native Gemini session mapping in the shared JSON
  session store;
- resumes through `gemini --resume`;
- sends messages through tmux;
- reads or exports history only through the compatibility methods implemented
  in `gemini_cli_adapter.go`.

## Lifecycle boundary

The canonical lifecycle sets are in [`../harnesses.go`](../harnesses.go):

- active: `claude-code`, `codex-cli`, `agy`, `opencode-cli`;
- deprecated: `gemini-cli`.

New defaults, parity requirements, and feature work must target active
harnesses. Changes to this adapter should be limited to compatibility,
correctness, security, or safe removal work unless the harness is deliberately
re-activated through a separate governance decision.

## Verification

Adapter tests live beside `gemini_cli_adapter.go`. The lifecycle distinction is
also checked by the harness registry tests and the subsystem documentation-truth
test.
