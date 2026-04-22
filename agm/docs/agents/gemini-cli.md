# Gemini CLI Adapter - User Guide

Comprehensive guide for using Google's Gemini AI agent with AGM (AI/Agent Session Manager).

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Configuration](#configuration)
- [Getting Started](#getting-started)
- [Feature Overview](#feature-overview)
- [Usage Examples](#usage-examples)
- [Command Reference](#command-reference)
- [Known Limitations](#known-limitations)
- [Troubleshooting](#troubleshooting)
- [Comparison with Claude](#comparison-with-claude)

---

## Overview

The Gemini CLI adapter integrates Google's Gemini AI into AGM, allowing you to create and manage Gemini sessions alongside Claude and other agents. Gemini excels at research, summarization, and tasks requiring massive context (up to 1M tokens).

**Key Benefits:**
- **Massive Context**: 1M token context window (5x larger than Claude)
- **Fast Processing**: Optimized for speed and efficiency
- **Unified Interface**: Same AGM commands work across all agents
- **Session Persistence**: Resume sessions with full context preserved
- **Directory Authorization**: Pre-approve workspace directories to avoid trust prompts

**What Works:**
- ✅ Session creation and management
- ✅ Message sending and conversation history
- ✅ Session resume with UUID tracking
- ✅ Directory authorization via `--include-directories`
- ✅ Command execution (SetDir, ClearHistory, SetSystemPrompt)
- ✅ Lifecycle hooks (SessionStart, SessionEnd, etc.)
- ✅ Multi-session support (concurrent sessions)

**What Doesn't Work (Yet):**
- ❌ Real-time streaming output in AGM (works in native Gemini CLI)
- ❌ HTML conversation export (JSONL and Markdown supported)
- ❌ Runtime directory authorization (must be set at session creation)

---

## Installation

### Prerequisites

1. **Gemini CLI** - Install the official Google Gemini CLI
2. **tmux** - Required for session management
3. **AGM** - AI/Agent Session Manager

### Step 1: Install Gemini CLI

```bash
# Install via npm (official method)
npm install -g @google-labs/gemini-cli

# Verify installation
gemini --version
```

**Alternative:** Follow [Google's official installation guide](https://github.com/google-labs/gemini-cli) for platform-specific instructions.

### Step 2: Install tmux

```bash
# Ubuntu/Debian
sudo apt-get install tmux

# macOS
brew install tmux

# Verify installation
tmux -V
```

### Step 3: Install AGM

```bash
# Install from source
go install github.com/vbonnet/ai-tools/agm/cmd/agm@latest

# Verify installation
agm --version
```

---

## Configuration

### API Key Setup

Gemini CLI requires a Google AI API key. Set it as an environment variable:

```bash
# Add to your ~/.bashrc or ~/.zshrc
export GEMINI_API_KEY="your-api-key-here"

# Reload shell configuration
source ~/.bashrc
```

**Get an API Key:**
1. Visit [Google AI Studio](https://makersuite.google.com/app/apikey)
2. Sign in with your Google account
3. Create a new API key
4. Copy the key and set the environment variable

**Verify Configuration:**

```bash
# Test Gemini CLI directly
gemini "Hello, Gemini!"

# If successful, you'll see a response
# If it fails, check your API key and network connection
```

### Directory Authorization (Optional but Recommended)

Pre-authorize directories to avoid interactive trust prompts:

```bash
# Create a Gemini config file (if needed)
mkdir -p ~/.gemini
touch ~/.gemini/config.yaml

# AGM handles directory authorization automatically
# No manual configuration required!
```

**How it works:** When you create a session with AGM, it automatically passes `--include-directories` to authorize your workspace. No manual setup needed.

---

## Getting Started

### Create Your First Gemini Session

```bash
# Basic session creation
agm new --harness gemini-cli my-research-session

# With working directory
agm new --harness gemini-cli research-task --project ~/documents/research

# With tags for organization
agm new --harness gemini-cli literature-review --tags research,papers,ml
```

**What Happens:**
1. AGM creates a new tmux session
2. Starts Gemini CLI with authorized directories
3. Stores session metadata (UUID, working directory, timestamps)
4. Returns to your shell with session ready for use

### Resume a Session

```bash
# Resume by name
agm resume my-research-session

# Or use interactive picker
agm
# (Select "my-research-session" from the list)
```

**Session Persistence:** AGM extracts and stores Gemini's native session UUID, ensuring you can resume exactly where you left off, even after closing tmux or restarting your machine.

### Send Messages

Once in a session, interact naturally with Gemini:

```bash
# Inside the Gemini session
> Analyze this research paper and summarize key findings

> Compare approaches from these 5 papers: ...

> Generate a literature review table
```

**Exit session:** Type `exit` or press `Ctrl+D` to close the Gemini CLI while preserving the session in AGM.

---

## Feature Overview

### Session Management

**Create Sessions:**
```bash
agm new --harness gemini-cli session-name
```

**Resume Sessions:**
```bash
agm resume session-name
```

**List Sessions:**
```bash
agm list
# Shows all sessions with their agents, status, and timestamps
```

**Terminate Sessions:**
```bash
agm terminate session-name
# Gracefully exits Gemini and removes session metadata
```

### Directory Authorization

AGM automatically authorizes your workspace when creating sessions:

```bash
# Single directory (working directory)
agm new --harness gemini-cli my-session --project ~/workspace

# Multiple directories
agm new --harness gemini-cli multi-dir-session \
  --project ~/workspace \
  --authorized-dirs ~/data,~/models,~/configs
```

**Behind the scenes:**
```bash
# AGM runs this for you:
gemini --include-directories '~/workspace' \
       --include-directories '~/data' \
       --include-directories '~/models'
```

**Why this matters:** Gemini CLI prompts for directory trust on first access. Pre-authorization eliminates interactive prompts, enabling automation and unattended operation.

### Conversation History

**View History:**
```bash
# Get session history via AGM API (for developers)
# Or view directly in Gemini CLI (in session)
```

**Export Conversations:**
```bash
# JSONL format (full message history)
agm export my-session --format jsonl > conversation.jsonl

# Markdown format (human-readable)
agm export my-session --format markdown > conversation.md
```

**Note:** HTML export is not yet supported for Gemini sessions.

### Command Execution

AGM provides unified commands that work across agents:

**Set Working Directory:**
```bash
# Changes the working directory within the session
agm command setdir my-session --path ~/new/workspace
```

**Clear History:**
```bash
# Removes conversation history (fresh start)
agm command clear-history my-session
```

**Set System Prompt:**
```bash
# Updates the system instruction
agm command set-system-prompt my-session \
  --prompt "You are a research assistant specializing in machine learning."
```

**Rename Session:**
```bash
# Updates session title (in both AGM and Gemini CLI)
agm rename my-session research-project-alpha
```

---

## Usage Examples

### Research & Summarization

**Scenario:** Analyze multiple research papers

```bash
# Create research session with large context
agm new --harness gemini-cli paper-analysis --project ~/research

# In session: Upload papers and analyze
> I'm uploading 10 research papers on neural architecture search.
> Please summarize each paper in 2-3 sentences.

> Now compare the approaches and identify common themes.

> Generate a comparison table with columns: Paper, Approach, Strengths, Weaknesses.

# Exit session (preserves all context)
> exit

# Resume later to continue analysis
agm resume paper-analysis
```

**Why Gemini?** The 1M token context window can hold multiple full-length papers simultaneously, enabling comprehensive cross-document analysis.

### Log Analysis

**Scenario:** Debug production issues from large log files

```bash
# Create session for log analysis
agm new --harness gemini-cli debug-prod-logs --project ~/logs

# In session: Analyze logs
> I'm pasting 500MB of error logs from the past 7 days.
> Identify patterns and frequency of timeout errors.

> What's the root cause of the database connection errors?

> Generate a timeline of incidents with error counts per hour.

# Archive session when resolved
agm archive debug-prod-logs
```

**Tip:** Gemini can process massive log files that exceed other agents' context limits.

### Multi-Day Research Project

**Scenario:** Long-term research with periodic sessions

```bash
# Week 1: Initial research
agm new --harness gemini-cli quantum-research \
  --project ~/research/quantum \
  --tags research,quantum,phase1

# Work on research...
# Archive when done with phase 1
agm archive quantum-research

# Week 2: Deep dive (new session, reference old)
agm new --harness gemini-cli quantum-algorithms \
  --project ~/research/quantum \
  --tags research,quantum,phase2

# In session:
> Context: Previous research in session 'quantum-research'
> Deep dive into quantum error correction based on previous findings.

# Week 3: Synthesis
agm new --harness gemini-cli quantum-synthesis \
  --tags research,quantum,final

> Synthesize findings from phase1 and phase2
> Generate final research paper outline
```

**Pattern:** Create separate sessions for distinct phases, reference previous sessions for context continuity.

### Concurrent Sessions

**Scenario:** Work on multiple research topics simultaneously

```bash
# Session 1: ML research
agm new --harness gemini-cli ml-research --project ~/ml

# Session 2: Database optimization
agm new --harness gemini-cli db-optimization --project ~/db

# Session 3: API design
agm new --harness gemini-cli api-design --project ~/api

# List all active sessions
agm list

# Switch between sessions as needed
agm resume ml-research
agm resume db-optimization
agm resume api-design
```

**Each session maintains:**
- Independent conversation history
- Separate working directories
- Unique Gemini session UUIDs
- Isolated context (no cross-contamination)

---

## Command Reference

### Session Lifecycle

| Command | Description | Example |
|---------|-------------|---------|
| `agm new --harness gemini-cli <name>` | Create new Gemini session | `agm new --harness gemini-cli research` |
| `agm resume <name>` | Resume existing session | `agm resume research` |
| `agm list` | List all sessions | `agm list` |
| `agm terminate <name>` | End session and cleanup | `agm terminate research` |
| `agm archive <name>` | Archive completed session | `agm archive research` |

### Session Commands

| Command | Description | Example |
|---------|-------------|---------|
| `agm command setdir <session> --path <dir>` | Change working directory | `agm command setdir research --path ~/new-dir` |
| `agm command clear-history <session>` | Clear conversation history | `agm command clear-history research` |
| `agm command set-system-prompt <session>` | Set system instruction | `agm command set-system-prompt research --prompt "..."` |
| `agm rename <session> <new-name>` | Rename session | `agm rename research ml-research` |

### Export & Import

| Command | Description | Example |
|---------|-------------|---------|
| `agm export <session> --format jsonl` | Export as JSONL | `agm export research --format jsonl > chat.jsonl` |
| `agm export <session> --format markdown` | Export as Markdown | `agm export research --format markdown > chat.md` |

---

## Known Limitations

### Current Limitations

1. **Real-time Streaming Output in AGM**
   - **What:** Gemini's streaming responses don't display in real-time within AGM-managed tmux sessions
   - **Why:** AGM uses tmux for session management, which buffers output
   - **Workaround:** Attach to the tmux session directly: `tmux attach -t <session-name>`
   - **Status:** Planned improvement in future release

2. **HTML Export**
   - **What:** `agm export --format html` is not supported for Gemini
   - **Why:** Gemini CLI stores conversations in JSONL format without HTML templates
   - **Workaround:** Use Markdown export instead: `agm export session --format markdown`
   - **Status:** Low priority (Markdown provides similar functionality)

3. **Runtime Directory Authorization**
   - **What:** Cannot add authorized directories after session creation
   - **Why:** Gemini CLI accepts `--include-directories` only at startup
   - **Workaround:** Authorize all needed directories during session creation
   - **Status:** Gemini CLI limitation (not AGM-specific)

4. **History File Timing**
   - **What:** Conversation history may not be immediately available via AGM API
   - **Why:** Gemini CLI writes to history file asynchronously
   - **Workaround:** Wait a few seconds after message send before calling `GetHistory()`
   - **Status:** Inherent to Gemini CLI's file-based history

### Comparison with Claude

| Feature | Gemini | Claude | Notes |
|---------|--------|--------|-------|
| Context Window | 1M tokens | 200K tokens | Gemini: 5x larger |
| Session Resume | ✅ | ✅ | Both support UUID-based resume |
| Streaming Output | ⚠️ Buffered | ✅ Real-time | Claude: Better UX in tmux |
| Directory Auth | ✅ Pre-auth | ✅ Runtime | Gemini: Requires upfront authorization |
| HTML Export | ❌ | ✅ | Claude: More export formats |
| Code Generation | Good | Excellent | Claude: Better for complex code |
| Research/Summary | Excellent | Good | Gemini: Better for large documents |

**When to use Gemini:**
- Processing >200K tokens of content
- Research and summarization tasks
- Large log file analysis
- Multi-document comparison

**When to use Claude:**
- Complex code generation
- Multi-step reasoning
- Interactive debugging
- Better streaming output experience

---

## Troubleshooting

### Session Creation Fails

**Symptom:** `agm new --harness gemini-cli` returns error

**Possible Causes & Solutions:**

1. **Gemini CLI not installed**
   ```bash
   # Check installation
   which gemini

   # If not found, install
   npm install -g @google-labs/gemini-cli
   ```

2. **API key not set**
   ```bash
   # Check environment variable
   echo $GEMINI_API_KEY

   # If empty, set it
   export GEMINI_API_KEY="your-api-key"
   ```

3. **tmux not installed**
   ```bash
   # Check tmux
   which tmux

   # Install if missing (Ubuntu/Debian)
   sudo apt-get install tmux
   ```

### Session Resume Fails

**Symptom:** `agm resume my-session` returns "session not found"

**Solutions:**

1. **Check session exists**
   ```bash
   # List all sessions
   agm list

   # Verify session name matches exactly
   ```

2. **UUID extraction failed**
   - AGM may have failed to extract Gemini's UUID
   - Resume will fall back to "latest" session
   - Check AGM logs for UUID extraction warnings

3. **Session was terminated**
   ```bash
   # Check archived sessions
   agm list --archived

   # Unarchive if needed
   agm unarchive my-session
   ```

### Messages Not Sending

**Symptom:** Messages sent via AGM don't appear in Gemini

**Solutions:**

1. **Session not active**
   ```bash
   # Check session status
   agm list

   # Resume if paused
   agm resume my-session
   ```

2. **Tmux session terminated**
   ```bash
   # List tmux sessions
   tmux ls

   # If missing, resume will recreate
   agm resume my-session
   ```

3. **Gemini CLI crashed**
   ```bash
   # Attach to tmux and check
   tmux attach -t <session-name>

   # If crashed, exit and resume
   exit
   agm resume my-session
   ```

### History Not Available

**Symptom:** `agm export` or `GetHistory()` returns empty

**Causes & Solutions:**

1. **Timing issue** - Gemini CLI writes history asynchronously
   ```bash
   # Wait a few seconds after sending messages
   sleep 5
   agm export my-session --format jsonl
   ```

2. **History file location** - AGM looks in `~/.gemini/sessions/`
   ```bash
   # Check if history file exists
   ls ~/.gemini/sessions/*/history.jsonl

   # If missing, history may not be written yet
   ```

3. **Session too new** - No messages sent yet
   ```bash
   # Send at least one message first
   agm resume my-session
   # > Hello, Gemini!
   # > exit
   agm export my-session --format jsonl
   ```

### Directory Trust Prompts

**Symptom:** Gemini prompts for directory trust despite authorization

**Solutions:**

1. **Authorize at creation**
   ```bash
   # Include all directories upfront
   agm new --harness gemini-cli my-session \
     --project ~/workspace \
     --authorized-dirs ~/data,~/models
   ```

2. **Check Gemini config**
   ```bash
   # View Gemini's trusted directories
   cat ~/.gemini/config.yaml

   # Manually add to config if needed
   ```

3. **Workaround for existing sessions**
   - Cannot add directories after creation
   - Terminate and recreate session with full authorization
   ```bash
   agm terminate my-session
   agm new --harness gemini-cli my-session --authorized-dirs ~/all,~/needed,~/dirs
   ```

### API Rate Limits

**Symptom:** Gemini returns 429 errors (rate limit exceeded)

**Solutions:**

1. **Wait and retry**
   ```bash
   # Wait for rate limit reset (typically 1 minute)
   sleep 60
   agm resume my-session
   ```

2. **Reduce message frequency**
   - Batch multiple questions into single messages
   - Avoid rapid-fire messages

3. **Check quota**
   - Visit [Google AI Studio](https://makersuite.google.com/app/apikey)
   - Review your quota limits and usage

### Performance Issues

**Symptom:** Slow response times or high latency

**Solutions:**

1. **Check context size**
   - Very large context (near 1M tokens) may slow responses
   - Consider splitting into multiple sessions

2. **Network latency**
   ```bash
   # Test network connection to Google AI
   ping generativelanguage.googleapis.com
   ```

3. **System resources**
   ```bash
   # Check memory usage
   free -h

   # Check CPU usage
   top

   # Gemini CLI may use significant resources for large context
   ```

### Getting Help

If you encounter issues not covered here:

1. **Check AGM logs**
   ```bash
   # View AGM debug output
   AGM_DEBUG=1 agm resume my-session
   ```

2. **Check Gemini CLI logs**
   ```bash
   # Attach to tmux session
   tmux attach -t <session-name>

   # View Gemini's error messages directly
   ```

3. **Report issues**
   - GitHub: [AGM Issues](https://github.com/vbonnet/ai-tools/issues)
   - Include: AGM version, Gemini CLI version, error messages

---

## Advanced Usage

### Custom System Prompts

Set role-specific instructions:

```bash
# Create session with custom system prompt
agm new --harness gemini-cli research-assistant

# Set system prompt
agm command set-system-prompt research-assistant \
  --prompt "You are a research assistant specializing in machine learning.
  Provide detailed, academic-style responses with citations when possible."

# All subsequent messages use this system prompt
agm resume research-assistant
```

### Session Hooks

Execute custom scripts on session lifecycle events:

```bash
# AGM automatically triggers hooks for:
# - SessionStart: When session is created
# - SessionEnd: When session is terminated
# - BeforeAgent: Before attaching to session
# - AfterAgent: After detaching from session

# Hook configuration stored in ~/.agm/gemini-hooks/
# Each hook receives session metadata as JSON
```

**Example hook use case:**
- Log session starts to analytics
- Backup conversation history on session end
- Update project status on session events

### Multi-Workspace Organization

Organize sessions by workspace:

```bash
# Create workspace-specific sessions
agm new --harness gemini-cli ml-research \
  --project ~/workspaces/ml \
  --workspace ml-team

agm new --harness gemini-cli web-dev \
  --project ~/workspaces/web \
  --workspace web-team

# List sessions by workspace
agm list --workspace ml-team

# Resume all workspace sessions
agm sessions resume-all --workspace ml-team
```

### Integration with Other Tools

**Export for analysis:**
```bash
# Export to JSONL for processing
agm export research --format jsonl | jq '.[] | select(.role == "assistant")'

# Count message exchanges
agm export research --format jsonl | jq '. | length'
```

**Git integration:**
```bash
# AGM auto-commits session manifests
cd ~/.agm
git log  # View session creation history

# Manual export and commit conversations
agm export important-research --format markdown > research-notes.md
git add research-notes.md
git commit -m "Save research session notes"
```

---

## Next Steps

- **User Guide:** See [USER-GUIDE.md](../USER-GUIDE.md) for comprehensive AGM documentation
- **Examples:** See [EXAMPLES.md](../EXAMPLES.md) for Gemini-specific workflows
- **Agent Comparison:** See [AGENT-COMPARISON.md](../AGENT-COMPARISON.md) to compare agents
- **API Reference:** See [API-REFERENCE.md](../API-REFERENCE.md) for developer documentation

---

**Last Updated:** 2026-03-11
**AGM Version:** 3.0+
**Gemini CLI Version:** Compatible with @google-labs/gemini-cli 1.0+
**Maintained By:** AGM Gemini Integration Team
