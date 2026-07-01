# MCP Harness Parity Specification

<!-- Last audited at: NEEDS-AUDIT -->

**Version:** 1.0
**Status:** Baseline
**Scope:** MCP tool discovery, session creation, message delivery, and model validation parity across AGM harnesses.

## Overview

MCP parity means an MCP client can discover and drive the same core AGM
session lifecycle operations that CLI users rely on. The MCP surface must not
special-case Claude Code as the only valid harness. Claude Code remains the
reference behavior, while Codex CLI, AGY, and OpenCode must be accepted by the
same registry and model-validation path. Gemini CLI remains accepted only as
deprecated compatibility.

## EARS Requirements

**MCP-01** When an MCP client creates an AGM session, the system shall validate the requested harness with the shared agent harness registry.

**MCP-02** When an MCP client omits the harness, the system shall default the session to `claude-code`.

**MCP-03** When an MCP client omits the model, the system shall use the selected harness default when one exists.

**MCP-04** When an MCP client creates an OpenCode session without an explicit model, the system shall use a safe non-interactive fallback model.

**MCP-05** When an MCP client supplies a model identifier, the system shall validate it with the shared model registry and safe passthrough character policy.

**MCP-06** When an MCP client supplies an OpenRouter-style model identifier containing `/` or `:`, the system shall accept it if it passes the shared model validator.

**MCP-07** When an active harness is present in the agent registry, the system shall provide a concrete MCP session startup command for that harness.

**MCP-08** When an MCP client requests operation discovery, the system shall list `create_session` and `send_message` as MCP mutation operations.

**MCP-09** When an MCP client lists tool schemas, the system shall document all active harnesses and deprecated Gemini compatibility rather than only Claude Code.

**MCP-10** When an active harness or supported model family is added, the system shall require MCP parity tests before the harness or family is considered supported.
