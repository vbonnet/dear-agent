# Security Audit - Diagram-as-Code Skills

**Date**: 2026-03-17
**Status**: ✗ **3 Critical, 2 High vulnerabilities found**
**Action Required**: Apply Phase 1 fixes immediately

---

## Quick Summary

### What Was Audited?
All 4 diagram-as-code skills in the spec-review-marketplace plugin:
- `create-diagrams` - Generate C4 diagrams from codebase
- `render-diagrams` - Render diagram files to images
- `review-diagrams` - Multi-persona diagram quality review
- `diagram-sync` - Detect diagram-code drift

### What We Found

**Good News** ✓:
- No command injection vulnerabilities (proper subprocess usage)
- Timeouts configured (30s validation, 300s rendering)
- YAML loaded safely (`yaml.safe_load()`)
- Format validation via enums

**Bad News** ✗:
- **3 Critical vulnerabilities** - Path traversal and symlink attacks
- **2 High vulnerabilities** - DoS via file size, output path escape
- **2 Medium vulnerabilities** - Binary PATH manipulation, codebase size limits

### Impact
- **Attackers can read arbitrary files** on the system (e.g., `/etc/passwd`)
- **Attackers can write files anywhere** (e.g., overwrite system configs)
- **Attackers can crash the service** (memory exhaustion via huge files)

---

## Files in This Directory

### 1. SECURITY-AUDIT-REPORT.md
**Full security audit** with detailed vulnerability analysis:
- 4 threat categories assessed
- 12 code locations reviewed per skill
- Severity ratings (CVSS scores)
- Remediation recommendations

**Read this for**: Complete vulnerability details and remediation steps

### 2. SECURITY-FIXES-GUIDE.md
**Implementation guide** with code examples:
- Vulnerable vs. secure coding patterns
- Step-by-step fixes for each skill
- Testing procedures
- Deployment checklist

**Read this for**: How to fix the vulnerabilities

### 3. test_security.py
**Automated test suite** (14 tests):
- Demonstrates vulnerabilities
- Provides secure implementations
- Verifies fixes work correctly

**Run this to**: Test that fixes are applied correctly

```bash
python3 -m pytest plugins/spec-review-marketplace/tests/test_security.py -v
```

---

## Quick Start: Fix the Vulnerabilities

### Step 1: Read the Audit Report
```bash
cat plugins/spec-review-marketplace/tests/SECURITY-AUDIT-REPORT.md
```

### Step 2: Apply Fixes
```bash
# Follow the implementation guide
cat plugins/spec-review-marketplace/tests/SECURITY-FIXES-GUIDE.md
```

### Step 3: Run Tests
```bash
# Verify fixes work
python3 -m pytest plugins/spec-review-marketplace/tests/test_security.py -v

# Expected: 14 passed
```

### Step 4: Manual Security Testing
```bash
# Test path traversal is blocked
create-diagrams --codebase ../../../etc --output /tmp/out
# Expected: Error - path outside allowed directory

# Test symlink is blocked
ln -s /etc/passwd /tmp/evil.d2
review-diagrams /tmp/evil.d2
# Expected: Error - path outside allowed directory

# Test large file is blocked
dd if=/dev/zero of=/tmp/huge.d2 bs=1M count=100
review-diagrams /tmp/huge.d2
# Expected: Error - file too large
```

---

## Vulnerability Summary

| ID | Vulnerability | Severity | Files Affected |
|---|---|---|---|
| **VUL-1** | Path traversal via `../` | **CRITICAL** | All 4 skills |
| **VUL-2** | Symlink following | **CRITICAL** | All 4 skills |
| **VUL-3** | Output path validation | **HIGH** | create-diagrams, render-diagrams |
| **VUL-4** | File size DoS | **HIGH** | review-diagrams |
| **VUL-5** | Binary PATH manipulation | MEDIUM | All 4 skills |
| **VUL-6** | Codebase size DoS | MEDIUM | create-diagrams |

**Total**: 6 vulnerabilities (3 Critical, 2 High, 2 Medium)

---

## Fix Priority

### ⚠️ Phase 1: CRITICAL (Deploy Immediately)

**Timeline**: Within 24 hours
**Effort**: 2-3 hours

1. Replace `os.path.abspath()` with path validation
2. Add file size limits
3. Validate output paths

**Impact**: Blocks 5/6 vulnerabilities

### 📋 Phase 2: HIGH (Deploy Week 1)

**Timeline**: Within 7 days
**Effort**: 1-2 hours

4. Harden binary search paths
5. Add codebase size limits

**Impact**: Blocks 6/6 vulnerabilities

### ✅ Phase 3: Testing (Week 2)

**Timeline**: Within 14 days
**Effort**: 2-3 hours

6. Expand security test suite
7. Add fuzzing tests
8. Security documentation

---

## For Developers

### Before You Commit
```bash
# Run security tests
python3 -m pytest plugins/spec-review-marketplace/tests/test_security.py -v

# All tests must pass
```

### Secure Coding Pattern
```python
# ❌ DON'T
path = os.path.abspath(user_path)  # VULNERABLE

# ✅ DO
path = validate_path(user_path, allowed_base)  # SECURE
```

See `SECURITY-FIXES-GUIDE.md` for complete patterns.

---

## For Security Researchers

### Responsible Disclosure
Found a vulnerability? Please:
1. **Do NOT** open public GitHub issues for critical vulnerabilities
2. Email: security@engram.dev
3. Use GitHub Security Advisories for private reporting
4. Allow 90 days for patching before public disclosure

### Bug Bounty
Currently no formal bug bounty program, but:
- Security contributions are highly valued
- Credit given in CHANGELOG and security advisories
- Swag/recognition for significant findings

---

## References

### Standards & Compliance
- **OWASP Top 10 (2021)**: A01 (Broken Access Control)
- **CWE-22**: Path Traversal
- **CWE-59**: Improper Link Resolution
- **CWE-400**: Resource Exhaustion

### Tools
- `pytest` - Test runner
- `bandit` - Python security linter
- `safety` - Dependency vulnerability scanner

### Related Documentation
- `/docs/SECURITY.md` - Project-wide security policy
- `/docs/CONTRIBUTING.md` - Security contribution guidelines

---

**Questions?** Open an issue with the `security` label or contact security@engram.dev

**Last Updated**: 2026-03-17
**Next Audit**: 2026-04-01 (after fixes deployed)
