# AGM Examples & Use Cases

Real-world examples and scenarios for using AGM (AI/Agent Session Manager) effectively.

## Table of Contents

- [Daily Workflows](#daily-workflows)
- [Software Development](#software-development)
- [Research & Analysis](#research--analysis)
- [Multi-Agent Collaboration](#multi-agent-collaboration)
- [Team & Collaboration](#team--collaboration)
- [Automation & Scripting](#automation--scripting)
- [Advanced Scenarios](#advanced-scenarios)

## Daily Workflows

### Morning Startup Routine

**Scenario:** Start your day by resuming yesterday's work

```bash
# Option 1: Interactive picker shows all sessions
agm

# Option 2: Resume specific session
agm resume coding-session

# Option 3: Create if doesn't exist, resume if exists
agm resume coding-session || agm new --agent claude coding-session
```

**Workflow:**
1. Run `agm` to see all active sessions
2. Select yesterday's session with arrow keys
3. Resume where you left off
4. Context and conversation history preserved

### End of Day Cleanup

**Scenario:** Archive completed work, keep ongoing sessions

```bash
# List all sessions to review
agm list

# Archive completed sessions
agm archive feature-123-completed
agm archive bug-fix-456

# Or use batch cleanup
agm clean
# (Interactive: select completed sessions)
```

**Best practice:**
- Archive sessions when feature/task is done
- Keep active sessions running
- Use weekly cleanup for old sessions

### Context Switching

**Scenario:** Switch between multiple projects throughout the day

```bash
# Morning: Backend work
agm resume backend-api

# Afternoon: Frontend work
agm resume frontend-ui

# Quick chat for questions
agm new --agent gpt quick-questions

# Back to backend
agm resume backend-api
```

**Tip:** Use fuzzy matching for quick switching:
```bash
agm back      # Matches "backend-api"
agm front     # Matches "frontend-ui"
```

## Software Development

### Feature Development

**Scenario:** Implement a new feature from start to finish

```bash
# Create session for feature
agm new --agent claude feature-user-auth \
  --project ~/projects/myapp \
  --tags feature,auth,backend

# Work on implementation...
# (Claude helps with code generation, debugging)

# When done, archive
agm archive feature-user-auth
```

**Session content:**
- Design discussions
- Code implementation
- Debugging sessions
- Test writing

### Code Review

**Scenario:** Review pull request with AI assistance

```bash
# Create review session
agm new --agent claude review-pr-123 \
  --project ~/projects/myapp \
  --description "Review: Add payment integration"

# In session, analyze code
# > "Review the changes in src/payment.go for security issues"

# Document findings
# > "Create a checklist of required changes"

# Archive when review complete
agm archive review-pr-123
```

**Multi-file review:**
```bash
# Claude can analyze multiple files with 200K context
# > "Compare old vs new implementation in these 5 files: ..."
```

### Debugging Session

**Scenario:** Debug complex production issue

```bash
# Create debugging session
agm new --agent claude debug-payment-timeout \
  --project ~/projects/myapp \
  --tags bug,urgent,payment

# Send logs for analysis
# > "Analyze these error logs and identify the root cause"

# Test hypothesis
# > "Generate test cases to reproduce the timeout"

# Document solution
# > "Summarize the bug, root cause, and fix"

# Archive when resolved
agm archive debug-payment-timeout
```

**Recovery scenario:**
```bash
# If session hangs during analysis
agm send debug-payment-timeout \
  --prompt "⚠️ Session was stuck. Please continue analysis."
```

### Refactoring Project

**Scenario:** Large-scale refactoring with AI guidance

```bash
# Create refactoring session
agm new --agent claude refactor-legacy-auth \
  --project ~/projects/myapp \
  --tags refactor,auth,tech-debt

# Phase 1: Analyze current code
# > "Analyze the auth module and identify code smells"

# Phase 2: Design new architecture
# > "Propose a refactoring plan with minimal breaking changes"

# Phase 3: Implement incrementally
# (Resume session daily as you refactor)

# Archive when complete
agm archive refactor-legacy-auth
```

**Long-running refactor:**
```bash
# Work on refactoring over multiple days
# Day 1
agm resume refactor-legacy-auth

# Day 2
agm resume refactor-legacy-auth
# (Full context preserved from Day 1)
```

## Research & Analysis

### Literature Review

**Scenario:** Review academic papers for research project

```bash
# Use Gemini for massive context (1M tokens)
agm new --agent gemini research-ml-papers \
  --project ~/research/machine-learning \
  --tags research,ml,papers

# Upload multiple papers
# > "Summarize these 10 papers on neural architecture search"

# Extract insights
# > "Create comparison table of approaches"

# Generate bibliography
# > "List all papers with citations in APA format"

# Archive when complete
agm archive research-ml-papers
```

**Why Gemini?**
- 1M token context can hold multiple papers
- Excellent summarization
- Fast processing

### Competitive Analysis

**Scenario:** Analyze competitor products and features

```bash
# Use Gemini for research
agm new --agent gemini competitor-analysis \
  --project ~/research \
  --tags research,competitive,strategy

# Analyze multiple sources
# > "Compare features of Product A, B, C based on these docs"

# Create SWOT analysis
# > "Generate SWOT analysis for each competitor"

# Strategic recommendations
# > "What features should we prioritize to differentiate?"
```

### Log Analysis

**Scenario:** Analyze large application logs for patterns

```bash
# Use Gemini for large log files
agm new --agent gemini analyze-prod-logs \
  --project ~/logs \
  --tags logs,production,debugging

# Upload log files (up to 1M tokens)
# > "Identify error patterns in these 500MB of logs"

# Time-series analysis
# > "Plot error frequency over the past 7 days"

# Root cause analysis
# > "What's the common cause of the timeout errors?"
```

### Market Research

**Scenario:** Research market trends and customer needs

```bash
# Use Gemini for research
agm new --agent gemini market-research-2026 \
  --project ~/research/market \
  --tags research,market,customer

# Analyze survey data
# > "Summarize key insights from 1000 customer responses"

# Identify trends
# > "What are the top 5 feature requests?"

# Strategic planning
# > "Based on research, what should our 2026 roadmap focus on?"
```

## Multi-Agent Collaboration

### Research to Implementation Pipeline

**Scenario:** Research → Design → Implement workflow

```bash
# Phase 1: Research with Gemini
agm new --agent gemini research-microservices \
  --project ~/projects/myapp \
  --tags research,architecture

# Research phase
# > "Analyze microservices patterns in these 20 articles"
# > "Summarize pros/cons of each approach"

agm archive research-microservices

# Phase 2: Design with GPT
agm new --agent gpt design-microservices \
  --project ~/projects/myapp \
  --tags design,architecture

# Brainstorm phase
# > "Based on research, brainstorm 3 architecture options"
# > "Create decision matrix for selecting approach"

agm archive design-microservices

# Phase 3: Implement with Claude
agm new --agent claude implement-microservices \
  --project ~/projects/myapp \
  --tags implementation,code

# Implementation phase
# > "Implement the selected architecture with Go"
# > "Generate tests for each microservice"

agm archive implement-microservices
```

**Benefits:**
- Right agent for each phase
- Clear separation of concerns
- Full conversation history preserved

### Creative Brainstorming to Execution

**Scenario:** Brainstorm → Refine → Execute

```bash
# Phase 1: Brainstorm with GPT
agm new --agent gpt brainstorm-features \
  --tags brainstorm,product

# > "Brainstorm 20 innovative features for a task manager"

# Phase 2: Refine with Claude
agm new --agent claude refine-features \
  --tags design,product

# > "Analyze feasibility and prioritize these features"

# Phase 3: Implement with Claude
agm new --agent claude implement-top-features \
  --project ~/projects/taskapp \
  --tags implementation

# > "Implement the top 3 features"
```

### Documentation Generation

**Scenario:** Code → Research → Documentation

```bash
# Phase 1: Code with Claude
agm new --agent claude feature-api-endpoints \
  --project ~/projects/api

# > "Implement REST API endpoints for user management"

# Phase 2: Research examples with Gemini
agm new --agent gemini research-api-docs

# > "Analyze API documentation examples from top companies"

# Phase 3: Generate docs with Claude
agm resume feature-api-endpoints

# > "Generate OpenAPI spec and user-facing docs for the API"
```

## Team & Collaboration

### Code Handoff

**Scenario:** Share session context with teammate

```bash
# Your work
agm new --agent claude feature-payment-gateway \
  --project ~/projects/payment

# Document approach
# > "Summarize the implementation approach for the payment gateway"
# > "List remaining tasks and blockers"

# Archive session (preserves manifest)
agm archive feature-payment-gateway

# Share session name with teammate
# Teammate can create their own session and reference:
# > "Context: See agm session 'feature-payment-gateway' for background"
```

**Note:** Sessions are local, but conversation summaries can be shared

### Pair Programming

**Scenario:** Collaborate with AI during pair programming

```bash
# Create shared context session
agm new --agent claude pair-refactor-auth \
  --project ~/projects/myapp

# Driver: Work on code
# Navigator (Claude): Review and suggest

# Rotate roles periodically
# > "Review the last 3 functions I wrote for issues"

# Continue next day
agm resume pair-refactor-auth
```

### Knowledge Transfer

**Scenario:** Document learnings for team

```bash
# Research session
agm new --agent gemini learn-kubernetes \
  --tags learning,kubernetes

# Learn and document
# > "Explain Kubernetes networking concepts"
# > "Create cheat sheet for common kubectl commands"
# > "Document troubleshooting steps for pod issues"

# Export summary
# > "Generate a comprehensive guide for the team"

# Archive when complete
agm archive learn-kubernetes
```

## Automation & Scripting

### Batch Session Creation

**Scenario:** Create multiple sessions programmatically

```bash
#!/bin/bash
# create-project-sessions.sh

PROJECT_ROOT=~/projects/myapp
TASKS=("backend-api" "frontend-ui" "database-schema" "deployment")

for task in "${TASKS[@]}"; do
  agm new --agent claude "$task" \
    --project "$PROJECT_ROOT" \
    --tags project-alpha,setup \
    --detached
done

echo "Created ${#TASKS[@]} sessions"
agm list
```

### Automated Cleanup

**Scenario:** Weekly cleanup script

```bash
#!/bin/bash
# weekly-cleanup.sh

# Archive stopped sessions older than 7 days
agm list --format=json | jq -r '
  .[] |
  select(.status == "stopped") |
  select(.updated < (now - 604800)) |
  .name
' | while read session; do
  echo "Archiving: $session"
  agm archive "$session" --force
done

# Delete archived sessions older than 30 days
agm list --all --format=json | jq -r '
  .[] |
  select(.status == "archived") |
  select(.updated < (now - 2592000)) |
  .name
' | while read session; do
  echo "Deleting: $session"
  # Manual deletion (agm doesn't have delete command yet)
  rm -rf ~/.claude-sessions/"$session"
done
```

### Session Health Monitoring

**Scenario:** Monitor session health in CI/CD

```bash
#!/bin/bash
# monitor-sessions.sh

# Run health check
agm doctor --validate --json > health-report.json

# Parse results
UNHEALTHY=$(jq -r '.unhealthy_sessions | length' health-report.json)

if [ "$UNHEALTHY" -gt 0 ]; then
  echo "⚠️ Found $UNHEALTHY unhealthy sessions"
  jq -r '.unhealthy_sessions[] | "\(.name): \(.issue)"' health-report.json
  exit 1
fi

echo "✓ All sessions healthy"
```

### Automated Recovery

**Scenario:** Recover stuck sessions automatically

```bash
#!/bin/bash
# recover-stuck-sessions.sh

# Find stuck sessions (no activity in 1 hour)
STUCK_SESSIONS=$(agm list --format=json | jq -r '
  .[] |
  select(.status == "active") |
  select(.updated < (now - 3600)) |
  .name
')

for session in $STUCK_SESSIONS; do
  echo "Recovering stuck session: $session"

  # Send diagnosis prompt
  agm send "$session" --prompt-file ~/prompts/diagnosis.txt

  # Wait for response
  sleep 30

  # Check if recovered
  STATUS=$(agm list --format=json | jq -r ".[] | select(.name==\"$session\") | .status")
  if [ "$STATUS" == "active" ]; then
    echo "✓ Recovered: $session"
  else
    echo "❌ Failed to recover: $session"
  fi
done
```

## Advanced Scenarios

### Long-Running Research Projects

**Scenario:** Multi-month research project with periodic sessions

```bash
# Month 1: Initial research
agm new --agent gemini research-quantum-computing \
  --project ~/research/quantum \
  --tags research,quantum,phase1

# Work on research...

# Archive after month 1
agm archive research-quantum-computing

# Month 2: Deep dive (create new session, reference old)
agm new --agent gemini research-quantum-algorithms \
  --project ~/research/quantum \
  --tags research,quantum,phase2

# > "Context: Previous research in session 'research-quantum-computing'"
# > "Deep dive into quantum algorithms based on previous findings"

# Month 3: Final synthesis
agm new --agent gemini research-quantum-final \
  --project ~/research/quantum \
  --tags research,quantum,final

# > "Synthesize findings from phase1 and phase2"
# > "Generate final research paper"
```

### Cross-Project Code Analysis

**Scenario:** Analyze code patterns across multiple projects

```bash
# Use Gemini for massive context
agm new --agent gemini analyze-auth-patterns \
  --tags research,code-analysis,security

# Analyze authentication implementations
# > "Compare auth implementations in these 5 projects"
# > "Identify security vulnerabilities and best practices"
# > "Generate security audit report"

# Create recommendations
# > "Generate authentication library spec based on analysis"
```

### Incident Response

**Scenario:** Production incident investigation and resolution

```bash
# Create incident session
agm new --agent claude incident-db-outage \
  --project ~/projects/myapp \
  --tags incident,critical,database

# Document timeline
# > "Document incident timeline starting at 14:30 UTC"

# Analyze logs
# > "Analyze error logs from the past 2 hours"

# Root cause analysis
# > "Identify root cause based on logs and metrics"

# Create action items
# > "Generate incident report with action items"

# Archive when resolved
agm archive incident-db-outage

# Create follow-up session for fixes
agm new --agent claude fix-db-connection-pool \
  --project ~/projects/myapp \
  --tags fix,database,post-incident \
  --description "Fix connection pool issues from incident"
```

### Onboarding New Team Members

**Scenario:** Use AI to help onboard to codebase

```bash
# Create onboarding session
agm new --agent claude onboarding-john \
  --project ~/projects/myapp \
  --tags onboarding,documentation

# Generate codebase overview
# > "Analyze the codebase and create architecture overview"

# Explain key components
# > "Explain how authentication works in this app"

# Create guided tour
# > "Generate step-by-step guide for running the app locally"

# Document common tasks
# > "Document how to add a new API endpoint"

# Share summary with new team member
```

### Migration Planning

**Scenario:** Plan and execute technology migration

```bash
# Phase 1: Research with Gemini
agm new --agent gemini research-postgres-to-cockroachdb \
  --tags research,migration,database

# > "Compare PostgreSQL and CockroachDB for our use case"
# > "Identify migration challenges and risks"

agm archive research-postgres-to-cockroachdb

# Phase 2: Plan with Claude
agm new --agent claude plan-db-migration \
  --project ~/projects/myapp \
  --tags planning,migration

# > "Create migration plan with zero-downtime strategy"
# > "Generate testing checklist"

agm archive plan-db-migration

# Phase 3: Execute with Claude
agm new --agent claude execute-db-migration \
  --project ~/projects/myapp \
  --tags implementation,migration

# > "Implement dual-write strategy"
# > "Generate migration scripts"

# Monitor execution
agm resume execute-db-migration
# (Resume daily during multi-day migration)
```

### Performance Optimization

**Scenario:** Systematic performance analysis and optimization

```bash
# Create optimization session
agm new --agent claude optimize-api-performance \
  --project ~/projects/api \
  --tags optimization,performance

# Baseline analysis
# > "Analyze current API performance metrics"

# Identify bottlenecks
# > "Profile the code and identify top 5 bottlenecks"

# Generate optimizations
# > "Suggest optimization strategies for each bottleneck"

# Implement and test
# (Iterative process over multiple days)

# Document results
# > "Compare before/after metrics and document improvements"

agm archive optimize-api-performance
```

## Tips & Patterns

### Naming Conventions

**Project-based:**
```bash
agm new myapp-backend-api
agm new myapp-frontend-ui
agm new myapp-database-schema
```

**Task-based:**
```bash
agm new implement-user-auth
agm new debug-payment-timeout
agm new refactor-legacy-code
```

**Phase-based:**
```bash
agm new research-microservices
agm new design-microservices
agm new implement-microservices
```

### Session Hierarchies

**Parent-child relationship via naming:**
```bash
# Parent
agm new project-alpha

# Children
agm new project-alpha-backend
agm new project-alpha-frontend
agm new project-alpha-deployment
```

**Benefits:**
- Logical grouping
- Easy to find related sessions
- Pattern matching: `agm unarchive "project-alpha*"`

### Progressive Summarization

**Pattern:** Summarize before archiving

```bash
# Before archiving
agm resume my-session

# Summarize key points
# > "Summarize this conversation in 5 bullet points"

# Archive with context preserved
agm archive my-session
```

**Benefits:**
- Quick reference later
- Easy handoff to others
- Better searchability

## Next Steps

- **User Guide:** See [USER-GUIDE.md](USER-GUIDE.md) for comprehensive documentation
- **CLI Reference:** See [CLI-REFERENCE.md](CLI-REFERENCE.md) for all commands
- **FAQ:** See [FAQ.md](FAQ.md) for common questions

---

**Last updated:** 2026-02-03
**AGM Version:** 3.0
**Maintained by:** Foundation Engineering
