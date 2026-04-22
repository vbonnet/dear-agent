# BDD Scenario Catalog

Behavior-Driven Development (BDD) test scenarios for AGM. These scenarios document and validate AGM's multi-agent capabilities.

---

## Overview

AGM uses BDD tests to ensure consistent behavior across all supported agents (Claude, Gemini, GPT). Scenarios are written in Gherkin format and executed with `godog`.

**Location:** [`test/bdd/features/`](../test/bdd/features/)

**How to run:** See [test/bdd/README.md](../test/bdd/README.md)

---

## Find Scenarios by Use Case

### Session Management
- [Session Lifecycle](#session-lifecycle) - Creating, pausing, resuming sessions
- [Conversation Persistence](#conversation-persistence) - Message history across resumes

### Agent Operations
- [Agent Selection](#agent-selection) - Choosing and persisting agent selection
- [Agent Interface](#agent-interface) - Uniform adapter interface contract
- [Agent Registry](#agent-registry) - Agent registration and discovery
- [Agent Capabilities](#agent-capabilities) - Feature support per agent

### Quality & Reliability
- [Error Handling](#error-handling) - Graceful degradation and error scenarios
- [UX Consistency](#ux-consistency) - Consistent user experience across agents

---

## Feature Files

### Agent Selection

**File:** [`agent_selection.feature`](../test/bdd/features/agent_selection.feature)

**Purpose:** Validates multi-agent selection and persistence.

**Key Scenarios:**
- **Use different agents for different tasks**
  - Create Claude session for code work
  - Create Gemini session for research
  - Verify each session uses correct agent

- **Agent selection persists across resume**
  - Create session with specific agent (Claude/Gemini/GPT)
  - Pause and resume session
  - Verify agent persists correctly

- **Agent persists across full lifecycle**
  - Create, pause, resume, send message
  - Verify agent used throughout lifecycle
  - Test all agents (Claude, Gemini, GPT)

**Why this matters:**
Ensures users can reliably choose different agents for different tasks without agent confusion or switching.

---

### Conversation Persistence

**File:** [`conversation_persistence.feature`](../test/bdd/features/conversation_persistence.feature)

**Purpose:** Validates conversation history persists across session resumes.

**Key Scenarios:**
- **Messages persist after pause/resume**
  - Send messages to session
  - Pause and resume session
  - Verify message history restored

- **Multi-turn conversations work across agents**
  - Test conversation flow for Claude, Gemini, GPT
  - Ensure all agents handle message history correctly

**Why this matters:**
Users expect conversation continuity. Losing message history breaks user workflow and trust.

---

### Session Lifecycle

**File:** [`session_lifecycle.feature`](../test/bdd/features/session_lifecycle.feature)

**Purpose:** Validates session creation, pause, resume, archiving.

**Key Scenarios:**
- **Create new session**
  - Verify session created with correct metadata
  - Validate manifest format

- **Pause and resume session**
  - Session state preserved across pause/resume
  - Context restored correctly

- **Archive and restore session**
  - Archive inactive sessions
  - Restore archived sessions
  - Verify data integrity

**Why this matters:**
Core AGM workflow. Users must reliably manage session lifecycle without data loss.

---

### Agent Interface

**File:** [`agent_interface.feature`](../test/bdd/features/agent_interface.feature)

**Purpose:** Validates uniform adapter interface across all agents.

**Key Scenarios:**
- **All agents implement required interface methods**
  - `CreateSession`
  - `SendMessage`
  - `GetHistory`
  - Other adapter contract methods

- **Interface methods behave consistently**
  - Same input produces expected output
  - Error handling consistent across agents

**Why this matters:**
AGM's multi-agent support depends on uniform adapter interface. Inconsistent interfaces cause bugs and confuse users.

---

### Agent Registry

**File:** [`agent_registry.feature`](../test/bdd/features/agent_registry.feature)

**Purpose:** Validates agent registration and discovery.

**Key Scenarios:**
- **Register agents dynamically**
  - Add Claude, Gemini, GPT adapters to registry
  - Verify agents discoverable

- **Lookup agents by name**
  - Request agent by name (`claude`, `gemini`, `gpt`)
  - Verify correct adapter returned

- **Handle unknown agents gracefully**
  - Request non-existent agent
  - Verify graceful error (not crash)

**Why this matters:**
Dynamic agent registry enables extensibility. New agents can be added without core changes.

---

### Agent Capabilities

**File:** [`agent_capabilities.feature`](../test/bdd/features/agent_capabilities.feature)

**Purpose:** Validates agent-specific capabilities and limitations.

**Key Scenarios:**
- **Command Translator support levels**
  - Test `RenameSession` command per agent
  - Test `SetDirectory` command per agent
  - Test `RunHook` command per agent
  - Verify graceful degradation for unsupported commands

- **Context window limits**
  - Claude: 200K tokens
  - Gemini: 1M tokens
  - GPT: 128K tokens

- **Feature availability**
  - Some features agent-specific (not all agents support all features)

**Why this matters:**
Users need to understand what each agent can and cannot do. Capability awareness prevents user frustration.

---

### Error Handling

**File:** [`error_handling.feature`](../test/bdd/features/error_handling.feature)

**Purpose:** Validates graceful error handling across failure scenarios.

**Key Scenarios:**
- **Session not found**
  - Resume non-existent session
  - Verify clear error message (not crash)

- **Invalid harness name**
  - Create session with typo in agent name
  - Verify helpful error message

- **API key missing**
  - Attempt agent operation without API key
  - Verify graceful degradation

- **Network failures**
  - Simulate network timeout
  - Verify retry logic or clear error

**Why this matters:**
Errors are inevitable. Users judge software by error handling quality. Good errors help users self-recover.

---

### UX Consistency

**File:** [`ux_consistency.feature`](../test/bdd/features/ux_consistency.feature)

**Purpose:** Validates consistent user experience across all agents.

**Key Scenarios:**
- **Commands work identically across agents**
  - `agm session new` behaves same for Claude/Gemini/GPT
  - `agm session resume` works uniformly
  - `agm session list` output format consistent

- **Error messages consistent**
  - Same error produces same message format
  - Agent name included in messages for clarity

- **Session metadata consistent**
  - Manifest format same across agents
  - Metadata fields uniform

**Why this matters:**
Users shouldn't need to remember agent-specific commands or workflows. Consistency reduces cognitive load.

---

## Running BDD Tests

### Run all scenarios

```bash
cd main/agm
make test-bdd
```

### Run specific feature

```bash
cd test/bdd
go test -v -godog.tags=@agent-selection
```

### Run specific scenario

```bash
cd test/bdd
go test -v -godog.tags="@claude"
```

### Generate test report

```bash
cd test/bdd
go test -v -godog.format=junit -godog.output=junit.xml
```

---

## Adding New Scenarios

**Want to add a new scenario?** See [test/bdd/README.md](../test/bdd/README.md#writing-new-scenarios) for step-by-step guide.

**Steps:**
1. Add Gherkin scenario to appropriate `.feature` file
2. Implement step definitions in `steps/` directory
3. Register steps in `main_test.go`
4. Run tests to verify

---

## Scenario Coverage

| Category | Feature File | Scenarios | Status |
|----------|-------------|-----------|--------|
| Agent Selection | agent_selection.feature | 3 | ✅ Passing |
| Conversation | conversation_persistence.feature | 2+ | ✅ Passing |
| Lifecycle | session_lifecycle.feature | 3+ | ✅ Passing |
| Interface | agent_interface.feature | 2+ | ✅ Passing |
| Registry | agent_registry.feature | 3 | ✅ Passing |
| Capabilities | agent_capabilities.feature | 3+ | ✅ Passing |
| Errors | error_handling.feature | 4+ | ✅ Passing |
| UX | ux_consistency.feature | 3+ | ✅ Passing |

**Total:** 20+ scenarios across 8 feature files

---

## Using Scenarios as Documentation

**BDD scenarios are living documentation.** They:
- Define expected behavior in plain English (Gherkin)
- Validate implementation matches specification
- Serve as examples for users and developers
- Auto-update when behavior changes (tests fail if docs drift)

**Example: Understanding agent selection**

Instead of reading prose documentation, read the scenario:

```gherkin
Scenario: Use different agents for different tasks
  Given I have AGM installed
  When I run "agm session new --harness=claude-code code-session"
  And I run "agm session new --harness=gemini-cli research-session"
  Then session "code-session" should have agent "claude"
  And session "research-session" should have agent "gemini"
```

This is executable documentation. The test ensures the example actually works.

---

## Next Steps

- **Run scenarios:** Follow instructions in [test/bdd/README.md](../test/bdd/README.md)
- **Choose agent:** See [AGENT-COMPARISON.md](AGENT-COMPARISON.md)
- **Troubleshoot:** See [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- **Migrate:** See [MIGRATION-CLAUDE-MULTI.md](MIGRATION-CLAUDE-MULTI.md)
