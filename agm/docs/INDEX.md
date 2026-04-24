# AGM Documentation Index

Complete guide to AGM (AI/Agent Gateway Manager) documentation.

**Version**: 3.0
**Last Updated**: 2026-02-04

---

## Quick Navigation

### Getting Started (5-10 minutes)
1. **[Quick Reference](AGM-QUICK-REFERENCE.md)** - One-page cheat sheet
2. **[Getting Started](GETTING-STARTED.md)** - Installation and first steps
3. **[Examples](EXAMPLES.md)** - Real-world usage scenarios

### Core Documentation
4. **[Command Reference](AGM-COMMAND-REFERENCE.md)** - Complete CLI reference
5. **[User Guide](USER-GUIDE.md)** - Comprehensive usage guide
6. **[Architecture](ARCHITECTURE.md)** - System architecture overview
7. **[API Reference](API-REFERENCE.md)** - Developer API documentation

### Specialized Guides
8. **[Agent Comparison](AGENT-COMPARISON.md)** - Choose the right agent
9. **[Migration Guide](AGM-MIGRATION-GUIDE.md)** - AGM to AGM migration
10. **[Troubleshooting](TROUBLESHOOTING.md)** - Common issues and solutions
11. **[FAQ](FAQ.md)** - Frequently asked questions

### Advanced Topics
12. **[BDD Catalog](BDD-CATALOG.md)** - Living documentation (behavior specs)
13. **[Accessibility](ACCESSIBILITY.md)** - WCAG compliance and screen readers
14. **[Command Translation](COMMAND-TRANSLATION-DESIGN.md)** - Multi-agent abstraction

---

## Documentation by Role

### For End Users

**I want to get started quickly:**
- **[Quick Reference](AGM-QUICK-REFERENCE.md)** - Essential commands
- **[Getting Started](GETTING-STARTED.md)** - 10-minute setup

**I need help with a specific task:**
- **[Examples](EXAMPLES.md)** - Real-world scenarios
- **[User Guide](USER-GUIDE.md)** - Comprehensive walkthrough
- **[Command Reference](AGM-COMMAND-REFERENCE.md)** - Complete command list

**I'm having problems:**
- **[Troubleshooting](TROUBLESHOOTING.md)** - Common issues
- **[FAQ](FAQ.md)** - Frequently asked questions
- **[Migration Guide](AGM-MIGRATION-GUIDE.md)** - AGM to AGM transition

**I want to choose the right agent:**
- **[Agent Comparison](AGENT-COMPARISON.md)** - Claude vs Gemini vs GPT

### For Developers

**I want to understand how AGM works:**
- **[Architecture](ARCHITECTURE.md)** - System design and components
- **[BDD Catalog](BDD-CATALOG.md)** - Behavior specifications

**I want to integrate with AGM:**
- **[API Reference](API-REFERENCE.md)** - Go package API
- **[Command Reference](AGM-COMMAND-REFERENCE.md)** - CLI interface

**I want to extend AGM:**
- **[Architecture](ARCHITECTURE.md)** - Extensibility section
- **[Command Translation](COMMAND-TRANSLATION-DESIGN.md)** - Multi-agent abstraction
- **[API Reference](API-REFERENCE.md)** - Agent interface

### For Contributors

**I want to contribute to AGM:**
- **[Architecture](ARCHITECTURE.md)** - System overview
- **[BDD Catalog](BDD-CATALOG.md)** - Test specifications
- **[API Reference](API-REFERENCE.md)** - Internal APIs

---

## Documentation by Topic

### Installation & Setup
- **[Getting Started](GETTING-STARTED.md)** - Installation, prerequisites, configuration
- **[Migration Guide](AGM-MIGRATION-GUIDE.md)** - Migrating from AGM

### Basic Usage
- **[Quick Reference](AGM-QUICK-REFERENCE.md)** - Essential commands
- **[Getting Started](GETTING-STARTED.md)** - First steps
- **[Examples](EXAMPLES.md)** - Common workflows

### Advanced Usage
- **[User Guide](USER-GUIDE.md)** - Comprehensive usage
- **[Examples](EXAMPLES.md)** - Advanced scenarios
- **[Command Reference](AGM-COMMAND-REFERENCE.md)** - All commands

### Multi-Agent Workflows
- **[Agent Comparison](AGENT-COMPARISON.md)** - Agent selection guide
- **[Examples](EXAMPLES.md)** - Multi-agent collaboration
- **[User Guide](USER-GUIDE.md)** - Agent-specific features

### Troubleshooting
- **[Troubleshooting](TROUBLESHOOTING.md)** - Common issues
- **[FAQ](FAQ.md)** - Quick answers
- **[Migration Guide](AGM-MIGRATION-GUIDE.md)** - Migration issues

### Architecture & Design
- **[Architecture](ARCHITECTURE.md)** - Complete system design
- **[Command Translation](COMMAND-TRANSLATION-DESIGN.md)** - Multi-agent abstraction
- **[BDD Catalog](BDD-CATALOG.md)** - Behavior specifications

### Development & API
- **[API Reference](API-REFERENCE.md)** - Go package API
- **[Architecture](ARCHITECTURE.md)** - Component structure
- **[BDD Catalog](BDD-CATALOG.md)** - Test scenarios

### Accessibility
- **[Accessibility](ACCESSIBILITY.md)** - WCAG compliance
- **[Quick Reference](AGM-QUICK-REFERENCE.md)** - Accessibility flags
- **[Command Reference](AGM-COMMAND-REFERENCE.md)** - Global flags

---

## Quick Links

### Essential Commands
```bash
# Create session
agm new --harness claude-code my-session

# Resume session
agm resume my-session

# List sessions
agm list

# Health check
agm doctor --validate
```

**Full reference**: [Command Reference](AGM-COMMAND-REFERENCE.md)

### Configuration Files
- `~/.config/agm/config.yaml` - User configuration
- `~/sessions/` - Session storage (unified)
- `~/.claude-sessions/` - Legacy session storage
- `~/.agm/logs/messages/` - Message logs

**Details**: [Architecture](ARCHITECTURE.md#storage-architecture)

### Environment Variables
```bash
ANTHROPIC_API_KEY=...   # Claude
GEMINI_API_KEY=...      # Gemini
OPENAI_API_KEY=...      # GPT
AGM_DEBUG=true          # Debug mode
```

**Full list**: [API Reference](API-REFERENCE.md#environment-variables)

---

## Complete Documentation List

### Quick Start (5-10 minutes)
1. **AGM-QUICK-REFERENCE.md** - One-page cheat sheet with essential commands
2. **GETTING-STARTED.md** - Installation, setup, and first session (10 minutes)

### Core Guides
3. **AGM-COMMAND-REFERENCE.md** - Complete CLI reference (all commands, flags, examples)
4. **USER-GUIDE.md** - Comprehensive usage guide (workflows, best practices)
5. **EXAMPLES.md** - Real-world scenarios (30+ examples across 7 categories)
6. **ARCHITECTURE.md** - System architecture (components, data flow, design)
7. **API-REFERENCE.md** - Developer API documentation (Go packages, interfaces)

### Specialized Guides
8. **AGENT-COMPARISON.md** - Agent selection guide (Claude vs Gemini vs GPT)
9. **AGM-MIGRATION-GUIDE.md** - AGM to AGM migration (validation, rollback)
10. **TROUBLESHOOTING.md** - Common issues and solutions (detailed diagnostics)
11. **FAQ.md** - Frequently asked questions (quick answers)

### Advanced Topics
12. **BDD-CATALOG.md** - Living documentation (8 feature files, 20+ scenarios)
13. **ACCESSIBILITY.md** - WCAG compliance (screen readers, high contrast)
14. **COMMAND-TRANSLATION-DESIGN.md** - Multi-agent abstraction design
15. **SESSION_LIFECYCLE_TESTS.md** - Session lifecycle testing documentation
16. **CLI-REFERENCE.md** - Extended CLI reference (additional details)

### Specialized Documentation
17. **agm-environment-management-spec.md** - Environment management specification
18. **gemini-readiness-detection.md** - Gemini agent readiness detection
19. **engram-integration.md** - Engram integration guide
20. **UX_PATTERNS.md** - User experience patterns
21. **UX-ACCESSIBILITY-REVIEW.md** - UX accessibility review
22. **UX-SPRINT1-REVIEW.md** - UX sprint 1 review
23. **ux-style-guide.md** - UX style guide
24. **performance-benchmarks.md** - Performance benchmarks
25. **unified-storage-migration.md** - Unified storage migration spec
26. **tmux-lock-refactoring.md** - Tmux lock refactoring documentation
27. **lock-improvements.md** - Lock improvements documentation
28. **deep-research-e2e-test-plan.md** - Deep research E2E test plan

### Agent-Specific
29. **AGENTS.md.example** - Example AGENTS.md configuration

---

## Learning Paths

### Path 1: Quick Start (10 minutes)
1. Read [Quick Reference](AGM-QUICK-REFERENCE.md) (2 min)
2. Install AGM: [Getting Started](GETTING-STARTED.md#installation) (3 min)
3. Create first session: [Getting Started](GETTING-STARTED.md#first-steps) (5 min)
4. Try essential commands: [Quick Reference](AGM-QUICK-REFERENCE.md#essential-commands)

### Path 2: Effective Usage (30 minutes)
1. Quick Start (10 min)
2. Choose an agent: [Agent Comparison](AGENT-COMPARISON.md) (5 min)
3. Review workflows: [Examples](EXAMPLES.md#daily-workflows) (10 min)
4. Learn advanced commands: [Command Reference](AGM-COMMAND-REFERENCE.md) (5 min)

### Path 3: Power User (1 hour)
1. Effective Usage (30 min)
2. Read [User Guide](USER-GUIDE.md) (15 min)
3. Study advanced scenarios: [Examples](EXAMPLES.md#advanced-scenarios) (10 min)
4. Configure settings: [Command Reference](AGM-COMMAND-REFERENCE.md#configuration-file) (5 min)

### Path 4: Developer (2 hours)
1. Effective Usage (30 min)
2. Understand architecture: [Architecture](ARCHITECTURE.md) (30 min)
3. Study API: [API Reference](API-REFERENCE.md) (30 min)
4. Review tests: [BDD Catalog](BDD-CATALOG.md) (30 min)

### Path 5: Contributor (3 hours)
1. Developer Path (2 hours)
2. Deep dive into architecture: [Architecture](ARCHITECTURE.md) (complete read)
3. Study command translation: [Command Translation](COMMAND-TRANSLATION-DESIGN.md)
4. Review all test scenarios: [BDD Catalog](BDD-CATALOG.md)
5. Understand accessibility: [Accessibility](ACCESSIBILITY.md)

---

## Document Relationships

```
Quick Reference ──────────────┬─────> Getting Started ───> User Guide ───> Examples
                              │
                              └─────> Command Reference ──> API Reference

Migration Guide ──> Troubleshooting ──> FAQ

Agent Comparison ──> Examples (Multi-Agent sections)

Architecture ──> API Reference ──> Command Translation Design

BDD Catalog ──> SESSION_LIFECYCLE_TESTS ──> Troubleshooting

Accessibility ──> Command Reference (Global Flags)
```

---

## Documentation Standards

### Code Examples
All code examples are:
- **Tested**: Examples verified to work
- **Complete**: Include necessary imports and error handling
- **Commented**: Explain non-obvious parts
- **Copy-paste ready**: Can be run as-is

### Command Examples
All command examples show:
- **Input**: Full command with flags
- **Output**: Expected output (when relevant)
- **Context**: When to use the command
- **Alternatives**: Related commands

### Cross-References
- **Internal links**: Use relative links to other docs
- **Section links**: Deep links to specific sections
- **Related docs**: "See also" sections at end of docs

---

## Version Information

### Current Version: 3.0

**Major features**:
- Multi-agent support (Claude, Gemini, GPT)
- Command translation layer
- Unified session storage
- Message logging system
- Workflow automation (experimental)

**Breaking changes from v2**:
- Agent field required in manifest
- Session storage location (migration required)

**Migration**: See [Migration Guide](AGM-MIGRATION-GUIDE.md)

### Documentation Coverage

- ✅ **Installation & Setup**: Complete
- ✅ **Basic Usage**: Complete
- ✅ **Advanced Features**: Complete
- ✅ **Multi-Agent Workflows**: Complete
- ✅ **Architecture**: Complete
- ✅ **API Reference**: Complete
- ✅ **Troubleshooting**: Complete
- ✅ **Accessibility**: Complete
- ⚠️ **Workflows**: Experimental (deep-research only)
- 🔜 **Cloud Sync**: Planned (v3.1)

---

## Getting Help

### Documentation Issues
- **Missing information**: File issue on GitHub
- **Unclear explanation**: Request clarification
- **Example needed**: Suggest specific scenario

### Technical Support
1. Check [Troubleshooting](TROUBLESHOOTING.md)
2. Review [FAQ](FAQ.md)
3. Run `agm doctor --validate`
4. File issue on GitHub with diagnostics

### Community
- **GitHub Issues**: Bug reports and feature requests
- **Discussions**: Questions and ideas
- **Pull Requests**: Documentation improvements welcome

---

## Contributing to Documentation

### Documentation Structure
```
docs/
├── INDEX.md                    # This file (navigation hub)
├── AGM-QUICK-REFERENCE.md      # One-page cheat sheet
├── GETTING-STARTED.md          # Getting started guide
├── AGM-COMMAND-REFERENCE.md    # Complete CLI reference
├── USER-GUIDE.md               # Comprehensive usage guide
├── EXAMPLES.md                 # Real-world examples
├── ARCHITECTURE.md             # System architecture
├── API-REFERENCE.md            # Developer API
├── AGENT-COMPARISON.md         # Agent selection guide
├── AGM-MIGRATION-GUIDE.md      # Migration guide
├── TROUBLESHOOTING.md          # Troubleshooting
├── FAQ.md                      # FAQ
├── BDD-CATALOG.md              # BDD scenarios
├── ACCESSIBILITY.md            # Accessibility
└── ... (specialized docs)
```

### Writing Guidelines
1. **Audience**: Define target reader (end user, developer, contributor)
2. **Structure**: Use consistent heading hierarchy
3. **Examples**: Include runnable code/commands
4. **Links**: Cross-reference related documentation
5. **Updates**: Keep version and date current

### Style Guide
- **Tone**: Clear, concise, helpful
- **Voice**: Active voice preferred
- **Formatting**: Use code blocks, tables, lists
- **Clarity**: Explain "why" not just "what"

---

## Changelog

### 2026-02-04
- ✅ Created comprehensive documentation suite
- ✅ Added ARCHITECTURE.md (complete system architecture)
- ✅ Added API-REFERENCE.md (developer API documentation)
- ✅ Added INDEX.md (this file - navigation hub)
- ✅ Updated existing docs for v3.0 consistency

### 2026-02-03
- Updated AGM-COMMAND-REFERENCE.md with new commands
- Updated USER-GUIDE.md with multi-agent workflows
- Updated EXAMPLES.md with 30+ scenarios

### 2026-02-01
- Initial AGM v3.0 documentation
- Migrated from AGM documentation
- Added multi-agent support documentation

---

## Future Documentation Plans

### Planned (v3.1)
- [ ] Workflow automation guide (deep-research, code-review, architect)
- [ ] Cloud sync documentation
- [ ] Advanced configuration guide
- [ ] Performance tuning guide

### Planned (v4.0)
- [ ] Web UI documentation
- [ ] MCP integration guide
- [ ] Plugin development guide
- [ ] Advanced agent customization

---

**Maintained by**: Foundation Engineering
**Last Updated**: 2026-02-04
**AGM Version**: 3.0
