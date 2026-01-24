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

| Feature | Claude (Anthropic) | Gemini (Google) | GPT (OpenAI) |
|---------|-------------------|-----------------|--------------|
| **Context Window** | 200K tokens | 1M tokens | 128K tokens |
| **Best For** | Code, reasoning | Research, summarization | Chat, brainstorming |
| **Strengths** | Long context, multi-step reasoning | Massive context, speed | General purpose, familiar |
| **Code Generation** | Excellent | Good | Good |
| **Research/Summary** | Good | Excellent | Good |
| **Reasoning Depth** | Excellent | Moderate | Good |
| **Speed** | Moderate | Fast | Fast |
| **Limitations** | Slower for simple tasks | Less reasoning depth | Shorter context |

---

## When to Use Claude

**Ideal for:**
- ✅ Writing code (functions, classes, refactoring)
- ✅ Multi-step problem solving (debugging, architecture decisions)
- ✅ Long document analysis (up to 200K tokens)
- ✅ Complex reasoning tasks

**Command:**
```bash
agm new --agent claude my-coding-session
```

**Example use cases:**
- Debugging a complex multi-file codebase
- Designing system architecture
- Code reviews with context from multiple files
- Refactoring legacy code

**Avoid for:**
- ❌ Processing documents >200K tokens (use Gemini)
- ❌ Simple quick questions (GPT is faster)

---

## When to Use Gemini

**Ideal for:**
- ✅ Research and summarization (massive context)
- ✅ Processing large datasets (up to 1M tokens)
- ✅ Document analysis (books, research papers, logs)
- ✅ Tasks requiring maximum context window

**Command:**
```bash
agm new --agent gemini research-task
```

**Example use cases:**
- Summarizing entire books or long research papers
- Analyzing large log files or datasets
- Research across many documents simultaneously
- Processing massive codebases

**Avoid for:**
- ❌ Complex multi-step reasoning (Claude is better)
- ❌ Code generation requiring deep reasoning

---

## When to Use GPT

**Ideal for:**
- ✅ General chat and brainstorming
- ✅ Quick Q&A
- ✅ Familiar OpenAI interface
- ✅ General-purpose tasks

**Command:**
```bash
agm new --agent gpt chat-session
```

**Example use cases:**
- Brainstorming ideas
- Quick questions and answers
- General writing assistance
- Tasks not requiring specialized capabilities

**Avoid for:**
- ❌ Very long context (>128K tokens)
- ❌ Deep multi-step reasoning

---

## Command Translator Support

AGM provides unified commands that work across all agents using the `CommandTranslator` abstraction.

**Supported commands (all agents):**
- `RenameSession` - Rename agent session/conversation
- `SetDirectory` - Set working directory context
- `RunHook` - Execute initialization hook

**Agent-specific behavior:**
- **Claude:** Full command support
- **Gemini:** Core commands supported
- **GPT:** Core commands supported

For unsupported commands, AGM gracefully degrades (returns `ErrNotSupported`).

See [Command Translator documentation](https://github.com/vbonnet/ai-tools/tree/main/claude-session-manager#-command-translation-multi-agent) for details.

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

**GPT:**
- Provider: OpenAI
- Requires: OpenAI API key
- Pricing: Pay-per-token (see OpenAI pricing)

**Setup:** See [AGM README](../README.md) for API key configuration.

---

## Switching Agents

You can use different agents for different sessions:

```bash
# Use Claude for code work
agm new --agent claude my-code-session

# Use Gemini for research
agm new --agent gemini research-session

# Use GPT for chat
agm new --agent gpt quick-chat

# Resume any session (agent auto-detected)
agm resume my-code-session
```

Agent selection is stored in session manifest and persists across resume operations.

---

## Still Have Questions?

- **BDD scenarios:** See [BDD-CATALOG.md](BDD-CATALOG.md) for agent behavior examples
- **Troubleshooting:** See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for agent-specific issues
- **Migration:** See [MIGRATION-CLAUDE-MULTI.md](MIGRATION-CLAUDE-MULTI.md) to transition from single to multi-agent
