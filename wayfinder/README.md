# Wayfinder Plugin

Structured SDLC workflow management with a 9-phase methodology that guides
AI-assisted development from charter through retrospective. Wayfinder acts as
a navigation system, ensuring AI agents complete all critical development
phases with validation gates between each step.

## Phases

| # | Phase | Purpose |
|---|-------|---------|
| 1 | CHARTER | Define problem, scope, success criteria |
| 2 | PROBLEM | Validate problem exists with evidence |
| 3 | RESEARCH | Search for existing solutions (build/buy/adapt) |
| 4 | DESIGN | Compare approaches, detailed architecture |
| 5 | SPEC | Functional and non-functional requirements |
| 6 | PLAN | Task breakdown, estimates, dependencies |
| 7 | SETUP | Prototype, proof-of-concept, environment setup |
| 8 | BUILD | TDD implementation with state machine enforcement |
| 9 | RETRO | Lessons learned, actual vs estimated effort |

## Commands

| Command | Purpose |
|---------|---------|
| `/wayfinder:start "desc"` | Create new project session |
| `/wayfinder:next` | Execute next phase |
| `/wayfinder:run-all-phases` | Autopilot through all remaining phases |
| `/wayfinder:close` | Complete project (completed/abandoned/blocked) |
| `/wayfinder:rewind` | Rewind to earlier phase |
| `/wayfinder:verify` | Validate and sign a phase file |

## Usage

### Start a Project

```
/wayfinder:start "Implement OAuth authentication"
```

Creates project directory with `WAYFINDER-STATUS.md` and begins CHARTER phase.

### Execute Phases

```
/wayfinder:next          # Execute next phase interactively
/wayfinder:run-all-phases  # Autopilot mode
```

### Complete Project

```
/wayfinder:close         # Validates retrospective and session hygiene
```

## When to Use Wayfinder

**Use when:**
- Multi-phase project (>1 day effort)
- Stakeholder alignment needed
- Build/buy/adapt decision required
- Complex implementation with validation needs

**Don't use when:**
- Simple task (<1 hour)
- Single-file change or obvious bug fix
- Requirements are already clear

## Key Features

- **Progressive rigor**: Auto-adjusts depth (Minimal/Standard/Thorough/Comprehensive)
  based on project complexity signals
- **Multi-persona validation**: Automatic domain expert detection (Security, ML, etc.)
  for design reviews
- **Phase isolation**: Validates artifacts contain only phase-appropriate content
- **Context compression**: Summarizes completed phases to reduce token usage (40-50%)
- **Filesystem as truth**: All state in YAML frontmatter and phase files, no hidden databases

## Dependencies

- Go >= 1.21 (`wayfinder-session` CLI)
- Node.js >= 18.0.0 (TypeScript phase orchestrator)
- Git (validation, archiving)
- `cobra` (CLI framework)
- `yaml` (frontmatter parsing)
