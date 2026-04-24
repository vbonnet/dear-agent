# ADR-008: Plugin Architecture with Three Patterns

**Status**: Accepted

**Date**: 2024-02-20

**Context**: Engram needs extensibility for:
- Custom AI agent guidance (project-specific patterns)
- External tool integrations (API connectors)
- Workflow customization (team-specific processes)

Hard-coding all functionality limits flexibility. Different extension types have different needs:
- **Guidance plugins**: Markdown files for AI consumption
- **Tool plugins**: Executable scripts for automation
- **Connector plugins**: API integrations for external services

A single plugin model doesn't fit all use cases.

**Decision**: Implement a plugin architecture with three distinct patterns:

1. **Guidance Pattern**: Provide AI-readable guidance files
2. **Tool Pattern**: Executable commands and scripts
3. **Connector Pattern**: External service integrations

**Plugin Manifest** (`manifest.yaml`):

```yaml
name: plugin-name
version: 1.0.0
pattern: guidance|tool|connector
description: Brief description

# Tool Pattern: Commands
commands:
  - name: command-name
    script: ./scripts/command.sh
    description: Command description
    args:
      - name: arg-name
        required: true
        description: Argument description

# Connector Pattern: External APIs
connectors:
  - name: connector-name
    type: rest|graphql|grpc
    endpoint: https://api.example.com
    auth: apikey|oauth2|basic

# EventBus: Subscribe to system events
eventbus:
  subscribe:
    - session.start
    - session.end
    - engram.created

# Security: Required permissions
permissions:
  filesystem:
    - ~/.engram/user
    - /tmp
  network:
    - api.example.com
  commands:
    - git
    - curl
```

**Pattern Details**:

### 1. Guidance Pattern

**Purpose**: Provide AI-readable guidance files for context injection.

**Structure**:
```
plugin-guidance/
├── manifest.yaml
├── README.md
└── guidance/
    ├── patterns/
    │   └── custom-pattern.ai.md
    ├── strategies/
    │   └── custom-strategy.ai.md
    └── principles/
        └── custom-principle.ai.md
```

**Manifest**:
```yaml
name: team-patterns
version: 1.0.0
pattern: guidance
description: Team-specific coding patterns

guidance:
  paths:
    - ./guidance/patterns
    - ./guidance/strategies
  index: true  # Include in ecphory index
```

**Use Cases**:
- Team coding standards
- Project-specific patterns
- Custom best practices
- Domain-specific knowledge

**Integration**:
- Guidance files indexed by `engram index rebuild`
- Retrieved by `engram retrieve` along with core engrams
- Priority: Plugin guidance > User > Team > Core

### 2. Tool Pattern

**Purpose**: Executable commands and automation scripts.

**Structure**:
```
plugin-tool/
├── manifest.yaml
├── README.md
├── scripts/
│   ├── lint-check.sh
│   ├── test-runner.sh
│   └── deploy.sh
└── health-check.sh  # Optional health check
```

**Manifest**:
```yaml
name: project-tools
version: 1.0.0
pattern: tool
description: Project automation tools

commands:
  - name: lint
    script: ./scripts/lint-check.sh
    description: Run linter on codebase
    args:
      - name: path
        required: false
        default: .
        description: Path to lint

  - name: test
    script: ./scripts/test-runner.sh
    description: Run test suite

permissions:
  filesystem:
    - ~/project
  commands:
    - eslint
    - jest
```

**Use Cases**:
- Project-specific automation
- Custom linting/testing
- Deployment scripts
- Code generation

**Integration**:
- Commands registered as `engram <plugin>:<command>`
- Example: `engram project-tools:lint --path src/`
- Health checks run by `engram doctor` if `health-check.sh` exists

### 3. Connector Pattern

**Purpose**: External service integrations (APIs, databases).

**Structure**:
```
plugin-connector/
├── manifest.yaml
├── README.md
├── connectors/
│   ├── jira.sh
│   ├── slack.sh
│   └── github.sh
└── config.example.yaml
```

**Manifest**:
```yaml
name: issue-tracker
version: 1.0.0
pattern: connector
description: JIRA issue tracking integration

connectors:
  - name: jira
    type: rest
    endpoint: https://company.atlassian.net
    auth: apikey
    script: ./connectors/jira.sh

eventbus:
  subscribe:
    - session.end  # Post session summary to JIRA

permissions:
  network:
    - company.atlassian.net
  filesystem:
    - ~/.engram/user/issues
```

**Use Cases**:
- Issue tracker integration (JIRA, Linear)
- Chat integrations (Slack, Discord)
- Version control (GitHub, GitLab)
- Documentation (Confluence, Notion)

**Integration**:
- EventBus subscriptions trigger connector scripts
- Example: Session end → Post to Slack
- Credentials from environment variables

**Plugin Discovery**:

**Plugin Paths** (configured in `~/.engram/user/config.yaml`):
```yaml
plugins:
  paths:
    - ~/.engram/plugins        # User plugins
    - ~/.engram/team/plugins   # Team plugins
    - ~/.engram/core/plugins   # Core plugins
  disabled:
    - deprecated-plugin
```

**Loading Process**:
1. Scan plugin paths for directories
2. Read `manifest.yaml` from each directory
3. Validate manifest schema
4. Check permissions
5. Load enabled plugins (skip disabled)
6. Register commands/connectors

**Security Model**:

**Permissions** (manifest.yaml):
```yaml
permissions:
  filesystem:
    - ~/.engram/user         # Allowed filesystem paths
    - /tmp
  network:
    - api.example.com        # Allowed network endpoints
  commands:
    - git                    # Allowed commands to execute
    - curl
```

**Validation**:
- Plugins must declare all permissions upfront
- Requests outside permissions are denied
- User prompted to approve new permissions (future)

**Sandboxing** (future):
- Filesystem: chroot to allowed paths
- Network: Firewall rules for allowed endpoints
- Commands: Whitelist of allowed executables

**EventBus**:

Plugins can subscribe to system events:

**Events**:
- `session.start` - New AI session started
- `session.end` - Session completed
- `engram.created` - New engram created
- `engram.updated` - Engram modified
- `index.rebuilt` - Index rebuild completed

**Example** (Slack notification on session end):
```yaml
eventbus:
  subscribe:
    - session.end

# Script receives event JSON on stdin
# ./scripts/on-session-end.sh
```

**Rationale**:

1. **Pattern Separation**: Different extension types have different needs
2. **Guidance Simple**: Just markdown files, no code
3. **Tools Executable**: Shell scripts, any language
4. **Connectors Declarative**: API integration without custom code
5. **Security**: Explicit permissions, sandboxing (future)
6. **Discoverability**: `engram plugin list` shows all plugins

**Alternatives Considered**:

1. **Single plugin pattern**: Doesn't fit all use cases
2. **Script-only plugins**: No guidance/connector support
3. **Language-specific (Go only)**: Limits contributors
4. **No permissions**: Security risk
5. **No EventBus**: Limited integration

**Consequences**:

**Positive**:
- Clear separation of plugin types
- Simple guidance plugins (just markdown)
- Flexible tool plugins (any language)
- Declarative connectors (no code)
- Security via permissions
- EventBus for integration

**Negative**:
- Three patterns to document
- Permission model complexity
- EventBus implementation overhead

**Implementation Phases**:

**Phase 1 (v0.1.0)**:
- [x] Plugin loading
- [x] Manifest parsing
- [x] Guidance pattern
- [x] Tool pattern
- [ ] `engram plugin list`

**Phase 2 (v0.2.0)**:
- [ ] Connector pattern
- [ ] EventBus
- [ ] Permission validation
- [ ] Health checks

**Phase 3 (v0.3.0)**:
- [ ] Sandboxing
- [ ] Permission approval UI
- [ ] Plugin marketplace

**Example Plugins**:

**1. Team Patterns (Guidance)**:
```yaml
name: backend-patterns
pattern: guidance
guidance:
  paths:
    - ./patterns
```

**2. Lint Tools (Tool)**:
```yaml
name: code-quality
pattern: tool
commands:
  - name: lint
    script: ./scripts/lint.sh
```

**3. Slack Connector (Connector)**:
```yaml
name: slack-integration
pattern: connector
connectors:
  - name: slack
    type: rest
    endpoint: https://hooks.slack.com
eventbus:
  subscribe:
    - session.end
```

**Related Decisions**:
- ADR-005: Hierarchical Workspace Structure
- ADR-006: Security-First Input Validation
