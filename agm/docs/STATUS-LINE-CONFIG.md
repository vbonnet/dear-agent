# AGM Status Line Configuration Guide

## Overview

The AGM status line displays real-time session information in your tmux status bar, including:
- Session state (DONE/WORKING/USER_PROMPT)
- Context usage percentage
- Git branch and uncommitted file count
- Agent type (Claude, Gemini, GPT, OpenCode)

## Installation

```bash
agm session install-tmux-status
```

This automatically:
1. Backs up your ~/.tmux.conf
2. Adds AGM status line configuration
3. Reloads tmux

## Configuration File

Create `~/.config/agm/config.yaml`:

```yaml
status_line:
  enabled: true
  default_format: "{{.AgentIcon}} #[fg={{.StateColor}}]{{.State}}#[default] | #[fg={{.ContextColor}}]{{.ContextPercent}}%#[default] | {{.Branch}} (+{{.Uncommitted}}) | {{.SessionName}}"
  refresh_interval: 10
  show_context_usage: true
  show_git_status: true
  agent_icons:
    claude: "🤖"
    gemini: "✨"
    gpt: "🧠"
    opencode: "💻"
  custom_formats:
    minimal: "{{.AgentIcon}} {{.State}} | {{.ContextPercent}}%"
    compact: "{{.AgentIcon}} #[fg={{.StateColor}}]●#[default] {{.ContextPercent}}% | {{.Branch}}"
    multi-agent: "{{.AgentIcon}}{{.AgentType}} | #[fg={{.StateColor}}]{{.State}}#[default] | {{.ContextPercent}}%"
    full: "{{.AgentIcon}} #[fg={{.StateColor}}]{{.State}}#[default] | CTX:#[fg={{.ContextColor}}]{{.ContextPercent}}%#[default] | {{.Branch}}(+{{.Uncommitted}}) | {{.SessionName}}"
```

## Template Variables

- `{{.AgentIcon}}` - Agent emoji (🤖/✨/🧠/💻)
- `{{.AgentType}}` - Agent name (claude/gemini/gpt/opencode)
- `{{.State}}` - Session state
- `{{.StateColor}}` - tmux color for state
- `{{.ContextPercent}}` - Context usage %
- `{{.ContextColor}}` - tmux color for context
- `{{.Branch}}` - Git branch
- `{{.Uncommitted}}` - Uncommitted file count
- `{{.SessionName}}` - AGM session name
- `{{.Workspace}}` - Workspace name

## Color Codes

**State Colors:**
- DONE: green
- WORKING: blue
- USER_PROMPT: yellow
- COMPACTING: magenta
- OFFLINE: colour240 (gray)

**Context Colors:**
- <70%: green (safe)
- 70-85%: yellow (warning)
- 85-95%: colour208 (orange)
- >95%: red (critical)

## Usage

```bash
# Auto-detect current session
agm session status-line

# Specific session
agm session status-line --session my-session

# Custom format
agm session status-line --format "{{.AgentIcon}} {{.ContextPercent}}%"

# JSON output
agm session status-line --json

# Set context usage manually
agm session set-context-usage 75
```

## Environment Variables

```bash
export AGM_STATUS_LINE_ENABLED=true
export AGM_STATUS_LINE_FORMAT="{{.AgentIcon}} {{.State}}"
```
