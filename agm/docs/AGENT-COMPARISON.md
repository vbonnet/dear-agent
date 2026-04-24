# Agent Comparison Guide

Choose the right AI agent for your task. AGM supports Claude (Anthropic), Gemini (Google), and GPT (OpenAI) with unified command interface.

## Quick Decision Guide

**Start here:**

1. **Processing >200K tokens of context?**
   - Yes → **Gemini** (1M token context window)
   - No → Continue to question 2

2. **Need complex multi-step reasoning or code generation?**
   - Yes → **Claude** (best-in-class reasoning)
   - No → Continue to question 3

3. **General chat, brainstorming, or quick Q&A?**
   - Yes → **GPT** (fast, general-purpose)

**Still unsure?** See detailed comparison below.

---

## Feature Comparison

| Feature | Claude (Anthropic) | Gemini (Google) | Codex (OpenAI) | OpenCode |
|---------|-------------------|-----------------|--------------|----------|
| **Architecture** | CLI (tmux) | CLI (tmux) | API | API |
| **Context Window** | 200K tokens | 1M tokens | 128K tokens | Varies |
| **Best For** | Code, reasoning | Research, large context | Code completion | Open models |
| **Strengths** | Multi-step reasoning | Massive context, multimodal | Fast code gen | Self-hosted |
| **Code Generation** | Excellent | Good | Excellent | Good |
| **Research/Summary** | Good | Excellent | Good | Good |
| **Reasoning Depth** | Excellent | Good | Good | Moderate |
| **Speed** | Moderate | Fast | Fast | Fast |
| **Limitations** | Shorter context | Requires more guidance | Context limits | Model-dependent |

---

## When to Use Claude

**Ideal for:**
- ✅ Writing code (functions, classes, refactoring)
- ✅ Multi-step problem solving (debugging, architecture decisions)
- ✅ Long document analysis (up to 200K tokens)
- ✅ Complex reasoning tasks

**Command:**
```bash
agm new --harness claude-code my-coding-session
```

**Example use cases:**
- Debugging a complex multi-file codebase
- Designing system architecture
- Code reviews with context from multiple files
- Refactoring legacy code

**Avoid for:**
- ❌ Processing documents >200K tokens (use Gemini)
- ❌ Simple quick questions (Codex/OpenCode may be faster)

---

## When to Use Gemini

**Ideal for:**
- ✅ Research and summarization (massive context)
- ✅ Processing large datasets (up to 1M tokens)
- ✅ Document analysis (books, research papers, logs)
- ✅ Tasks requiring maximum context window

**Command:**
```bash
agm new --harness gemini-cli research-task
```

**Example use cases:**
- Summarizing entire books or long research papers
- Analyzing large log files or datasets
- Research across many documents simultaneously
- Processing massive codebases

**Avoid for:**
- ❌ Complex multi-step reasoning (Claude excels here)
- ❌ Tasks requiring extensive planning without guidance

**Architecture Note:**
Gemini runs as a CLI agent (similar to Claude) using tmux session management. This provides full command execution support including directory changes, history management, and system prompts.

---

## When to Use Codex

**Ideal for:**
- ✅ Fast code completion and generation
- ✅ Code explanation and refactoring
- ✅ Quick coding tasks
- ✅ OpenAI ecosystem integration

**Command:**
```bash
agm new --harness codex-cli coding-session
```

**Example use cases:**
- Code completion and generation
- Quick refactoring tasks
- Code explanation
- Integration with OpenAI workflows

**Avoid for:**
- ❌ Very long context (>128K tokens)
- ❌ Large-scale research

---

## When to Use OpenCode

**Ideal for:**
- ✅ Self-hosted deployments
- ✅ Privacy-sensitive projects
- ✅ Custom model experimentation
- ✅ On-premise requirements

**Command:**
```bash
agm new --harness opencode-cli self-hosted-session
```

**Example use cases:**
- Private/sensitive codebases
- Custom-tuned models
- Air-gapped environments
- Cost optimization with self-hosted models

**Avoid for:**
- ❌ Tasks requiring state-of-the-art reasoning
- ❌ When cloud-based solutions are acceptable

---

## Command Translator Support

AGM provides unified commands that work across all agents using the `CommandTranslator` abstraction.

**Supported commands (all agents):**
- `CommandRename` - Rename agent session/conversation
- `CommandSetDir` - Set working directory context
- `CommandRunHook` - Execute initialization hook
- `CommandClearHistory` - Clear conversation history
- `CommandSetSystemPrompt` - Set/update system prompt
- `CommandAuthorize` - Authorize directory for agent access

**Agent-specific behavior:**
- **Claude (CLI):** Full command support via tmux slash commands
- **Gemini (CLI):** Full command support via tmux session management
- **Codex (API):** Core commands supported with API translations
- **OpenCode (API):** Core commands supported with API translations

**Command Translation Examples:**

| Command | Claude (CLI) | Gemini (CLI) | Codex/OpenCode (API) |
|---------|-------------|-------------|---------------------|
| `CommandSetDir` | `cd /path` via tmux | `cd /path` via tmux | Update session metadata |
| `CommandClearHistory` | Clear history file | Remove history.jsonl | Clear API conversation |
| `CommandSetSystemPrompt` | Send system instruction | Update session metadata | API system parameter |

For unsupported commands, AGM gracefully degrades (returns `ErrNotSupported`).

See [Command Translator documentation](https://github.com/vbonnet/dear-agent/tree/main/agm#-command-translation-multi-agent) for details.

---

## Cost & Availability

**Claude:**
- Provider: Anthropic
- Requires: Anthropic API key
- Pricing: Pay-per-token (see Anthropic pricing)

**Gemini:**
- Provider: Google Vertex AI
- Requires: Google Cloud account + API key
- Pricing: Pay-per-token (see Google Cloud pricing)

**Codex:**
- Provider: OpenAI
- Requires: OpenAI API key
- Pricing: Pay-per-token (see OpenAI pricing)

**OpenCode:**
- Provider: Self-hosted / Open-source
- Requires: Local model or API endpoint
- Pricing: Infrastructure costs only

**Setup:** See [AGM README](../README.md) for API key configuration.

---

## Switching Agents

You can use different agents for different sessions:

```bash
# Use Claude for code work
agm new --harness claude-code my-code-session

# Use Gemini for research
agm new --harness gemini-cli research-session

# Use Codex for quick coding
agm new --harness codex-cli coding-task

# Use OpenCode for self-hosted
agm new --harness opencode-cli private-project

# Resume any session (agent auto-detected)
agm resume my-code-session
```

Agent selection is stored in session manifest and persists across resume operations.

**All 4 agents are fully supported** with comprehensive BDD test coverage ensuring feature parity.

---

## Still Have Questions?

- **BDD scenarios:** See [BDD-CATALOG.md](BDD-CATALOG.md) for agent behavior examples
- **Troubleshooting:** See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for agent-specific issues
- **Migration:** See [MIGRATION-CLAUDE-MULTI.md](MIGRATION-CLAUDE-MULTI.md) to transition from single to multi-agent
