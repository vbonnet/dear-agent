# Multi-Session Coordination Guide

**Version**: 1.0
**Last Updated**: 2026-02-20
**Target Audience**: AGM users implementing multi-session workflows

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
- [Session State Management](#session-state-management)
- [Inter-Session Messaging](#inter-session-messaging)
- [Coordination Patterns](#coordination-patterns)
- [Advanced Workflows](#advanced-workflows)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

---

## Overview

### What is Multi-Session Coordination?

AGM's multi-session coordination system enables multiple Claude sessions to work together on complex tasks. Sessions can communicate asynchronously through a message queue, coordinating their work based on dynamic state awareness.

### Key Capabilities

- **State-Aware Messaging**: Send messages only when recipient is ready
- **Asynchronous Communication**: Non-blocking message delivery with retry logic
- **Session State Tracking**: Automatic DONE/WORKING/COMPACTING/OFFLINE detection
- **Acknowledgment Protocol**: Confirm message delivery and processing
- **Background Daemon**: Reliable message delivery without manual intervention

### Architecture at a Glance

```
┌─────────────┐                    ┌─────────────┐
│  Session A  │                    │  Session B  │
│  (Sender)   │                    │ (Receiver)  │
└──────┬──────┘                    └──────▲──────┘
       │                                  │
       │ agm send session-b "task"        │
       │                                  │
       └─────────▶ Message Queue ─────────┘
                   (SQLite)
                       │
                       ▼
                  AGM Daemon
                  - Polls every 30s
                  - Checks session state
                  - Delivers when DONE
```

---

## Quick Start

### Prerequisites

1. **AGM installed**: `agm version` should show v3.x+
2. **Tmux installed**: `tmux -V` should show 3.0+
3. **Claude CLI configured**: `claude --version`

### 5-Minute Setup

```bash
# 1. Start the daemon (manages message delivery)
agm daemon start

# 2. Create two sessions
agm new research-session --detached
agm new analysis-session --detached

# 3. Send a message from one session to another
agm send analysis-session "Please analyze the research findings in ~/reports/data.csv"

# 4. Check message status
agm daemon status --queue

# 5. Resume target session to see delivered message
agm resume analysis-session
```

### Verification

```bash
# Check daemon is running
agm daemon status
# Output: Daemon is running (PID: 12345)

# View queue contents
agm daemon status --queue
# Output: 1 pending message, 0 delivered

# Tail daemon logs
tail -f ~/.agm/logs/daemon/daemon.log
```

---

## Core Concepts

### Session States

AGM tracks four session states to enable intelligent message routing:

| State | Description | Message Delivery | Updated By |
|-------|-------------|------------------|------------|
| **DONE** | Session idle, awaiting input | ✅ Immediate | User interaction, hooks |
| **WORKING** | Processing a request | ⏸️ Deferred | Message delivery, hooks |
| **COMPACTING** | Database maintenance | ⏸️ Deferred | Claude compact hooks |
| **OFFLINE** | Session not running | ⏸️ Retried (3x) | Daemon detection |

### State Transitions

```
Session Lifecycle:
  CREATE → DONE (initial state)
  DONE → WORKING (message received)
  WORKING → DONE (response complete)
  DONE → COMPACTING (compact start)
  COMPACTING → DONE (compact complete)
  ANY → OFFLINE (session exits)
```

### Message Queue Lifecycle

```
Message Flow:
  1. ENQUEUED (agm send creates entry)
  2. PENDING (waiting for session to be DONE)
  3. DELIVERED (sent to session via tmux)
  4. ACK_RECEIVED (session confirms processing)
  5. ARCHIVED (removed from active queue)

Error Flow:
  PENDING → RETRY (attempt 1, 2, 3)
  RETRY → PERMANENTLY_FAILED (after 3 attempts)
```

### Daemon Polling Cycle

The daemon runs a 30-second polling loop:

```
Every 30 seconds:
  1. Query queue.GetAllPending()
  2. For each pending message:
     a. Resolve session identifier
     b. Detect session state
     c. Route based on state:
        - DONE → deliver immediately
        - WORKING/COMPACTING → skip (stays pending)
        - OFFLINE → retry with backoff
  3. Check for acknowledgment timeouts (60s)
  4. Requeue timed-out messages
```

---

## Session State Management

### Automatic State Detection

AGM uses a hybrid detection strategy:

1. **Manifest State**: Read `~/.agm/sessions/{name}/manifest.json` `state` field
2. **Tmux Validation**: Check `tmux list-sessions` for session existence
3. **Hook Updates**: Claude hooks update state on transitions

### State Update Triggers

**DONE State Set By:**
- `~/.claude/hooks/session-start/agm-state-ready` (on Claude startup)
- `~/.claude/hooks/message-complete/agm-state-ready` (after response)
- `~/.claude/hooks/compact-complete/agm-state-ready` (after compact)

**WORKING State Set By:**
- AGM daemon (after delivering message)
- User input (hook on message submission)

**COMPACTING State Set By:**
- `~/.claude/hooks/compact-start/agm-state-compacting`

**OFFLINE State Set By:**
- Daemon (detects missing tmux session)

### Manual State Management

```bash
# Query current state
agm session get-state research-session
# Output: DONE

# Force state update (admin only)
agm admin set-state research-session OFFLINE

# View state history
agm session history research-session --filter state
```

### Hook Installation

AGM automatically installs state management hooks during session creation. To manually install:

```bash
# Copy hooks to Claude config directory
cp ~/.agm/hooks/claude/* ~/.claude/hooks/

# Verify hooks installed
ls -la ~/.claude/hooks/session-start/
ls -la ~/.claude/hooks/message-complete/
ls -la ~/.claude/hooks/compact-start/
ls -la ~/.claude/hooks/compact-complete/
```

---

## Inter-Session Messaging

### Basic Messaging

**Syntax:**
```bash
agm send <session-name-or-id> "<message>"
```

**Examples:**
```bash
# Send to specific session by name
agm send analysis-session "Analyze the latest data"

# Send to session by ID prefix
agm send a1b2c3 "Update the report"

# Send from within a session (automatic sender detection)
agm send research-session "Research complete, proceeding to phase 2"

# Send with high priority (future feature)
agm send --priority high critical-session "Urgent: API quota exhausted"
```

### Message Format

Messages are delivered as multi-line prompts to the target session:

```
[Delivered Format in Target Session]
> From: research-session (12345678-abcd-ef01-2345-6789abcdef01)
> Sent: 2026-02-20T14:30:00Z
>
> Analyze the latest data in ~/reports/q4-2025.csv and summarize key trends.
```

### Delivery Guarantees

AGM provides **at-least-once delivery**:

- ✅ Messages persist in SQLite queue (survives daemon restarts)
- ✅ Automatic retry (3 attempts with exponential backoff)
- ✅ Acknowledgment protocol (confirms delivery)
- ⚠️ No guarantee of ordering (parallel delivery possible)
- ⚠️ Duplicate delivery possible (on network errors, use idempotency)

### Acknowledgment Protocol

```bash
# Send with acknowledgment (default)
agm send session-b "Task 1" --ack

# Send without acknowledgment (fire-and-forget)
agm send session-b "Task 2" --no-ack

# Wait for acknowledgment (blocks up to 60s)
agm send session-b "Critical task" --ack --wait

# Query acknowledgment status
agm daemon ack-status <message-id>
```

---

## Coordination Patterns

### Pattern 1: Parallel Research Tasks

**Use Case**: Distribute research tasks across multiple sessions.

```bash
# Create coordinator session
agm new coordinator --detached

# Create worker sessions
agm new research-papers --detached
agm new research-datasets --detached
agm new research-code --detached

# From coordinator session, dispatch tasks
agm send research-papers "Find papers on LLM fine-tuning published in 2025"
agm send research-datasets "Locate open-source datasets for sentiment analysis"
agm send research-code "Review top GitHub repos for Transformer implementations"

# Resume coordinator to aggregate results
agm resume coordinator
```

### Pattern 2: Sequential Pipeline

**Use Case**: Chain tasks where each step depends on the previous.

```bash
# Create pipeline sessions
agm new data-fetch --detached
agm new data-clean --detached
agm new data-analyze --detached

# Stage 1: Fetch data
agm send data-fetch "Download Q4 sales data from API endpoint /sales/q4"

# Stage 2: Clean (triggered after stage 1 completes)
agm send data-clean "Clean the CSV at ~/data/sales-q4-raw.csv, remove duplicates"

# Stage 3: Analyze (triggered after stage 2)
agm send data-analyze "Analyze cleaned data at ~/data/sales-q4-clean.csv, report trends"
```

**Implementation Pattern:**
Use message content to indicate readiness:
```bash
# In data-fetch session (after task completes):
agm send data-clean "Stage 1 complete, data ready at ~/data/sales-q4-raw.csv"
```

### Pattern 3: Broadcast to Multiple Sessions

**Use Case**: Send same instruction to multiple sessions.

```bash
# Create sessions for code review
agm new review-backend --detached
agm new review-frontend --detached
agm new review-tests --detached

# Broadcast code style update
for session in review-backend review-frontend review-tests; do
  agm send "$session" "Apply new code style guide: ~/docs/style-guide-v2.md"
done

# Check delivery status
agm daemon status --queue --filter pending
```

### Pattern 4: Request-Response Coordination

**Use Case**: Session A asks Session B for information, waits for response.

```bash
# Session A (requester)
agm send session-b "What is the current test coverage percentage?"

# Session B (responder) - after completing task
agm send session-a "Test coverage is 87.3% (measured 2026-02-20)"

# Session A receives response and continues work
```

**Best Practice**: Include correlation ID in messages:
```bash
agm send session-b "[REQ-001] What is the test coverage?"
agm send session-a "[RESP-REQ-001] Test coverage is 87.3%"
```

### Pattern 5: Error Handling & Recovery

**Use Case**: Detect and recover from failed tasks.

```bash
# Monitor daemon logs for failures
tail -f ~/.agm/logs/daemon/daemon.log | grep FAILED

# Query failed messages
agm daemon status --queue --filter failed

# Retry failed message manually
agm daemon retry <message-id>

# Restart session if offline
agm session restart data-processor

# Re-send message to restarted session
agm send data-processor "Retry: Process ~/data/large-dataset.csv"
```

---

## Advanced Workflows

### Workflow 1: Distributed Code Review

```bash
# 1. Create specialized review sessions
agm new review-security --harness claude-code --detached
agm new review-performance --harness claude-code --detached
agm new review-style --harness claude-code --detached

# 2. Dispatch review tasks
agm send review-security "Review ~/code/auth.py for security vulnerabilities"
agm send review-performance "Profile ~/code/api.py, identify bottlenecks"
agm send review-style "Check ~/code/*.py against PEP 8 style guide"

# 3. Create aggregator session
agm new review-summary --detached

# 4. Each review session sends results to aggregator
# (Manually from each session after review completes)
agm send review-summary "Security review: 2 critical issues found, see report"
agm send review-summary "Performance review: 3 slow queries identified"
agm send review-summary "Style review: 12 PEP 8 violations"

# 5. Resume aggregator to create final report
agm resume review-summary
# Prompt: "Synthesize all review findings into a prioritized action plan"
```

### Workflow 2: Autonomous Research Agent

```bash
# 1. Create research coordinator
agm new research-coordinator --detached

# 2. Create specialized researchers
agm new research-academic --detached  # Academic papers
agm new research-industry --detached  # Industry reports
agm new research-news --detached      # News articles

# 3. Coordinator dispatches research queries
agm send research-academic "Find latest academic papers on federated learning"
agm send research-industry "Locate Gartner/Forrester reports on edge computing"
agm send research-news "Search tech news for recent AI regulation developments"

# 4. Researchers send findings back
agm send research-coordinator "[ACADEMIC] Found 12 relevant papers, top 3 summarized"
agm send research-coordinator "[INDUSTRY] 2 Gartner reports, key insights attached"
agm send research-coordinator "[NEWS] EU AI Act updates, 3 major developments"

# 5. Coordinator synthesizes and generates report
agm resume research-coordinator
# Prompt: "Create executive summary from all research findings"
```

### Workflow 3: Multi-Stage Data Processing Pipeline

```bash
# 1. Define pipeline stages
STAGES=(
  "ingest"      # Fetch data from sources
  "validate"    # Check data quality
  "transform"   # Clean and normalize
  "enrich"      # Add external data
  "analyze"     # Run analytics
  "report"      # Generate insights
)

# 2. Create session for each stage
for stage in "${STAGES[@]}"; do
  agm new "pipeline-$stage" --detached
done

# 3. Trigger first stage
agm send pipeline-ingest "Fetch data from API endpoints defined in ~/config/sources.yaml"

# 4. Each stage triggers next (in session hooks or manually)
# Example from pipeline-ingest after completion:
agm send pipeline-validate "Data ingested to ~/data/raw/, validate schema"

# 5. Monitor pipeline progress
watch -n 5 'agm session list --filter "pipeline-*" --format table'

# 6. Check for errors in any stage
agm daemon status --queue --filter failed
```

---

## Best Practices

### Message Design

**DO:**
- ✅ Be explicit: "Analyze ~/data/sales.csv and output summary to ~/reports/summary.md"
- ✅ Include context: "Continuing from previous research on topic X"
- ✅ Specify output format: "Respond with JSON array of findings"
- ✅ Use correlation IDs: "[TASK-123] Process this dataset"

**DON'T:**
- ❌ Be vague: "Do the thing we discussed"
- ❌ Assume shared context: "Analyze that file"
- ❌ Send large payloads: Send file paths, not file contents
- ❌ Ignore errors: Always check daemon logs for failures

### Session Naming

**Conventions:**
```bash
# Good: Descriptive, task-oriented
agm new analyze-sales-q4
agm new review-auth-module
agm new research-llm-papers

# Better: Include project prefix
agm new myapp-backend-review
agm new myapp-frontend-tests

# Best: Hierarchical naming
agm new myapp.backend.api-review
agm new myapp.backend.db-optimization
agm new myapp.frontend.ui-refactor
```

### State Management

**Guidelines:**
1. **Trust the daemon**: Don't manually set state unless debugging
2. **Install hooks**: Ensure all sessions have state hooks installed
3. **Monitor state transitions**: Check `agm session history` for anomalies
4. **Handle OFFLINE gracefully**: Implement retry logic for critical messages

### Performance Tuning

**Optimize Polling Interval:**
```bash
# Default: 30s (good for most use cases)
agm daemon start

# Low latency (5s poll, higher CPU usage)
agm daemon start --poll-interval 5s

# Batch processing (2m poll, lower CPU usage)
agm daemon start --poll-interval 2m
```

**Queue Maintenance:**
```bash
# Archive delivered messages older than 7 days
agm daemon clean --older-than 7d --status delivered

# Purge failed messages
agm daemon clean --status failed

# Vacuum SQLite database
agm daemon vacuum
```

### Security Considerations

**Sensitive Data:**
- ❌ Never send API keys or passwords in messages
- ✅ Use file paths to sensitive data: "Use credentials in ~/secrets/api-key"
- ✅ Encrypt message queue database: Enable SQLite encryption (future feature)

**Session Isolation:**
- Each session runs in isolated tmux session
- Sessions cannot directly access each other's memory
- Messages are the only inter-session communication channel

---

## Troubleshooting

### Common Issues

#### Issue 1: Messages Not Delivered

**Symptoms:**
- `agm daemon status --queue` shows messages stuck in PENDING
- Daemon logs show "session not DONE" warnings

**Diagnosis:**
```bash
# Check target session state
agm session get-state target-session

# Check tmux session exists
tmux list-sessions | grep target-session

# Check daemon is running
agm daemon status
```

**Solutions:**
```bash
# If session is WORKING: Wait for it to become DONE
# If session is OFFLINE: Restart session
agm session restart target-session

# If daemon not running: Start daemon
agm daemon start

# If hooks not installed: Reinstall hooks
agm admin install-hooks target-session
```

#### Issue 2: Daemon Not Starting

**Symptoms:**
- `agm daemon start` fails with "daemon already running"
- PID file exists but process is dead

**Diagnosis:**
```bash
# Check PID file
cat ~/.agm/daemon.pid

# Check if process exists
ps -p $(cat ~/.agm/daemon.pid)
```

**Solutions:**
```bash
# Remove stale PID file
rm ~/.agm/daemon.pid

# Restart daemon
agm daemon start

# If still failing, check logs
tail -50 ~/.agm/logs/daemon/daemon.log
```

#### Issue 3: Duplicate Message Delivery

**Symptoms:**
- Same message delivered to session multiple times
- Acknowledgments timing out

**Diagnosis:**
```bash
# Check message status
agm daemon status --queue --verbose

# Check acknowledgment timeouts
grep "ack timeout" ~/.agm/logs/daemon/daemon.log
```

**Solutions:**
```bash
# Increase ack timeout (future config option)
# For now: Implement idempotency in message handlers
# Use correlation IDs to detect duplicates

# Example in session:
# "If you see [TASK-123] again, respond: Already processed"
```

#### Issue 4: Session State Stuck

**Symptoms:**
- Session shows WORKING but is actually idle
- Messages not delivered despite session being ready

**Diagnosis:**
```bash
# Check last state update
agm session history target-session --limit 10

# Check for stale state
agm admin validate-state target-session
```

**Solutions:**
```bash
# Force state to DONE
agm admin set-state target-session DONE

# Reinstall state hooks
agm admin install-hooks target-session

# Restart session (clears all state)
agm session restart target-session
```

### Debugging Commands

```bash
# Enable verbose daemon logging
agm daemon start --log-level debug

# Tail logs in real-time
tail -f ~/.agm/logs/daemon/daemon.log

# Query specific message
agm daemon message-info <message-id>

# Dump queue contents (JSON)
agm daemon status --queue --format json > queue-dump.json

# Check state hook execution
agm admin test-hooks target-session
```

### Recovery Procedures

**Scenario: Complete System Reset**
```bash
# 1. Stop daemon
agm daemon stop

# 2. Backup queue database
cp ~/.agm/queue.db ~/.agm/queue.db.backup

# 3. Archive all sessions
for session in $(agm session list --format simple); do
  agm session archive "$session"
done

# 4. Clean queue
agm daemon clean --all

# 5. Restart daemon
agm daemon start

# 6. Recreate sessions as needed
```

**Scenario: Corrupted Queue Database**
```bash
# 1. Stop daemon
agm daemon stop

# 2. Move corrupted database
mv ~/.agm/queue.db ~/.agm/queue.db.corrupt

# 3. Daemon creates new database on start
agm daemon start

# 4. Lost messages are unrecoverable (restore from backup if available)
```

---

## Next Steps

- **[Operations Runbook](OPERATIONS_RUNBOOK.md)**: Production deployment and monitoring
- **[Architecture Documentation](ARCHITECTURE.md)**: Deep dive into system internals
- **[Performance Guide](PERFORMANCE_TUNING.md)**: Optimization strategies
- **[API Reference](API-REFERENCE.md)**: Programmatic access to AGM

---

**Maintained by**: AGM Core Team
**Feedback**: Submit issues at https://github.com/vbonnet/dear-agent/issues
**License**: MIT
