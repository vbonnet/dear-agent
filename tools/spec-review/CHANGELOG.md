# Changelog

All notable changes to the spec-review-marketplace will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - 2026-03-17

### Added

#### New Skills (Diagram-as-Code Support)
- **create-diagrams**: Generate C4 architecture diagrams from codebase analysis
  - Supports D2, Mermaid, and Structurizr DSL formats
  - Multi-language codebase analysis (Go, Python, TypeScript, Java)
  - C4 model generation (Context, Container, Component levels)
  - 10-1200x faster than manual diagram creation

- **review-diagrams**: Multi-persona diagram quality validation
  - 4-persona weighted voting (System Architect 40%, Technical Writer 30%, Developer 20%, DevOps 10%)
  - C4 compliance checking
  - Cost tracking integration (~/.engram/diagram-costs.jsonl)
  - Structured verdicts (GO/NO-GO/ABSTAIN with confidence scoring)

- **render-diagrams**: Compile diagram-as-code to visual formats
  - Output formats: PNG, SVG, PDF
  - Syntax validation before rendering
  - Multiple layout engines (elk, dagre, tala)
  - Accessibility validation (WCAG 2.1 Level AA framework)

- **diagram-sync**: Detect drift between diagrams and codebase
  - Automated sync scoring (diagram vs reality)
  - CI/CD integration support
  - Patch generation for outdated diagrams
  - Pre-commit hook templates

#### Infrastructure
- **Security hardening**: Comprehensive security validation module (lib/security_utils.py)
  - Path traversal prevention
  - Symlink protection
  - File size limits (10MB max for diagrams)
  - Extension validation (allowed formats only)
  - Safe directory creation

- **Build system**: Centralized Makefile for all skills
  - Individual and bulk build targets
  - Clean artifacts management
  - Test execution

- **Multi-persona gate**: Production-ready weighted voting system
  - Integrated from open-viking session
  - Dry-run mode for testing without LLM API costs
  - Blocker tracking with severity levels

### Changed
- **marketplace.json**: Updated to version 2.0.0
  - Added 4 new diagram-as-code skills
  - Updated description to include diagrams
  - Added diagram-related tags (c4-model, diagram-as-code)
  - Added diagram-quality-rubric

### Documentation
- **Phase 9 Testing Reports** (2,862+ lines):
  - PERFORMANCE-BENCHMARKS.md: All operations 10-1200x faster than targets
  - SECURITY-AUDIT-REPORT.md: Comprehensive vulnerability assessment
  - ACCESSIBILITY-REPORT.md: WCAG 2.1 Level AA compliance verification
  - CROSS-CLI-TEST-REPORT.md: Integration patterns and usage
  - PHASE-9-VALIDATION-REPORT.md: Multi-persona validation (CONDITIONAL GO → GO)
  - SECURITY-FIXES-APPLIED.md: All 5 vulnerabilities fixed, 45 tests passing

### Testing
- **45 security tests**: 100% passing
  - Path traversal attack prevention
  - Symlink following protection
  - File size DoS mitigation
  - Command injection prevention
  - Input sanitization verification

- **Performance benchmarks**: Production-ready
  - create-diagrams: 0.25-0.31s (constant time up to 300 files)
  - review-diagrams: 0.30-0.40s (mock mode; real LLM adds ~2-4s)
  - render-diagrams: 0.50-0.88s (validation only)
  - diagram-sync: 0.27-0.29s (90 files)

### Security
- **Fixed vulnerabilities** (all P1/P2):
  - VUL-1: Path traversal via `../` sequences (CRITICAL) ✓
  - VUL-2: Symlink following (CRITICAL) ✓
  - VUL-3: Output path validation (HIGH) ✓
  - VUL-4: File size DoS (HIGH) ✓
  - VUL-5: Insufficient path validation (HIGH) ✓

- **Secure patterns verified**:
  - No command injection (proper subprocess usage)
  - Timeout protections (30s validation, 300s render)
  - YAML safe loading
  - Format validation via enums

## [1.0.0] - 2026-03-11

### Added
- **review-spec**: Validate SPEC.md files with LLM-as-judge
- **review-architecture**: Multi-persona ARCHITECTURE.md validation
- **review-adr**: ADR validation with anti-pattern detection
- **create-spec**: Generate SPEC.md from codebase analysis
- Cross-CLI support (Claude Code, Gemini, OpenCode, Codex)
- Research-backed quality rubrics
- Multi-persona review framework

---

## Migration Guide (1.0.0 → 2.0.0)

### For Users
No breaking changes. All existing skills continue to work identically.

**New capabilities**:
- Generate diagrams with `/create-diagrams`
- Validate diagrams with `/review-diagrams`
- Render diagrams with `/render-diagrams`
- Check sync with `/diagram-sync`

### For Developers
**New dependencies** (diagram skills only):
- pyyaml>=6.0.0 (YAML parsing for diagram metadata)
- External tools (optional, for rendering):
  - d2 (https://d2lang.com/)
  - @mermaid-js/mermaid-cli (https://github.com/mermaid-js/mermaid-cli)
  - structurizr-cli (https://github.com/structurizr/cli)

**Security module**:
- All diagram skills use `lib/security_utils.py` for file operations
- Prevents path traversal, symlink attacks, and DoS
- See `tests/SECURITY-FIXES-APPLIED.md` for details

### Installation
```bash
# Install diagram rendering tools (optional)
# D2
go install oss.terrastruct.com/d2@latest

# Mermaid CLI
npm install -g @mermaid-js/mermaid-cli

# Structurizr CLI
# Download from https://github.com/structurizr/cli/releases
```

---

**Links**:
- [GitHub Repository](https://github.com/vbonnet/engram)
- [Documentation](https://github.com/vbonnet/engram/tree/main/plugins/spec-review-marketplace)
- [Security Reports](https://github.com/vbonnet/engram/tree/main/plugins/spec-review-marketplace/tests)
