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
agm resume coding-session || agm new --harness claude-code coding-session
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
agm new --harness codex-cli quick-questions

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
agm new --harness claude-code feature-user-auth \
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
agm new --harness claude-code review-pr-123 \
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
agm new --harness claude-code debug-payment-timeout \
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
agm session send debug-payment-timeout \
  --prompt "⚠️ Session was stuck. Please continue analysis."
```

### Refactoring Project

**Scenario:** Large-scale refactoring with AI guidance

```bash
# Create refactoring session
agm new --harness claude-code refactor-legacy-auth \
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
agm new --harness gemini-cli research-ml-papers \
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
agm new --harness gemini-cli competitor-analysis \
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
agm new --harness gemini-cli analyze-prod-logs \
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
agm new --harness gemini-cli market-research-2026 \
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
agm new --harness gemini-cli research-microservices \
  --project ~/projects/myapp \
  --tags research,architecture

# Research phase
# > "Analyze microservices patterns in these 20 articles"
# > "Summarize pros/cons of each approach"

agm archive research-microservices

# Phase 2: Design with GPT
agm new --harness codex-cli design-microservices \
  --project ~/projects/myapp \
  --tags design,architecture

# Brainstorm phase
# > "Based on research, brainstorm 3 architecture options"
# > "Create decision matrix for selecting approach"

agm archive design-microservices

# Phase 3: Implement with Claude
agm new --harness claude-code implement-microservices \
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
agm new --harness codex-cli brainstorm-features \
  --tags brainstorm,product

# > "Brainstorm 20 innovative features for a task manager"

# Phase 2: Refine with Claude
agm new --harness claude-code refine-features \
  --tags design,product

# > "Analyze feasibility and prioritize these features"

# Phase 3: Implement with Claude
agm new --harness claude-code implement-top-features \
  --project ~/projects/taskapp \
  --tags implementation

# > "Implement the top 3 features"
```

### Documentation Generation

**Scenario:** Code → Research → Documentation

```bash
# Phase 1: Code with Claude
agm new --harness claude-code feature-api-endpoints \
  --project ~/projects/api

# > "Implement REST API endpoints for user management"

# Phase 2: Research examples with Gemini
agm new --harness gemini-cli research-api-docs

# > "Analyze API documentation examples from top companies"

# Phase 3: Generate docs with Claude
agm resume feature-api-endpoints

# > "Generate OpenAPI spec and user-facing docs for the API"
```

## Gemini-Specific Workflows

### Massive Document Analysis

**Scenario:** Process entire books or technical specifications

```bash
# Create session with Gemini (1M token context)
agm new --harness gemini-cli analyze-kubernetes-docs \
  --project ~/research/k8s \
  --tags research,kubernetes,documentation

# In session: Upload full Kubernetes documentation
# > "I'm uploading the complete Kubernetes documentation (800+ pages).
# > Create a comprehensive index organized by topic."

# Deep dive into specific topics
# > "Explain the entire networking model with examples"

# Generate training materials
# > "Create a 5-day training curriculum based on this documentation"

# Export for team
agm export analyze-kubernetes-docs --format markdown > k8s-training.md
```

**Why this works:** Gemini's 1M token context can hold entire documentation sets that would require multiple sessions with other agents.

### Cross-Repository Code Analysis

**Scenario:** Analyze patterns across multiple codebases

```bash
# Create analysis session
agm new --harness gemini-cli code-patterns-analysis \
  --project ~/projects \
  --tags analysis,code,security

# In session: Analyze multiple repositories
# > "I'm sharing code from 5 microservices (total 200K tokens).
# > Identify common authentication patterns and security issues."

# Compare implementations
# > "Which service has the best error handling? Provide examples."

# Generate recommendations
# > "Create a unified authentication library spec based on best patterns found."

# Document findings
# > "Generate security audit report with code examples and recommendations"

agm export code-patterns-analysis --format markdown > security-audit.md
```

**Pattern:** Gemini excels at cross-repository analysis where context exceeds 200K tokens.

### Large Dataset Processing

**Scenario:** Analyze CSV/JSON data files

```bash
# Create data analysis session
agm new --harness gemini-cli sales-data-analysis \
  --project ~/data/sales \
  --tags analysis,data,sales

# In session: Upload large datasets
# > "Analyzing sales_data.csv (50MB, 1M rows) and customer_feedback.json (100K entries).
# > Identify trends and correlations between sales and customer satisfaction."

# Time-series analysis
# > "Plot monthly revenue trends with seasonal adjustments"

# Segmentation
# > "Segment customers by behavior patterns and revenue contribution"

# Predictions
# > "Based on historical data, forecast Q2 2026 revenue by segment"

# Export insights
agm export sales-data-analysis --format jsonl > insights.jsonl
```

**Advantage:** Process datasets too large for spreadsheets, without writing Python scripts.

### Multi-Language Documentation Translation

**Scenario:** Translate technical documentation to multiple languages

```bash
# Create translation session
agm new --harness gemini-cli docs-translation \
  --project ~/docs \
  --tags translation,documentation,i18n

# In session: Translate large documentation
# > "I'm uploading our entire API documentation (500 pages).
# > Translate to: Spanish, French, German, Japanese, Chinese.
# > Maintain technical accuracy and code examples."

# Quality check
# > "Compare Spanish and French translations for consistency"

# Technical glossary
# > "Generate bilingual technical glossary for each language pair"

# Export translations
# (Use Gemini CLI's native file handling for outputs)
```

**Scale:** Translate documentation that would take weeks manually or cost thousands in professional translation services.

### Session Migration Examples

**Scenario:** Moving work between agents based on task changes

```bash
# Start with Gemini for research
agm new --harness gemini-cli research-auth-systems \
  --tags research,auth,security

# > "Analyze 20 authentication systems and compare approaches"
# > "Summarize findings in a decision matrix"

# Archive research session
agm archive research-auth-systems

# Switch to Claude for implementation
agm new --harness claude-code implement-oauth-server \
  --project ~/projects/auth \
  --tags implementation,oauth,backend

# Reference research in Claude session
# > "Based on research from session 'research-auth-systems',
# > implement OAuth 2.0 server using the recommended approach."

# Archive when complete
agm archive implement-oauth-server
```

**Pattern:** Use Gemini for broad research, Claude for focused implementation.

### Resume Workflow with Directory Authorization

**Scenario:** Resume session with additional authorized directories

```bash
# Create initial session
agm new --harness gemini-cli data-pipeline \
  --project ~/projects/pipeline \
  --authorized-dirs ~/data/raw,~/data/processed

# Work on pipeline...
# Exit session
# > exit

# Later: Need to access new directory
# Note: Cannot add directories to existing session
# Workaround: Terminate and recreate with full authorization

agm terminate data-pipeline

agm new --harness gemini-cli data-pipeline \
  --project ~/projects/pipeline \
  --authorized-dirs ~/data/raw,~/data/processed,~/data/models,~/configs

# Now all directories are authorized
agm resume data-pipeline
```

**Best practice:** Authorize all directories upfront during session creation.

### Concurrent Research Sessions

**Scenario:** Run parallel research on different topics

```bash
# Session 1: ML research
agm new --harness gemini-cli ml-sota-2026 \
  --project ~/research/ml \
  --tags research,ml,sota

# Session 2: Cloud architecture research
agm new --harness gemini-cli cloud-arch-patterns \
  --project ~/research/cloud \
  --tags research,cloud,architecture

# Session 3: Security research
agm new --harness gemini-cli zero-trust-security \
  --project ~/research/security \
  --tags research,security,zero-trust

# Work on all three in parallel
# Each maintains independent context and history

# List all research sessions
agm list | grep research

# Export all research findings
agm export ml-sota-2026 --format markdown > ml-findings.md
agm export cloud-arch-patterns --format markdown > cloud-findings.md
agm export zero-trust-security --format markdown > security-findings.md
```

**Use case:** Research sprints where multiple team members work on different topics simultaneously.

## Cross-Agent Migration Patterns

### Claude → Gemini Migration

**When to migrate:** Task requires >200K tokens of context

```bash
# Start with Claude (coding session)
agm new --harness claude-code api-implementation \
  --project ~/projects/api

# Implement basic API structure
# > "Create REST API with user authentication"

# Realize you need to analyze large API documentation
# Archive Claude session
agm archive api-implementation

# Switch to Gemini for documentation analysis
agm new --harness gemini-cli api-docs-research \
  --project ~/projects/api

# > "Analyze these 10 API documentation examples (500+ pages total)
# > and recommend best practices for our API design"

# Export findings
agm export api-docs-research --format markdown > api-best-practices.md

# Resume Claude session to apply findings
agm unarchive api-implementation
agm resume api-implementation

# > "Based on api-best-practices.md, refactor the API design"
```

**Pattern:** Claude → Gemini → Claude workflow for research-informed implementation.

### Gemini → Claude Migration

**When to migrate:** Task requires deep reasoning or complex code generation

```bash
# Start with Gemini (research phase)
agm new --harness gemini-cli database-research \
  --tags research,database

# Research database options
# > "Compare PostgreSQL, MySQL, CockroachDB, and Spanner for our use case"

# Archive research
agm archive database-research

# Switch to Claude for implementation planning
agm new --harness claude-code database-migration-plan \
  --project ~/projects/db

# > "Based on research from 'database-research',
# > create detailed migration plan from MySQL to CockroachDB"

# Implementation with Claude
# > "Generate migration scripts with rollback strategy"
# > "Implement data validation and testing framework"
```

**Pattern:** Gemini for broad research → Claude for detailed implementation.

### Multi-Agent Workflow: Research → Design → Implement → Document

**Complete lifecycle using optimal agent for each phase:**

```bash
# Phase 1: Research with Gemini (massive context)
agm new --harness gemini-cli research-microservices-2026 \
  --tags research,architecture,phase1

# > "Analyze 50 case studies on microservices architecture"
# > "Identify patterns, anti-patterns, and evolution trends"

agm export research-microservices-2026 --format markdown > research.md

# Phase 2: Design with Claude (reasoning)
agm new --harness claude-code design-our-architecture \
  --project ~/projects/platform \
  --tags design,architecture,phase2

# > "Based on research.md, design microservices architecture for our platform"
# > "Create service boundaries, API contracts, and deployment strategy"

agm export design-our-architecture --format markdown > design.md

# Phase 3: Implement with Claude (code generation)
agm new --harness claude-code implement-services \
  --project ~/projects/platform \
  --tags implementation,phase3

# > "Implement user-service, auth-service, and api-gateway based on design.md"
# > "Include tests, monitoring, and error handling"

# Phase 4: Document with Gemini (synthesis)
agm new --harness gemini-cli generate-documentation \
  --project ~/projects/platform \
  --tags documentation,phase4

# > "Generate comprehensive documentation from research.md, design.md, and codebase"
# > "Include architecture diagrams, API docs, runbooks, and onboarding guide"

agm export generate-documentation --format markdown > complete-docs.md

# Archive all phases
agm archive research-microservices-2026
agm archive design-our-architecture
agm archive implement-services
agm archive generate-documentation
```

**Workflow summary:**
1. **Gemini:** Research (broad, massive context)
2. **Claude:** Design (reasoning, decision-making)
3. **Claude:** Implementation (code generation)
4. **Gemini:** Documentation (synthesis, comprehensive output)

### Agent Selection Decision Tree

Use this decision flow in your sessions:

```bash
# Decision 1: Context size
if context > 200K tokens:
  agent = gemini
  use_case = "research, large documents, logs, datasets"
else:
  continue to decision 2

# Decision 2: Task type
if task == "code_generation" or task == "debugging":
  agent = claude
  use_case = "implement features, debug issues, refactor code"
elif task == "research" or task == "summarization":
  agent = gemini
  use_case = "analyze papers, summarize docs, compare approaches"
elif task == "brainstorming" or task == "quick_chat":
  agent = gpt
  use_case = "ideation, Q&A, general assistance"

# Decision 3: Multi-step reasoning
if requires_complex_reasoning:
  agent = claude
  use_case = "architecture decisions, problem solving, analysis"

# Create session with selected agent
agm new --harness $harness session-name
```

**Example decision process:**

- **"Debug payment timeout"** → Claude (code + reasoning)
- **"Analyze 50 research papers"** → Gemini (massive context)
- **"Brainstorm product names"** → GPT (quick ideation)
- **"Design database schema"** → Claude (reasoning + implementation)
- **"Summarize 500MB logs"** → Gemini (large documents)

## Team & Collaboration

### Code Handoff

**Scenario:** Share session context with teammate

```bash
# Your work
agm new --harness claude-code feature-payment-gateway \
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
agm new --harness claude-code pair-refactor-auth \
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
agm new --harness gemini-cli learn-kubernetes \
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
  agm new --harness claude-code "$task" \
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
  agm session send "$session" --prompt-file ~/prompts/diagnosis.txt

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
agm new --harness gemini-cli research-quantum-computing \
  --project ~/research/quantum \
  --tags research,quantum,phase1

# Work on research...

# Archive after month 1
agm archive research-quantum-computing

# Month 2: Deep dive (create new session, reference old)
agm new --harness gemini-cli research-quantum-algorithms \
  --project ~/research/quantum \
  --tags research,quantum,phase2

# > "Context: Previous research in session 'research-quantum-computing'"
# > "Deep dive into quantum algorithms based on previous findings"

# Month 3: Final synthesis
agm new --harness gemini-cli research-quantum-final \
  --project ~/research/quantum \
  --tags research,quantum,final

# > "Synthesize findings from phase1 and phase2"
# > "Generate final research paper"
```

### Cross-Project Code Analysis

**Scenario:** Analyze code patterns across multiple projects

```bash
# Use Gemini for massive context
agm new --harness gemini-cli analyze-auth-patterns \
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
agm new --harness claude-code incident-db-outage \
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
agm new --harness claude-code fix-db-connection-pool \
  --project ~/projects/myapp \
  --tags fix,database,post-incident \
  --description "Fix connection pool issues from incident"
```

### Onboarding New Team Members

**Scenario:** Use AI to help onboard to codebase

```bash
# Create onboarding session
agm new --harness claude-code onboarding-john \
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
agm new --harness gemini-cli research-postgres-to-cockroachdb \
  --tags research,migration,database

# > "Compare PostgreSQL and CockroachDB for our use case"
# > "Identify migration challenges and risks"

agm archive research-postgres-to-cockroachdb

# Phase 2: Plan with Claude
agm new --harness claude-code plan-db-migration \
  --project ~/projects/myapp \
  --tags planning,migration

# > "Create migration plan with zero-downtime strategy"
# > "Generate testing checklist"

agm archive plan-db-migration

# Phase 3: Execute with Claude
agm new --harness claude-code execute-db-migration \
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
agm new --harness claude-code optimize-api-performance \
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
