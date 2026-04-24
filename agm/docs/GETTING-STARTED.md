# Getting Started with AGM

Welcome to AGM (AI/Agent Session Manager)! This guide will help you get started in under 10 minutes.

## What is AGM?

AGM is a smart session manager for AI agents (Claude, Gemini, GPT) that provides:

- **Multi-agent support** - Use Claude, Gemini, or GPT in unified sessions
- **Session management** - Create, resume, archive, and organize AI conversations
- **Interactive TUI** - Beautiful terminal interface with fuzzy search
- **Automatic tracking** - Auto-detects and associates AI sessions with projects
- **Command translation** - Unified commands work across all agents

**Evolved from:** Agent Session Manager (AGM)

## Installation

### Quick Install

```bash
# Install AGM
go install github.com/vbonnet/dear-agent/agm/cmd/agm@latest

# Verify installation
agm --version
```

### Enable Bash Completion (Recommended)

```bash
# Add to ~/.bashrc
if command -v agm &> /dev/null; then
    source <(agm completion bash)
fi

# Reload shell
source ~/.bashrc
```

**Features:**
- Command completion: `agm k<TAB>` → `agm kill`
- Session name completion
- No file fallback

## Prerequisites

### Required

- **tmux** - Session multiplexer
  ```bash
  # Ubuntu/Debian
  sudo apt-get install tmux

  # macOS
  brew install tmux
  ```

- **Go 1.24+** - For building from source
  ```bash
  # Check version
  go version
  ```

### Optional (for specific agents)

- **Claude CLI** - For Claude agent
- **Gemini CLI** - For Gemini agent (`npm install -g @google/generative-ai-cli`)
- **OpenAI CLI** - For GPT agent

## First Steps

### 1. Create Your First Session

```bash
# Create a session with Claude (default agent)
agm new my-first-session

# Or specify an agent explicitly
agm new --harness claude-code coding-session
agm new --harness gemini-cli research-task
agm new --harness codex-cli chat-session
```

**What happens:**
- AGM creates a tmux session
- Starts the selected AI agent
- Auto-detects and associates the agent's UUID
- Opens interactive session

### 2. List Your Sessions

```bash
# List active and stopped sessions
agm list

# Include archived sessions
agm list --all

# JSON output (for scripting)
agm list --format=json
```

**Output example:**
```
NAME              STATUS    AGENT   PROJECT                 UPDATED
my-first-session  active    claude  ~/projects/demo        2 minutes ago
research-task     stopped   gemini  ~/research             1 hour ago
```

### 3. Resume a Session

```bash
# Resume by exact name
agm resume my-first-session

# Or use fuzzy matching
agm my-fir        # Matches "my-first-session"

# Interactive picker (shows all sessions)
agm
```

**Fuzzy matching:**
- Typo-tolerant
- 0.6 similarity threshold
- "Did you mean?" prompts

### 4. Archive Old Sessions

```bash
# Archive a single session
agm archive my-first-session

# Interactive batch cleanup
agm clean
```

**Cleanup suggestions:**
- Stopped sessions >30 days → archive
- Archived sessions >90 days → delete

## Choosing an Agent

Not sure which agent to use? Here's a quick guide:

### Claude (Anthropic)
**Best for:** Code, reasoning, debugging

```bash
agm new --harness claude-code my-coding-session
```

**Use when you need:**
- ✅ Code generation and refactoring
- ✅ Multi-step reasoning
- ✅ Long context (200K tokens)
- ✅ Debugging complex issues

### Gemini (Google)
**Best for:** Research, summarization, massive context

```bash
agm new --harness gemini-cli research-task
```

**Use when you need:**
- ✅ Massive context (1M tokens)
- ✅ Document summarization
- ✅ Research across many files
- ✅ Processing large datasets

### GPT (OpenAI)
**Best for:** Chat, brainstorming, general Q&A

```bash
agm new --harness codex-cli chat-session
```

**Use when you need:**
- ✅ General chat
- ✅ Quick questions
- ✅ Brainstorming
- ✅ Familiar OpenAI interface

**Detailed comparison:** See [AGENT-COMPARISON.md](AGENT-COMPARISON.md)

## Configuration

### Create Configuration File

```bash
mkdir -p ~/.config/agm
cat > ~/.config/agm/config.yaml <<EOF
defaults:
  interactive: true
  auto_associate_uuid: true
  confirm_destructive: true
  cleanup_threshold_days: 30
  archive_threshold_days: 90

ui:
  theme: "agm"
  picker_height: 15
  show_project_paths: true
  fuzzy_search: true

advanced:
  tmux_timeout: "5s"
  health_check_cache: "5s"
  lock_timeout: "30s"
EOF
```

### Configure Agent API Keys

#### Claude
```bash
# Set API key
export ANTHROPIC_API_KEY="your-key-here"

# Or in ~/.bashrc
echo 'export ANTHROPIC_API_KEY="your-key-here"' >> ~/.bashrc
```

#### Gemini
```bash
# Set API key
export GEMINI_API_KEY="your-key-here"
export GOOGLE_GENAI_USE_VERTEXAI=false

# Or use agm doctor for setup help
agm doctor gemini
```

**Environment setup help:**
- Run `agm doctor <agent>` for validation
- See [agm-environment-management-spec.md](agm-environment-management-spec.md)

#### GPT
```bash
# Set API key
export OPENAI_API_KEY="your-key-here"

# Or in ~/.bashrc
echo 'export OPENAI_API_KEY="your-key-here"' >> ~/.bashrc
```

## Common Workflows

### Workflow 1: Daily Coding Session

```bash
# Morning: Create or resume coding session
agm resume coding-session || agm new --harness claude-code coding-session

# Work in session...

# Evening: Detach (session keeps running)
# Press Ctrl+b then d to detach

# Next day: Resume where you left off
agm resume coding-session
```

### Workflow 2: Research Project

```bash
# Create research session with Gemini (large context)
agm new --harness gemini-cli research-ai-papers

# Process multiple papers...

# Archive when done
agm archive research-ai-papers
```

### Workflow 3: Multi-Agent Workflow

```bash
# Use Claude for code
agm new --harness claude-code implement-feature

# Use Gemini for research
agm new --harness gemini-cli research-approach

# Use GPT for brainstorming
agm new --harness codex-cli brainstorm-ideas

# Switch between sessions
agm resume implement-feature
agm resume research-approach
```

## Essential Commands

### Session Management

```bash
agm new [session-name]           # Create new session
agm resume <session-name>        # Resume session
agm list                         # List sessions
agm archive <session-name>       # Archive session
agm clean                        # Batch cleanup
```

### Searching & Recovery

```bash
agm search "OAuth work"          # AI-powered semantic search
agm unarchive *pattern*          # Restore archived sessions
agm fix                          # Fix UUID associations
```

### Health & Diagnostics

```bash
agm doctor                       # Health check
agm doctor --validate            # Functional testing
agm doctor --validate --fix      # Auto-fix issues
agm doctor gemini                # Agent-specific validation
```

### Advanced Commands

```bash
agm session send <session> --prompt "text"    # Send message to session
agm session reject <session> --reason "..."   # Reject permission prompt
```

## Troubleshooting

### UUID Not Detected

**Problem:** Session created but UUID not auto-associated

**Solution:**
```bash
# Check Claude history
cat ~/.claude/history.jsonl | tail -5

# If empty, send a message in Claude, then:
agm fix --all
```

### Session Not Appearing

**Problem:** Created session doesn't show in list

**Solution:**
```bash
# Include archived sessions
agm list --all

# Check for duplicate directories
agm doctor
```

### Agent Not Available

**Problem:** "Harness not configured" error

**Solution:**
```bash
# Check agent setup
agm doctor <agent>

# Follow setup instructions
# See docs/agm-environment-management-spec.md
```

### tmux Session Lost

**Problem:** tmux session disappeared after logout

**Solution:**
```bash
# Enable user lingering (systemd)
loginctl enable-linger $USER

# Verify
agm doctor
```

**Detailed troubleshooting:** See [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## Next Steps

Now that you're up and running:

1. **Explore the docs:**
   - [User Guide](USER-GUIDE.md) - Comprehensive usage documentation
   - [Examples & Use Cases](EXAMPLES.md) - Real-world scenarios
   - [CLI Reference](CLI-REFERENCE.md) - Complete command reference
   - [FAQ](FAQ.md) - Common questions

2. **Customize your setup:**
   - [Accessibility](ACCESSIBILITY.md) - Screen reader support, high contrast
   - [BDD Catalog](BDD-CATALOG.md) - Living documentation of AGM behavior

3. **Advanced features:**
   - [Command Translation](COMMAND-TRANSLATION-DESIGN.md) - Multi-agent abstraction
   - [Agent Routing](../README.md#agent-routing-with-agentsmd) - Automatic agent selection

4. **Contributing:**
   - [Contributing Guide](../CONTRIBUTING.md) - Development setup and testing
   - [Test Plan](../TEST-PLAN.md) - Comprehensive testing strategy

## Getting Help

- **Documentation:** Check the `docs/` directory
- **Issues:** Open an issue on GitHub
- **Health check:** Run `agm doctor --validate` for diagnostics

## Quick Reference

```bash
# Create & manage sessions
agm new my-session              # Create session
agm                             # Interactive picker
agm my-ses                      # Fuzzy match & resume

# List & search
agm list                        # List sessions
agm list --all                  # Include archived
agm search "keyword"            # Semantic search

# Archive & cleanup
agm archive <session>           # Archive session
agm unarchive *pattern*         # Restore archived
agm clean                       # Batch cleanup

# Health & diagnostics
agm doctor                      # Health check
agm doctor --validate           # Functional tests
agm fix                         # Fix UUID associations
```

## Welcome to AGM!

You're now ready to manage AI agent sessions like a pro. Enjoy your productivity boost!

For questions, see [FAQ.md](FAQ.md) or open an issue.

---

**Last updated:** 2026-02-03
**AGM Version:** 3.0
**Maintained by:** Foundation Engineering
