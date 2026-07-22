# Gemini compatibility documentation

This directory contains only the strict compatibility specification and a
short architecture map for the deprecated `gemini-cli` harness.

- [`ARCHITECTURE.md`](ARCHITECTURE.md) explains the live CLI/tmux path and
  lifecycle boundary.
- [`SPEC.md`](SPEC.md) states the executable compatibility requirements.
- The implementation is [`../gemini_cli_adapter.go`](../gemini_cli_adapter.go).

The former Gemini API/SDK design and its accepted ADRs were retired because no
such implementation exists in the current tree. Git history preserves them if
historical investigation is required.
