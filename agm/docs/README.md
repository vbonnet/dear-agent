# AGM Documentation

Welcome to the AGM (AI Agent Manager) documentation.

## Quick Links

### Getting Started
- **[BUILD-AND-VERIFY.md](./BUILD-AND-VERIFY.md)** - Build AGM and verify the archive fix (15 min)
- **[IMPLEMENTATION-SUMMARY.md](./IMPLEMENTATION-SUMMARY.md)** - Executive summary of recent changes

### For Developers
- **[ARCHIVE-DOLT-MIGRATION.md](./ARCHIVE-DOLT-MIGRATION.md)** - Complete technical guide to the Dolt migration
- **[testing/README.md](./testing/README.md)** - Testing overview and best practices

### For QA/Testers
- **[testing/ARCHIVE-DOLT-RUNBOOK.md](./testing/ARCHIVE-DOLT-RUNBOOK.md)** - Manual testing procedures

## Recent Changes (2026-03-12)

### ✅ Archive Command Dolt Migration

Fixed bug where STOPPED sessions couldn't be archived ("session not found" error).

**What Changed**:
- Migrated `agm session archive` from filesystem to Dolt database
- Added `ResolveIdentifier()` method to Dolt adapter
- Comprehensive test coverage (unit + integration)
- Complete documentation package

**Quick Start**:
1. Read: [IMPLEMENTATION-SUMMARY.md](./IMPLEMENTATION-SUMMARY.md)
2. Build: [BUILD-AND-VERIFY.md](./BUILD-AND-VERIFY.md)
3. Test: [testing/ARCHIVE-DOLT-RUNBOOK.md](./testing/ARCHIVE-DOLT-RUNBOOK.md)

## Documentation Structure

```
docs/
├── README.md                          # This file - documentation overview
├── IMPLEMENTATION-SUMMARY.md          # Executive summary of archive fix
├── ARCHIVE-DOLT-MIGRATION.md          # Complete technical guide
├── BUILD-AND-VERIFY.md                # Build and verification guide
└── testing/
    ├── README.md                      # Testing overview
    └── ARCHIVE-DOLT-RUNBOOK.md        # Manual testing runbook
```

## Document Purpose Guide

### When to Use Each Document

| If you want to... | Read this... |
|-------------------|--------------|
| Understand what was fixed | [IMPLEMENTATION-SUMMARY.md](./IMPLEMENTATION-SUMMARY.md) |
| Build and verify the fix | [BUILD-AND-VERIFY.md](./BUILD-AND-VERIFY.md) |
| Understand technical details | [ARCHIVE-DOLT-MIGRATION.md](./ARCHIVE-DOLT-MIGRATION.md) |
| Run manual tests | [testing/ARCHIVE-DOLT-RUNBOOK.md](./testing/ARCHIVE-DOLT-RUNBOOK.md) |
| Write/run automated tests | [testing/README.md](./testing/README.md) |

## Quick Commands

### Build & Test
```bash
# Build AGM
cd main/agm
go build -o ~/go/bin/agm ./cmd/agm

# Run unit tests
go test ./internal/dolt/... -v

# Run integration tests
DOLT_TEST_INTEGRATION=1 go test ./test/integration/lifecycle/... -tags=integration

# Run linter
golangci-lint run ./...
```

### Usage
```bash
# List sessions
agm session list

# Archive session (by ID, tmux name, or manifest name)
agm session archive <identifier>

# List all sessions (including archived)
agm session list --all

# Verify in Dolt
dolt sql -q "SELECT id, name, status FROM agm_sessions"
```

## Support

### Getting Help
1. Check documentation in this directory
2. Review testing runbooks
3. File issue in GitHub repository
4. Open a GitHub issue

### Contributing
- Follow test-driven development (TDD)
- Write tests first, then implementation
- Document all changes
- Run linter before committing

### Reporting Issues
Include in your report:
- AGM version: `agm version`
- Dolt version: `dolt version`
- Workspace: `echo $WORKSPACE`
- Error message and full command
- Steps to reproduce

## Related Documentation

### External
- **Dolt Documentation**: https://docs.dolthub.com/
- **Go Testing**: https://golang.org/pkg/testing/
- **AGM GitHub**: https://github.com/vbonnet/dear-agent

### Internal
- **CONTRIBUTING.md**: Contribution guidelines (in repo root)
- **CHANGELOG.md**: Version history
- **README.md**: Project overview (in repo root)

---

**Last Updated**: 2026-03-12
**Version**: 1.0.0
