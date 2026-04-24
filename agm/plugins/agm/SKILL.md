---
name: agm
description: >-
  AGM (Agent Gateway Manager) session management for Claude and Gemini.
  TRIGGER when: user asks to associate a session, manage AGM sessions, exit and archive, or says "/agm", "agm session", or "associate session".
  DO NOT TRIGGER when: user wants to orchestrate multiple agents (use orchestrate/orchestrator) or manage tasks (use engram).
allowed-tools:
  - "Bash"
  - "Read"
  - "Write"
metadata:
  version: 0.4.0
  author: ai-tools
  activation_patterns:
    - "/agm"
    - "agm session"
    - "associate session"
    - "session management"
---

# agm Skill

**Purpose**: Manage AI agent sessions with AGM (Agent Gateway Manager) for persistence, multi-workspace detection, and session archival.

**When to use**: Starting new sessions, associating with existing AGM sessions, or cleanly exiting and archiving completed work.

**Invocation**: `/agm:agm-assoc` or `/agm:agm-exit`

---

## Commands Reference

| Command | Purpose | Example |
|---------|---------|---------|
| `/agm:agm-assoc` | Associate session with AGM | Auto-detects tmux session |
| `/agm:agm-assoc "{name}"` | Associate with explicit name | `agm:agm-assoc "phase-6-testing"` |
| `/agm:agm-exit` | Exit and archive session | Spawns async archive reaper |

---

## Workflow

### Associating a Session

**Auto-detect mode** (when in tmux):
```
/agm:agm-assoc
```

AGM will:
1. Detect current tmux session name
2. Create AGM manifest at `~/.agm/manifests/{session-name}.json`
3. Create ready-file signal: `~/.agm/ready-{session-name}`
4. Return success message

**Explicit mode** (specify session name):
```
/agm:agm-assoc "my-session-name"
```

Use when:
- Not in a tmux session
- Want to override auto-detected name
- Managing multiple concurrent sessions

### Exiting a Session

```
/agm:agm-exit
```

AGM will:
1. Spawn async archive reaper (background process)
2. Archive session files to `~/.agm/archive/{session-name}/`
3. Clean up manifest and ready-file
4. Exit the agent session

**Note**: The archival process runs asynchronously, so the session exits immediately.

---

## Use Cases

### Starting a New Project
1. Start tmux session: `tmux new -s project-name`
2. Start Claude/Gemini CLI
3. Associate with AGM: `/agm:agm-assoc`
4. Begin work

### Resuming Existing Work
1. Attach to tmux: `tmux attach -t project-name`
2. Start Claude/Gemini CLI
3. AGM auto-detects and resumes session context

### Completing a Session
1. Finish work
2. Exit cleanly: `/agm:agm-exit`
3. AGM archives session history, transcripts, manifests

---

## Requirements

**Dependencies**:
- `agm` CLI tool (installed at `~/go/bin/agm` or in PATH)
- `tmux` (for session detection)

**Verification**:
```bash
which agm    # Should return ~/go/bin/agm
which tmux   # Should return /usr/bin/tmux (or similar)
```

---

## Troubleshooting

**Problem**: "Not in tmux session and no session name provided"
- **Solution**: Either start a tmux session first, or provide explicit name: `/agm:agm-assoc "session-name"`

**Problem**: AGM association fails
- **Solution**: Check AGM installation: `agm doctor`

**Problem**: Session not persisting after restart
- **Solution**: Verify manifest exists: `ls ~/.agm/manifests/` and ready-file: `ls ~/.agm/ready-*`

---

## Integration with Other Tools

**Wayfinder**: AGM sessions integrate with Wayfinder projects - session name becomes project context

**Beads**: AGM tracks beads (tasks) associated with session

**Astrocyte**: AGM enables Astrocyte monitoring for system health and recovery

---

## Documentation

- AGM CLI: `main/agm/`
- Command files: `main/agm/agm-plugin/commands/`
