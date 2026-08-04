# Merge Loop Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/mergeloop` adapts GitHub, safe-rebase, safe-merge, review-thread, and AGM
session operations to the persistent merge-loop policy engine.

## EARS Requirements

**MLC-01** When `tick` mode is selected, the command shall perform one idempotent policy pass; when `run` mode is selected, it shall repeat until cancellation.

**MLC-02** When pull-request checks are normalized, the command shall consume the shared safegit effective required-check projection and shall exclude advisory rollup history from repair classification.

**MLC-03** When a pull request is behind, the command shall prefer `safe-rebase` and use GitHub update-branch only as a compatibility fallback.

**MLC-04** When a green pull request is merged, the command shall delegate irreversible execution to the shared safe-merge library.

**MLC-05** When known bot review threads remain unresolved, the command shall resolve only bot-authored threads and shall never resolve human-authored threads. A thread counts as bot-authored only when every comment in it — not merely the first — is from a known bot; a human reply anywhere in the thread, even after a bot's opening comment, disqualifies it from auto-resolution.

**MLC-06** When agent spawning is disabled or its selected harness lacks usable credentials, the command shall defer and audit the repair rather than blocking later pull requests.

**MLC-07** When a repair agent is enabled, the command shall pass the selected harness and optional model to `agm session new` without a Claude-specific credential precondition.

**MLC-08** When `--agent-model` names Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, or Qwen models, the command shall preserve that identifier for AGM's shared model validation.

**MLC-09** When dry-run mode is enabled, the command shall classify and report without rebasing, merging, resolving threads, or spawning sessions.

**MLC-10** When a child command exceeds its operation timeout, the command shall terminate it and return a contextual timeout error.

**MLC-11** When safegit returns a normalized required-check status, the command shall map each known status one-to-one into the merge-loop verdict model and shall reject an unknown status.

**MLC-12** When one pull request's effective required-check projection cannot be produced, the command shall mark only that pull request as pending and shall continue normalizing later independent pull requests.

**MLC-13** When the open pull-request count exceeds the configured cap, the command shall return metadata for backpressure detection without running any per-pull-request required-check projection.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`
- Package tests: `cmd/mergeloop/*_test.go`
