# Swarm Executor

**Safe parallel agent execution with automatic batch size validation**

## Overview

Swarm Executor prevents Claude Code deadlocks by enforcing safe batch size limits when launching parallel agents. Based on research from the 38-agent swarm deadlock incident (Task 0.3), this tool implements pre-flight validation and automatic wave-based batching.

## Research Foundation

**Context:** 38-agent swarm triggered confirmed deadlock (RNl+ state, 14.4% CPU, 12+ minutes stuck)

**Root Cause:** SSE stream overload during concurrent API operations

**Safe Limit:** **15 agents maximum** per parallel execution
- Hardware capacity: 8 cores × 2 = 16 agents (theoretical)
- Observed failure: 38 agents = deadlock
- Safety margin: 60% buffer from failure point
- Formula: `min(CPU_cores × 2, 15)`

## Features

- ✅ Pre-flight validation of swarm size against safe limits
- ✅ Automatic batching for large swarms (waves of 15 agents)
- ✅ Risk assessment based on historical deadlock data
- ✅ Progress tracking per wave
- ✅ Configurable limits and behavior
- ✅ Override mode for advanced users (with confirmation)

## Usage

### Validate Swarm Size

Check swarm size without executing:

```bash
# Basic validation
swarm-executor validate --queue tasks.yaml

# With custom limit
swarm-executor validate --queue tasks.yaml --max-parallel 12
```

**Output:**
```
Swarm Size Validation Report
============================

📊 Swarm Analysis
   Queue file: tasks.yaml
   Agent count: 35
   Safe limit: 15 agents

🔍 Risk Assessment
   Risk level: HIGH (21-30 agents)
   Status: ⚠️  EXCEEDS LIMIT by 20 agents (133%)
   Recommendation: Auto-batch into waves

📦 Recommended Batching Strategy
   Total waves: 3
   Wave size: 15 agents (max)

   Wave 1: 15 agents
   Wave 2: 15 agents
   Wave 3: 5 agents

⏱️  Estimated Impact
   Unbatched: ~10 minutes (high deadlock risk)
   Batched: ~30 minutes (20 min overhead, safe)

💡 Next Steps
   To execute with auto-batching:
     swarm-executor execute --queue tasks.yaml
```

### Execute Swarm

Launch swarm with automatic batching:

```bash
# Auto-batch if exceeds limit
swarm-executor execute --queue tasks.yaml

# Custom limit
swarm-executor execute --queue tasks.yaml --max-parallel 20

# Disable batching (risky - requires confirmation)
swarm-executor execute --queue tasks.yaml --batch-mode off --allow-override
```

**Output (35-agent swarm):**
```
Swarm Executor - Batch Size Validation
======================================
Queue: tasks.yaml
Max Parallel: 15 agents
Batch Mode: auto

🔍 Pre-Flight Validation
   Agent count: 35
   Safe limit: 15
   Status: ⚠️  Exceeds safe limit by 20 agents
   Risk level: 92% of known failure threshold

📦 Auto-Batching Enabled
   Splitting into 3 waves
   Wave size: 15 agents (max)

🚀 Launching Swarm (35 agents in 3 waves)
==========================================

Wave 1/3: Launching 15 agents...
Wave 1/3: ✓ Complete (2.0s)

Wave 2/3: Launching 15 agents...
Wave 2/3: ✓ Complete (2.0s)

Wave 3/3: Launching 5 agents...
Wave 3/3: ✓ Complete (2.0s)

✅ Swarm Complete
   Total agents: 35
   Total waves: 3
   Status: All waves completed successfully
```

## Configuration

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--max-parallel` | 15 | Maximum parallel agents (safe limit) |
| `--batch-mode` | auto | Batching mode: auto, manual, off |
| `--safety-margin` | 0.6 | Safety margin from failure point (60%) |
| `--allow-override` | false | Allow overriding limits with confirmation |
| `--verbose` | false | Verbose output |

### Batch Modes

**auto** (recommended):
- Automatically batch swarms exceeding limit
- No user intervention required
- Safe and reliable

**manual**:
- Prompt user for batching decision
- Shows risk assessment
- User chooses: batch, override, or cancel

**off** (risky):
- Disable batching entirely
- Requires `--allow-override` flag
- User must confirm risk acknowledgment
- **Not recommended** - high deadlock risk

## Task Queue Format

Create a YAML file listing tasks to execute:

```yaml
# tasks.yaml
tasks:
  - task: "Implement feature A"
    bead: "oss-abc"

  - task: "Fix bug B"
    bead: "oss-def"

  - task: "Refactor module C"
    bead: "oss-ghi"

  # ... up to 15 tasks recommended
```

Or use bead IDs from command output:

```yaml
beads:
  - oss-abc
  - oss-def
  - oss-ghi
```

## Integration

### With Beads

List open beads and create queue:

```bash
# Get open beads
bd list --status open --format json > beads.json

# Create task queue from beads
jq -r '.[] | "- bead: " + .id' beads.json > tasks.yaml

# Execute with validation
swarm-executor execute --queue tasks.yaml
```

### With Engram Swarms

Integrate with engram-swarm workflow:

```bash
# After /engram-swarm:start creates beads
# Extract bead IDs from ROADMAP.md
grep "**Bead**:" ROADMAP.md | sed 's/.*: \(oss-[a-z0-9]*\).*/- bead: \1/' > tasks.yaml

# Validate before launch
swarm-executor validate --queue tasks.yaml

# Execute with batching
swarm-executor execute --queue tasks.yaml
```

## Implementation Status

**Phase 1 - Task 1.1 (oss-2w3):**
- [x] Basic CLI structure
- [x] Pre-flight validation logic
- [x] Auto-batching implementation
- [x] Risk assessment
- [ ] Claude Code integration (Task tool wrapper)
- [ ] Progress monitoring
- [ ] Failure handling and recovery
- [ ] Metrics logging integration (depends on Task 1.2)

**Next Steps:**
1. Integrate with Claude Code Task tool for actual agent launches
2. Add progress monitoring during wave execution
3. Implement failure handling (retry, skip, abort)
4. Connect to metrics database (Task 1.2)

## Testing

```bash
# Build
go build -o swarm-executor ./cmd/swarm-executor

# Test with demo queue
echo "- task: Demo 1
- task: Demo 2
- task: Demo 3" > demo.yaml

swarm-executor validate --queue demo.yaml
swarm-executor execute --queue demo.yaml
```

## References

- **Research Report:** `PROJECT-3-BATCH-SIZE-RESEARCH-REPORT.md` (733 lines)
- **Implementation Guide:** `PROJECT-3-IMPLEMENTATION-GUIDE.md` (448 lines)
- **ROADMAP:** Task 1.1 - Swarm Size Validation
- **Bead:** oss-2w3

---

**Version:** 1.0.0
**Status:** ✅ Core validation complete, integration pending
**Maintainer:** Foundation Engineering
