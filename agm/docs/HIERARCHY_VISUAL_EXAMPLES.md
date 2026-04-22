# Hierarchy Display Visual Examples

## Example 1: Simple Parent-Child Hierarchy

```
Sessions Overview (3 total)

ACTIVE (2)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
● parent-project (2 children)  claude  ~/projects/main       5m ago
│  ├─ ◐ feature-auth [d:1]  claude  ~/projects/main       2h ago
│  └─ ◐ feature-api [d:1]   claude  ~/projects/main       1h ago

STOPPED (1)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
○ old-experiment  claude  ~/experiments/test           3d ago

Status: 2 active, 1 stopped
💡 Resume: agm resume <name>  |  Stop: agm stop <name>  |  Clean: agm clean
```

## Example 2: Deep Hierarchy (3 Levels)

```
Sessions Overview (5 total)

ACTIVE (5)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
● project-root (2 children)    claude  ~/work/monorepo      now
│  ├─ ◐ backend-api (2 children)  claude  ~/work/monorepo      15m ago
│  │  ├─ ○ auth-service [d:2]  claude  ~/work/monorepo/api  3h ago
│  │  └─ ● database-migration [d:2]  claude  ~/work/monorepo/api  5m ago
│  └─ ◐ frontend-ui [d:1]  claude  ~/work/monorepo/web  30m ago

Status: 5 active
💡 Resume: agm resume <name>  |  Stop: agm stop <name>  |  Clean: agm clean
```

## Example 3: Multiple Independent Roots

```
Sessions Overview (6 total)

ACTIVE (4)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
● project-alpha (1 children)  claude  ~/alpha      2h ago
│  └─ ◐ task-refactor [d:1]  claude  ~/alpha      45m ago

● project-beta  claude  ~/beta       10m ago

STOPPED (2)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
○ project-gamma (1 children)  claude  ~/gamma      2d ago
│  └─ ○ bugfix-123 [d:1]  claude  ~/gamma      1d ago

Status: 4 active, 2 stopped
💡 Resume: agm resume <name>  |  Stop: agm stop <name>  |  Clean: agm clean
```

## Example 4: Compact Layout (80-99 columns)

```
Sessions Overview (4 total)

ACTIVE (3)                                         Last Activity
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
● main-project (2)         ~/work/project      5m ago
│  ├─ ◐ task-a [d:1]       ~/work/project      1h ago
│  └─ ○ task-b [d:1]       ~/work/project      3h ago

STOPPED (1)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
○ test-session  ~/test              2d ago

Status: 3 active, 1 stopped
💡 Resume: agm resume <name>  |  Stop: agm stop <name>
```

## Example 5: Minimal Layout (60-79 columns)

```
Sessions Overview (3 total)

ACTIVE (2)                    Activity
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
● parent (2)  claude  now
│  ├─ ◐ child-a [d:1]  claude  2h ago
│  └─ ○ child-b [d:1]  claude  1d ago

Status: 2 active
💡 Resume: agm resume <name>
```

## Example 6: With TMUX Column (when NAME != TMUX)

```
Sessions Overview (3 total)

ACTIVE (3)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
● my-session (1 children)  tmux-alpha  claude  ~/project      5m ago
│  └─ ◐ child-task [d:1]  tmux-beta   claude  ~/project      1h ago
● other-session  tmux-gamma  claude  ~/other        2h ago

Status: 3 active
💡 Resume: agm resume <name>  |  Stop: agm stop <name>  |  Clean: agm clean
```

## Visual Legend

### Status Symbols
- `●` - Active session with attached clients
- `◐` - Active session without attached clients (detached)
- `○` - Stopped session
- `⊗` - Stale session (stopped for 7+ days)

### Tree Characters
- `├─` - Non-last child connector
- `└─` - Last child connector
- `│` - Vertical line continuation
- (space) - No continuation after last child

### Depth Indicators
- `[d:1]` - First level child
- `[d:2]` - Second level child (grandchild)
- `[d:3]` - Third level child (great-grandchild)
- etc.

### Children Count
- `(2 children)` - Full width display
- `(2)` - Compact display

## Comparison with Flat View

### Flat View (default)
```
Sessions Overview (5 total)

ACTIVE (5)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
● project-root       claude  ~/work/monorepo      now
◐ backend-api        claude  ~/work/monorepo      15m ago
○ auth-service       claude  ~/work/monorepo/api  3h ago
● database-migration claude  ~/work/monorepo/api  5m ago
◐ frontend-ui        claude  ~/work/monorepo/web  30m ago
```

### Hierarchy View (--hierarchy flag)
```
Sessions Overview (5 total)

ACTIVE (5)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
● project-root (2 children)    claude  ~/work/monorepo      now
│  ├─ ◐ backend-api (2 children)  claude  ~/work/monorepo      15m ago
│  │  ├─ ○ auth-service [d:2]  claude  ~/work/monorepo/api  3h ago
│  │  └─ ● database-migration [d:2]  claude  ~/work/monorepo/api  5m ago
│  └─ ◐ frontend-ui [d:1]  claude  ~/work/monorepo/web  30m ago
```

The hierarchy view clearly shows:
1. Parent-child relationships
2. Nesting levels
3. Which sessions belong together
4. Number of descendants for each parent

## Usage

```bash
# Standard flat list
agm list

# Hierarchical tree view
agm list --hierarchy

# Hierarchy with all sessions (including archived)
agm list --hierarchy --all

# JSON output (not hierarchical, just data)
agm list --json
```

## Notes

1. The hierarchy view is only available when the database contains parent_session_id data
2. Sessions must be created with `agm session create-child` to establish relationships
3. If database is not available, falls back to flat view with a warning
4. Tree rendering adapts to terminal width (minimal/compact/full)
5. Status colors remain consistent with flat view (green/yellow/dim)
