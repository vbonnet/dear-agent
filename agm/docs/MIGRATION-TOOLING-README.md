# AGM Migration Tooling

**Purpose**: Tools and documentation to help users migrate to multi-agent support.

---

## Overview

This migration tooling provides:

1. **Validation Script** - Automated session compatibility checking
2. **Migration Guide** - Step-by-step migration instructions
3. **Troubleshooting Guide** - Solutions for common migration issues
4. **Documentation** - Comprehensive migration resources

---

## Quick Start

### 1. Validate Your Sessions

```bash
cd main/agm
./scripts/agm-migration-validate.sh
```

**Expected output if sessions are compatible:**
```
✓ All sessions are valid and compatible with AGM!
```

### 2. Read Migration Guide

```bash
# View comprehensive migration guide
cat docs/AGM-MIGRATION-GUIDE.md

# Or browse online
open https://github.com/vbonnet/dear-agent/blob/main/agm/docs/AGM-MIGRATION-GUIDE.md
```

### 3. Follow Migration Steps

See [AGM-MIGRATION-GUIDE.md](AGM-MIGRATION-GUIDE.md) for:
- Pre-migration checklist
- Migration scenarios (incremental adoption, full migration)
- Step-by-step instructions
- Validation and verification procedures
- Rollback procedures

---

## Migration Tooling Components

### 1. Validation Script

**Location**: `scripts/agm-migration-validate.sh`

**Purpose**: Validate that existing sessions are compatible with AGM multi-agent architecture.

**Features**:
- ✅ Manifest structure validation (valid YAML)
- ✅ Required fields check (session_id, name, created_at, etc.)
- ✅ Schema version compatibility (v2.0 supported)
- ✅ UUID format validation
- ✅ Agent field compatibility check
- ✅ Batch validation (all sessions)
- ✅ Single session validation

**Usage**:
```bash
# Validate all sessions
./scripts/agm-migration-validate.sh

# Validate specific session
./scripts/agm-migration-validate.sh my-session

# Show help
./scripts/agm-migration-validate.sh --help
```

**Output**:
- Summary of valid/invalid sessions
- Detailed error messages for issues
- Fix recommendations

---

### 2. Migration Guide

**Location**: `docs/AGM-MIGRATION-GUIDE.md`

**Purpose**: Comprehensive guide for version migration.

**Sections**:
1. **Overview** - Migration impact, requirements
2. **Pre-Migration Checklist** - Validation steps
3. **Migration Scenarios**:
   - Scenario 1: Keep using Claude (no changes)
   - Scenario 2: Try other agents (incremental)
   - Scenario 3: Full multi-agent adoption
4. **Step-by-Step Migration** - Detailed instructions
5. **Validation and Verification** - Success criteria
6. **Rollback Procedures** - Recovery options
7. **Troubleshooting** - Common issues
8. **FAQs** - Frequently asked questions

**Target Audience**:
- Users transitioning to multi-agent workflows
- Teams adopting AGM

---

### 3. Troubleshooting Guide

**Location**: `docs/MIGRATION-TROUBLESHOOTING.md`

**Purpose**: Detailed solutions for migration issues.

**Coverage**:
- **Validation Errors**: Invalid YAML, missing fields, UUID format issues
- **Agent Configuration**: API key setup, agent availability
- **Session Resume Issues**: UUID mismatch, lock issues
- **Migration Script Issues**: Directory not found, Python dependency
- **Performance Issues**: Slow validation, large session counts
- **Advanced Debugging**: Manifest inspection, log analysis
- **Recovery Procedures**: Complete rollback, partial recovery

**Format**: Problem-Solution with code examples.

---

### 4. Documentation Structure

```
agm/
├── scripts/
│   └── agm-migration-validate.sh       # Validation script
├── docs/
│   ├── AGM-MIGRATION-GUIDE.md          # Main migration guide
│   ├── MIGRATION-TROUBLESHOOTING.md    # Detailed troubleshooting
│   ├── MIGRATION-TOOLING-README.md     # This file
│   ├── MIGRATION-CLAUDE-MULTI.md       # Conceptual migration guide
│   └── MIGRATION-V2-V3.md              # Future schema migration
└── README.md                            # Links to migration docs
```

---

## Migration Scenarios

### Scenario 1: No Changes Needed (Backward Compatible)

**Who**: Users happy with Claude, don't need other agents.

**Migration Steps**: None. Continue using existing workflow.

```bash
agm list                    # Works as before
agm resume my-session       # Resumes Claude sessions
agm new coding-work         # Creates Claude session (default)
```

---

### Scenario 2: Incremental Adoption

**Who**: Users wanting to try Gemini or GPT for specific tasks.

**Migration Steps**:

1. Configure API keys for desired agents
2. Create new sessions with specific agents
3. Existing Claude sessions remain unchanged

```bash
# Configure Gemini API key
export GOOGLE_API_KEY=your-key

# Create Gemini session for research
agm new --harness gemini-cli research-project

# Existing Claude sessions unaffected
agm list  # Shows mix of Claude and Gemini sessions
```

---

### Scenario 3: Full Multi-Agent Adoption

**Who**: Teams wanting to use different agents for different tasks.

**Migration Steps**:

1. Configure all agent API keys
2. Define agent selection conventions (code→Claude, research→Gemini)
3. Start creating sessions with explicit agent selection
4. Archive old sessions over time

```bash
# Configure all agents
export ANTHROPIC_API_KEY=...
export GOOGLE_API_KEY=...
export OPENAI_API_KEY=...

# Use agent-specific workflows
agm new --harness claude-code backend-api
agm new --harness gemini-cli market-analysis
agm new --harness codex-cli brainstorming
```

---

## Validation Checks

The validation script performs these checks:

### 1. Manifest Structure

**Check**: YAML parseable, no syntax errors.

**Common failures**:
- Indentation errors
- Missing quotes for special characters
- Tab characters (YAML requires spaces)

**Fix**: See MIGRATION-TROUBLESHOOTING.md section 1.1

---

### 2. Required Fields

**Check**: session_id, name, created_at, schema_version present.

**Common failures**:
- Empty fields
- Missing fields from incomplete migration
- Corrupted manifests

**Fix**: See MIGRATION-TROUBLESHOOTING.md section 1.2

---

### 3. Schema Version

**Check**: schema_version is "2.0" or "3.0".

**Common failures**:
- Legacy v1.0 schemas
- Missing schema_version field

**Fix**: See MIGRATION-TROUBLESHOOTING.md section 1.3

---

### 4. UUID Format

**Check**: session_id follows UUID format (8-4-4-4-12 hex).

**Common failures**:
- Legacy `session-<name>` format
- Manually created invalid UUIDs

**Fix**: See MIGRATION-TROUBLESHOOTING.md section 1.4

---

### 5. Agent Field (Informational)

**Check**: agent field present (optional in v2).

**Note**: Not an error for v2 manifests. Sessions without agent field default to Claude.

**Recommendation**: Add agent field for clarity in multi-agent environments.

---

## Rollback Procedures

### Full Rollback

**When**: Critical migration issues affecting all sessions.

**Steps**:
```bash
# 1. Stop all sessions
agm list | xargs -I {} agm kill {}

# 2. Restore from backup
rm -rf ~/.claude-sessions
mv ~/.claude-sessions.backup ~/.claude-sessions

# 3. Verify
agm list
```

**Recovery time**: < 5 minutes

---

### Partial Rollback

**When**: Only specific sessions affected.

**Steps**:
```bash
# Restore specific session
cp -r ~/.claude-sessions.backup/my-session \
      ~/.claude-sessions/my-session

# Verify
./scripts/agm-migration-validate.sh my-session
```

**Recovery time**: < 1 minute per session

---

---

## Testing

### Test Validation Script

```bash
# Run on all sessions
./scripts/agm-migration-validate.sh

# Test specific session
./scripts/agm-migration-validate.sh test-session

# Expected: No errors for valid sessions
```

---

### Test Migration

```bash
# 1. Create test session
agm new test-migration --harness claude-code

# 2. Validate
./scripts/agm-migration-validate.sh test-migration

# 3. Test agent switching
agm archive test-migration
agm new test-migration --harness gemini-cli

# 4. Cleanup
agm archive test-migration
```

---

## Best Practices

### 1. Backup Before Migration

```bash
# Create dated backup
cp -r ~/.claude-sessions \
      ~/.claude-sessions-backup-$(date +%Y%m%d)

# Verify backup
ls -lah ~/.claude-sessions-backup-*
```

---

### 2. Validate Before and After

```bash
# Before migration
./scripts/agm-migration-validate.sh > pre-migration.txt

# After migration
./scripts/agm-migration-validate.sh > post-migration.txt

# Compare
diff pre-migration.txt post-migration.txt
```

---

### 3. Test on Non-Critical Sessions First

```bash
# Identify test candidates
agm list --archived | head -3

# Restore and test
agm unarchive old-test-session
agm resume old-test-session  # Verify it works
```

---

### 4. Document Team Conventions

```markdown
# Team AGM Agent Conventions

- **Code review, debugging**: Use Claude
- **Research, documentation**: Use Gemini
- **Brainstorming, quick Q&A**: Use GPT

## Session Naming

- `code-*`: Claude sessions
- `research-*`: Gemini sessions
- `brainstorm-*`: GPT sessions
```

---

## Metrics and Monitoring

### Validation Metrics

Track these metrics during migration:

```bash
# Before migration
./scripts/agm-migration-validate.sh | tee validation-before.txt

# Extract metrics
TOTAL=$(grep "Total sessions:" validation-before.txt | awk '{print $3}')
VALID=$(grep "Valid sessions:" validation-before.txt | awk '{print $3}')
INVALID=$(grep "Invalid sessions:" validation-before.txt | awk '{print $3}')

echo "Validation success rate: $(echo "scale=2; $VALID / $TOTAL * 100" | bc)%"
```

---

### Migration Success Rate

```bash
# Sessions migrated successfully
MIGRATED_SUCCESS=$(agm list --all | grep -E "(claude|gemini|gpt)" | wc -l)

# Total sessions
TOTAL_SESSIONS=$(agm list --all | wc -l)

# Success rate
echo "Migration success: $(echo "scale=2; $MIGRATED_SUCCESS / $TOTAL_SESSIONS * 100" | bc)%"
```

---

## Support and Resources

### Documentation

- **[AGM-MIGRATION-GUIDE.md](AGM-MIGRATION-GUIDE.md)** - Main migration guide
- **[MIGRATION-TROUBLESHOOTING.md](MIGRATION-TROUBLESHOOTING.md)** - Troubleshooting
- **[AGENT-COMPARISON.md](AGENT-COMPARISON.md)** - Agent selection guide
- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - General AGM troubleshooting

### Commands

- **[AGM-COMMAND-REFERENCE.md](AGM-COMMAND-REFERENCE.md)** - Complete command reference
- **[AGM-QUICK-REFERENCE.md](AGM-QUICK-REFERENCE.md)** - Quick command cheat sheet

### Getting Help

1. **Run diagnostics**:
   ```bash
   ./scripts/agm-migration-validate.sh
   agm doctor --validate
   ```

2. **Check documentation**:
   - Migration guide for procedures
   - Troubleshooting guide for issues
   - FAQ for common questions

3. **File issue**:
   - URL: https://github.com/vbonnet/dear-agent/issues
   - Include: validation output, error logs, manifest sample

---

## Changelog

### Version 1.0 (2026-02-04)

**Initial Release**

- ✅ Validation script (`agm-migration-validate.sh`)
- ✅ Migration guide (`AGM-MIGRATION-GUIDE.md`)
- ✅ Troubleshooting guide (`MIGRATION-TROUBLESHOOTING.md`)
- ✅ Documentation structure

**Features**:
- Automated session validation
- Step-by-step migration instructions
- Rollback procedures
- Comprehensive troubleshooting

**Tested on**:
- AGM v3.0+
- Schema version 2.0
- Linux and macOS

---

**Migration Tooling Version**: 1.0
**Last Updated**: 2026-02-04
**Maintainer**: AGM Team
**License**: MIT
