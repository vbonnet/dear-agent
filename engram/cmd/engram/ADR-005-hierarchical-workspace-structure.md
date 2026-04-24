# ADR-005: Hierarchical Workspace Structure with Four Tiers

**Status**: Accepted

**Date**: 2024-02-05

**Context**: Engrams can come from multiple sources with different trust levels and visibility scopes:
- Individual user patterns (private)
- Team-shared patterns (team-level)
- Company-wide patterns (organization-level)
- Engram core patterns (public, curated)

Requirements:
- Separation of concerns (user vs team vs company vs core)
- Override capability (user can override company patterns)
- Security (team patterns shouldn't leak to public)
- Discoverability (search across relevant tiers)

**Decision**: Implement a four-tier hierarchical workspace structure:

```
~/.engram/
├── user/       # Tier 1: User-specific engrams (highest priority)
├── team/       # Tier 2: Team-shared engrams
├── company/    # Tier 3: Company-wide engrams
└── core/       # Tier 4: Engram core patterns (lowest priority)
```

**Tier Characteristics**:

| Tier | Priority | Visibility | Mutability | Example Use Case |
|------|----------|------------|------------|------------------|
| User | 1 (highest) | User only | Read/Write | Personal patterns, overrides |
| Team | 2 | Team members | Read/Write (team) | Team conventions, workflows |
| Company | 3 | Organization | Read/Write (admins) | Company standards, policies |
| Core | 4 (lowest) | Public | Read-only | Curated patterns, best practices |

**Priority Rules**:
1. Higher tier overrides lower tier (same engram name)
2. User can override company/core patterns
3. Team can override company/core patterns
4. Search includes all relevant tiers

**Directory Structure**:
```
~/.engram/
├── user/
│   ├── config.yaml              # User config
│   ├── patterns/
│   │   └── my-pattern.ai.md
│   └── learnings/
│       └── session-123.ai.md
├── team/                        # Symlink or mount
│   └── patterns/
│       └── team-pattern.ai.md
├── company/                     # Symlink or mount
│   └── standards/
│       └── company-std.ai.md
└── core -> /path/to/engram      # Symlink to repository
    ├── engrams/
    │   ├── patterns/
    │   ├── strategies/
    │   └── principles/
    └── plugins/
```

**Rationale**:

1. **Separation**: Clear boundaries between user/team/company/core
2. **Override**: User can customize any pattern locally
3. **Discovery**: `engram retrieve` searches all tiers
4. **Security**: Team/company patterns in separate directories
5. **Sync**: Team/company can be symlinks to shared storage (NFS, git)
6. **Simplicity**: Flat hierarchy within each tier

**Initialization Strategy**:

```go
// engram init creates:
os.MkdirAll("~/.engram/user")     // Always created
os.MkdirAll("~/.engram/logs")     // Always created
os.MkdirAll("~/.engram/cache")    // Always created
os.Symlink(detectRepo(), "~/.engram/core")  // Symlink to repo

// team/ and company/ created manually or by admin:
ln -s /mnt/team-engrams ~/.engram/team
ln -s /mnt/company-engrams ~/.engram/company
```

**Index Management**:

Each tier has its own index:
- `~/.engram/cache/index-user.json`
- `~/.engram/cache/index-team.json`
- `~/.engram/cache/index-company.json`
- `~/.engram/cache/index-core.json`

`engram index rebuild` rebuilds all tiers or specific tier via `--tier` flag.

**Search Order**:

When retrieving engrams:
1. Search all tiers in parallel (fast filter)
2. Merge results with priority (user > team > company > core)
3. Rank merged results (API ranking)
4. Return top N (budget limiting)

**Alternatives Considered**:

1. **Single directory**: No separation, security risk
2. **User-only**: No team/company sharing
3. **Git-based**: Operational complexity, merge conflicts
4. **Database**: Unnecessary complexity for file-based patterns

**Consequences**:

**Positive**:
- Clear separation of user/team/company/core
- User can override any pattern
- Team sharing via symlinks (simple)
- Security via filesystem permissions
- Scalable (add tiers as needed)

**Negative**:
- Requires admin setup for team/company tiers
- Symlink management (mitigated by documentation)
- Index maintenance for multiple tiers

**Configuration**:

```yaml
# ~/.engram/user/config.yaml
tiers:
  enabled:
    - user
    - team      # If team directory exists
    - company   # If company directory exists
    - core
  paths:
    user: ~/.engram/user
    team: ~/.engram/team
    company: ~/.engram/company
    core: ~/.engram/core
```

**Team/Company Setup**:

**Option 1: Shared Filesystem (NFS)**
```bash
ln -s /mnt/team-engrams ~/.engram/team
```

**Option 2: Git Repository**
```bash
git clone team-engrams-repo ~/.engram/team
```

**Option 3: Cloud Sync**
```bash
ln -s ~/Dropbox/team-engrams ~/.engram/team
```

**Security Considerations**:

1. Team directory: Readable by team, writable by team leads
2. Company directory: Readable by all, writable by admins
3. Core directory: Readable by all, read-only (symlink to repo)
4. User directory: Readable/writable by user only

**Implementation Notes**:

- `engram retrieve` defaults to searching all tiers
- `engram index rebuild --tier=user` rebuilds specific tier
- `engram index rebuild` rebuilds all tiers
- Missing tiers (team/company) silently skipped
- Core tier detection via `detectEngramRepo()`

**Related Decisions**:
- ADR-004: Three-Tier Retrieval System
- ADR-008: Plugin Architecture
