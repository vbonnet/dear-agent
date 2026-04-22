# AGM Environment Management: Product Specification

**Status**: Draft
**Created**: 2026-01-19
**Owner**: Foundation Engineering
**Target Release**: AGM v3.0

---

## Executive Summary

AGM (AI/Agent Session Manager) will provide **environment validation and guidance** for multiple AI CLI tools (Claude, Gemini, GPT), enabling users to quickly diagnose and fix configuration issues without AGM managing environments directly.

**Approach**: "Validate, Don't Manage" - AGM detects misconfigurations and guides users to fix them, but delegates actual environment management to existing tools (direnv, mise, shell RC files).

**MVP Scope**: Gemini CLI support only (validated pain point from Cloud Workstation environment conflicts).

---

## Problem Statement

### Current Pain Point

When users try to run the `gemini` CLI tool on Cloud Workstation, they encounter cryptic errors:

```
ApiError: {"error":{"code":403,"message":"Permission denied on resource project devunstbl-pleng-gafya8.","status":"PERMISSION_DENIED"}}
```

**Root cause**: Cloud Workstation sets environment variables optimized for Claude/Vertex AI that conflict with Gemini CLI:

```bash
GOOGLE_CLOUD_PROJECT=devunstbl-pleng-gafya8
GOOGLE_CLOUD_LOCATION=us-central1
GOOGLE_GENAI_USE_VERTEXAI=true  # ← Forces Vertex AI mode
```

**User impact**:
- Hours of debugging environment variable precedence
- Manual `export` commands needed in every shell session
- Variables set in `.bashrc` are overridden by `/etc/profile.d/` scripts
- No clear error messages explaining the conflict

**Why AGM should solve this**:
- AGM already manages session lifecycle (create, resume, archive)
- AGM understands multi-agent architecture (Claude, Gemini, GPT)
- AGM can provide agent-specific guidance during session creation
- Users expect `agm new --harness gemini-cli` to "just work"

### Why This Matters

As AGM evolves from AGM (Agent Session Manager) to multi-agent support, environment configuration becomes a critical user experience issue. The rename to AGM signals support for multiple AI agents, but current state requires users to manually configure each agent's environment.

---

## Goals

1. **Enable gemini CLI to work on Cloud Workstation** without manual environment variable exports
2. **Provide clear, actionable error messages** when environment is misconfigured
3. **Guide users to correct configuration** via `agm doctor gemini` command
4. **Validate environment before session creation** to fail fast with helpful errors
5. **Generate configuration templates** (`.envrc`, `~/.bashrc`) to reduce manual setup errors

---

## Non-Goals

❌ **NOT doing**:
- AGM will NOT manage environment variables at runtime (no override mechanism)
- AGM will NOT store API keys in AGM config files (security risk)
- AGM will NOT replace direnv/mise/shell RC files (overlap with existing tools)
- AGM will NOT install tools or manage tool versions (delegate to package managers)
- AGM will NOT support all agents in v1 (gemini only, expand later)

✅ **Explicitly deferring**:
- Claude environment validation (Phase 2 - after gemini validated)
- GPT environment validation (Phase 2 - after gemini validated)
- Keyring integration for API key storage (Phase 3 - advanced feature)
- Multi-agent comparison mode (Phase 3 - advanced feature)

---

## Solution Approach

### Philosophy: "Validate, Don't Manage"

**AGM's role**: Expert diagnostician and guide
**User's role**: Environment configuration
**Existing tools' role**: Environment management (direnv, mise, shell RC)

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│ User runs: agm new --harness gemini-cli research-task        │
└─────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│ AGM validates environment (agm doctor gemini)           │
│                                                         │
│ 1. Check GEMINI_API_KEY set?                           │
│ 2. Check GOOGLE_GENAI_USE_VERTEXAI=false?              │
│ 3. Detect conflicts (Vertex AI vars present?)          │
└─────────────────────────────────────────────────────────┘
                         │
                ┌────────┴────────┐
                │                 │
                ▼                 ▼
       ✅ All checks pass    ❌ Validation failed
                │                 │
                ▼                 ▼
       Create session    Show diagnosis:
                         - What's wrong
                         - Why it matters
                         - How to fix
                         - Template generation
```

### Key Components

1. **Agent Requirements Schema** (`~/.config/agm/agents/gemini.yaml`)
   - Declarative requirements (env vars, commands, conflicts)
   - Shipped with AGM binary (no external dependencies)
   - Versioned with AGM releases

2. **Validation Engine** (`agm doctor <agent>`)
   - Checks environment against requirements
   - Detects conflicts (Vertex AI vs API key mode)
   - Analyzes where variables are set (source analysis)
   - Generates human-readable error messages

3. **Template Generator** (`agm doctor <agent> --generate-{envrc,bashrc}`)
   - Produces `.envrc` or `~/.bashrc` snippets
   - Includes security warnings (never commit secrets)
   - Shows password manager integration examples

4. **Session Creation Gate** (`agm new --harness gemini-cli`)
   - Validates environment before creating session
   - Fails fast with clear errors
   - Guides user to `agm doctor gemini` for diagnosis

---

## User Experience

### Success Flow (Environment Already Configured)

```bash
$ agm new --harness gemini-cli research-task

Validating gemini environment...
✓ gemini CLI found
✓ GEMINI_API_KEY set
✓ GOOGLE_GENAI_USE_VERTEXAI=false

Creating session research-task...
✓ Session ready!

Attach with: agm attach research-task
```

**Time to success**: Instant

---

### First-Time Setup Flow (Environment Not Configured)

```bash
$ agm new --harness gemini-cli research-task

Validating gemini environment...
❌ Environment validation failed

Run `agm doctor gemini` for details and suggested fixes.
```

```bash
$ agm doctor gemini

╔══════════════════════════════════════════════════════════╗
║ Gemini Environment Validation                            ║
╚══════════════════════════════════════════════════════════╝

Command Checks:
  ✓ gemini CLI installed (~/.npm-global/bin/gemini)

Environment Variables:
  ❌ GEMINI_API_KEY not set
     Required for Gemini API authentication
     Docs: https://ai.google.dev/gemini-api/docs/api-key

  ❌ GOOGLE_GENAI_USE_VERTEXAI=true (expected: false)
     Current value forces Vertex AI mode, conflicts with API key

Conflicts Detected:
  ⚠️  GOOGLE_CLOUD_PROJECT=devunstbl-pleng-gafya8
     API key mode doesn't use project ID (can be ignored)

Environment Source Analysis:
  GOOGLE_GENAI_USE_VERTEXAI set by: /etc/profile.d/google-cloud-workstation.sh
  Override required in: ~/.bashrc or .envrc (loaded after /etc/profile.d/)

╔══════════════════════════════════════════════════════════╗
║ Recommended Fixes                                         ║
╚══════════════════════════════════════════════════════════╝

Option 1: Use direnv (recommended for per-project config)
  1. Create .envrc in your project directory:
     $ agm doctor gemini --generate-envrc > .envrc

  2. Edit .envrc and add your API key (never commit this!)

  3. Allow direnv to load it:
     $ direnv allow

Option 2: Use ~/.bashrc (global configuration)
  1. Add to ~/.bashrc:
     $ agm doctor gemini --generate-bashrc >> ~/.bashrc

  2. Edit ~/.bashrc and add your API key

  3. Reload shell:
     $ source ~/.bashrc

Option 3: Per-session export (temporary)
  $ export GEMINI_API_KEY="your-key-here"
  $ export GOOGLE_GENAI_USE_VERTEXAI=false
  $ agm new --harness gemini-cli my-session

Security Note:
  Never commit API keys to version control. Consider using:
  - pass (password store): export GEMINI_API_KEY=$(pass show gemini)
  - Vault: export GEMINI_API_KEY=$(vault kv get -field=key secret/gemini)
  - Encrypted secrets in .envrc.gpg

Run `agm doctor gemini --fix` to interactively configure.
```

```bash
$ agm doctor gemini --generate-envrc

# Gemini API Configuration
# WARNING: This file contains secrets. Add .envrc to .gitignore!

# Required: Gemini API key
# Get your key: https://console.cloud.google.com/apis/credentials
export GEMINI_API_KEY="REPLACE_WITH_YOUR_API_KEY"

# Required: Disable Vertex AI mode (use API key instead)
export GOOGLE_GENAI_USE_VERTEXAI=false

# Optional: Override Cloud Workstation defaults
unset GOOGLE_CLOUD_PROJECT
unset GOOGLE_CLOUD_LOCATION

# Security best practice: Load from password manager instead
# Example with pass:
#   export GEMINI_API_KEY=$(pass show gemini-api-key)
# Example with vault:
#   export GEMINI_API_KEY=$(vault kv get -field=key secret/gemini)
```

**Time to success**: 2-3 minutes (vs hours without guidance)

---

## Implementation Phases

### Phase 1: Foundation (Week 1)

**Deliverables**:
1. Agent Requirements Schema
   - `internal/agents/gemini.yaml` (requirements definition)
   - `internal/agents/schema.go` (YAML parser)
   - Unit tests for schema validation

2. Basic Validation Engine
   - `cmd/agm/doctor.go` (validation command)
   - Environment variable checking
   - Command existence checking
   - Basic error messages

3. Documentation
   - `docs/agents/gemini.md` (setup guide)
   - `docs/architecture/environment-validation.md` (design doc)

**Success Criteria**:
- `agm doctor gemini` detects GEMINI_API_KEY missing
- `agm doctor gemini` detects GOOGLE_GENAI_USE_VERTEXAI conflicts
- Exit code: 0 if valid, 1 if invalid

**Timeline**: 3-4 days

---

### Phase 2: Validation & Diagnosis (Week 2)

**Deliverables**:
1. Enhanced Validation
   - Environment source analysis (where vars are set)
   - Conflict detection (Vertex AI vs API key mode)
   - Detailed error messages with context

2. Template Generation
   - `agm doctor gemini --generate-envrc`
   - `agm doctor gemini --generate-bashrc`
   - Security warnings in templates

3. Session Creation Gate
   - Validate environment in `agm new --harness gemini-cli`
   - Fail fast with helpful errors
   - Guide to `agm doctor` for diagnosis

**Success Criteria**:
- Source analysis shows `/etc/profile.d/google-cloud-workstation.sh`
- Templates include security warnings
- `agm new --harness gemini-cli` blocks invalid environment

**Timeline**: 4-5 days

---

### Phase 3: Interactive Fixes (Week 3)

**Deliverables**:
1. Interactive Setup Wizard
   - `agm doctor gemini --fix` (interactive mode)
   - Prompt for API key with validation
   - Detect if direnv installed
   - Test environment after fix

2. Improved UX
   - Color-coded output (✓ green, ❌ red, ⚠️ yellow)
   - Progress indicators
   - Copy-paste friendly commands

**Success Criteria**:
- `agm doctor gemini --fix` walks user through setup
- API key format validated (starts with `AIza`, 40 chars)
- Environment tested after fix applied

**Timeline**: 3-4 days

---

### Phase 4: Polish & Expand (Week 4+)

**Deliverables**:
1. Claude Environment Validation
   - `internal/agents/claude.yaml`
   - `agm doctor claude` command
   - Vertex AI vs API key mode detection

2. GPT Environment Validation
   - `internal/agents/gpt.yaml`
   - `agm doctor gpt` command
   - OpenAI vs Azure OpenAI detection

3. Advanced Features (Deferred)
   - Keyring integration (macOS Keychain, gnome-keyring)
   - Multi-agent validation (`agm doctor --all`)
   - CI/CD mode (non-interactive validation)
   - Custom agent definitions (user can add agents)

**Success Criteria**:
- All 3 agents supported (claude, gemini, gpt)
- Validation logic reusable (agent-agnostic)
- Documentation complete for all agents

**Timeline**: 1-2 weeks (post-MVP)

---

## Acceptance Criteria

### Must Have (MVP - Gemini Only)

- [ ] `agm doctor gemini` command exists and runs
- [ ] Detects GEMINI_API_KEY missing (exit code 1)
- [ ] Detects GOOGLE_GENAI_USE_VERTEXAI=true conflict (exit code 1)
- [ ] Shows clear error message with fix suggestions
- [ ] Generates `.envrc` template via `--generate-envrc`
- [ ] Generates `~/.bashrc` snippet via `--generate-bashrc`
- [ ] Templates include security warnings (never commit secrets)
- [ ] `agm new --harness gemini-cli` validates environment before session creation
- [ ] Validation passes with correct environment (exit code 0)
- [ ] Documentation: `docs/agents/gemini.md` setup guide

### Should Have (Phase 2)

- [ ] Environment source analysis (shows where vars are set)
- [ ] Conflict detection shows `/etc/profile.d/` vs `.bashrc` precedence
- [ ] Template includes password manager examples (pass, vault)
- [ ] Error messages explain **why** conflicts matter (not just what)
- [ ] Validation output is color-coded (✓ green, ❌ red, ⚠️ yellow)

### Could Have (Phase 3)

- [ ] Interactive setup wizard (`agm doctor gemini --fix`)
- [ ] API key format validation (regex check)
- [ ] Test environment after fix applied
- [ ] Progress indicators during validation

### Won't Have (Explicitly Deferred)

- [ ] AGM managing environment variables at runtime
- [ ] AGM storing API keys in config files
- [ ] Claude/GPT agent support in MVP
- [ ] Keyring integration
- [ ] Multi-agent comparison mode

---

## Success Metrics

### Leading Indicators (Measure These First)

- **Time to first success**: User runs `agm new --harness gemini-cli` and succeeds (target: <5 minutes)
- **Error message clarity**: User understands error without external help (survey: >80% yes)
- **Template usage**: % of users who use `--generate-envrc` vs manual setup (target: >60%)

### Lagging Indicators (Measure After Launch)

- **Support requests**: Gemini environment issues reported (target: <5% of gemini users)
- **Adoption rate**: % of AGM users who use `--harness gemini-cli` (target: >30% within 1 month)
- **Setup success rate**: % of users who successfully configure gemini (target: >90%)

### Qualitative Indicators (User Feedback)

- **User satisfaction**: "AGM made gemini setup easy" (survey: >4/5 stars)
- **Confidence**: "I understand why the error occurred" (survey: >80% yes)
- **Trust**: "I trust AGM's guidance" (survey: >80% yes)

---

## Risks & Mitigations

### Risk 1: User Confusion ("Why doesn't AGM just fix it?")

**Risk**: Users expect AGM to auto-fix environment, frustrated by manual steps.

**Mitigation**:
- Clear documentation explaining "Validate, Don't Manage" philosophy
- `agm doctor --fix` interactive wizard feels like auto-fix (but user approves)
- Error messages explain **why** AGM doesn't manage (overlap with direnv, security)

**Acceptance**: Some users will prefer auto-fix. We accept this tradeoff for:
- Security (no secret management complexity)
- Simplicity (narrow scope, low maintenance)
- Compatibility (works with existing tools)

---

### Risk 2: Cloud Workstation Environment Changes

**Risk**: Cloud Workstation updates `/etc/profile.d/` scripts, breaking guidance.

**Mitigation**:
- Environment source analysis detects changes (shows where vars are set)
- Templates use `unset` for workstation defaults (defensive)
- Documentation includes troubleshooting section

**Monitoring**: Track support requests mentioning "Cloud Workstation" to detect breakage.

---

### Risk 3: Gemini CLI Breaking Changes

**Risk**: Gemini CLI changes environment variable names or behavior.

**Mitigation**:
- Agent requirements versioned with AGM releases
- CI tests validate requirements against actual gemini CLI
- Community can contribute requirement updates via PRs

**Acceptance**: We'll react to breakage, not predict it. Keep requirements simple.

---

### Risk 4: Template Security (Users Commit API Keys)

**Risk**: Users commit `.envrc` to git, leak API keys.

**Mitigation**:
- Templates include bold warnings: "Add .envrc to .gitignore!"
- `agm doctor --fix` checks if `.gitignore` exists, prompts to add `.envrc`
- Show password manager examples (pass, vault) in templates

**Long-term**: `agm doctor` could detect secrets in git-tracked files (future).

---

## Dependencies

### Internal Dependencies

- AGM v3.0 multi-agent architecture (in progress)
- Agent selection mechanism (`agm new --harness <harness>`)
- Session manifest schema migration

### External Dependencies

- Gemini CLI (`@google/generative-ai-cli` via npm)
- direnv (optional, recommended for users)
- User has API key from Google Cloud Console

---

## Open Questions

1. **Should `agm doctor` auto-add `.envrc` to `.gitignore`?**
   - Pro: Prevents secret leakage
   - Con: Modifying user's `.gitignore` feels invasive
   - **Recommendation**: Prompt user, don't auto-add

2. **How to handle multiple gemini API keys (dev/staging/prod)?**
   - Option A: Use direnv per-project (different `.envrc` per environment)
   - Option B: Use environment-specific config (`~/.config/agm/gemini-dev.yaml`)
   - **Recommendation**: Defer to Phase 3, use direnv for now

3. **Should validation cache results (avoid repeated checks)?**
   - Pro: Faster (don't re-check every `agm new`)
   - Con: Cache invalidation complexity (env vars change)
   - **Recommendation**: Don't cache in MVP, re-validate every time (fast enough)

---

## Alternatives Considered

### Alternative 1: AGM Manages Environments

**Approach**: AGM overrides environment variables when creating sessions.

**Pros**:
- "Just works" user experience
- No manual configuration needed

**Cons**:
- Overlaps with direnv/mise/shell RC (fifth environment layer)
- Security risk (storing API keys in AGM config)
- Doesn't work in CI/CD (AGM-specific, not portable)
- High support burden (environment debugging complexity)

**Decision**: Rejected. Multi-persona review showed 0/6 personas supported this.

---

### Alternative 2: Full Delegation (No Validation)

**Approach**: AGM assumes tools are pre-configured, provides no help.

**Pros**:
- Narrow scope (just session management)
- Low maintenance burden

**Cons**:
- Poor user experience (hours debugging environment)
- No value-add for multi-agent support
- Generic "command failed" errors (not actionable)

**Decision**: Rejected. User pain point is real, AGM should provide guidance.

---

### Alternative 3: Wrapper Scripts

**Approach**: AGM generates wrapper scripts that inject environment vars.

**Pros**:
- Works around Cloud Workstation defaults

**Cons**:
- Fragile (shell-specific, PATH issues)
- Doesn't teach users correct patterns
- Wrapper maintenance burden

**Decision**: Rejected. Template generation is simpler and more educational.

---

## References

- **Research Document**: `/tmp/agm-architecture-recommendation.md` (full analysis)
- **Multi-Persona Review**: `/tmp/agm-multi-persona-review.md` (stakeholder feedback)
- **Environment Management Research**: Agent research output (direnv, mise, nix-shell patterns)
- **Cloud Workstation Docs**: https://cloud.google.com/workstations
- **Gemini API Docs**: https://ai.google.dev/gemini-api/docs

---

## Success Definition

**MVP Success** = User runs `agm doctor gemini --fix`, follows wizard, and successfully creates gemini session in <5 minutes.

**Long-term Success** = AGM is recognized as the "AI agent session expert" that helps users configure environments correctly, without owning environment management.

---

## Appendix: Example Agent Requirements Schema

```yaml
# ~/.config/agm/agents/gemini.yaml (shipped with AGM)
name: gemini
description: Google Gemini CLI
docs: https://ai.google.dev/gemini-api/docs

requirements:
  commands:
    - name: gemini
      check: "command -v gemini"
      install_hint: "npm install -g @google/generative-ai-cli"

  env:
    GEMINI_API_KEY:
      required: true
      type: secret
      description: "Gemini API key from Google Cloud Console"
      docs: "https://ai.google.dev/gemini-api/docs/api-key"
      validation:
        pattern: "^AIza[A-Za-z0-9_-]{35}$"

    GOOGLE_GENAI_USE_VERTEXAI:
      required: true
      expected_value: "false"
      description: "Must be false for API key mode"

  conflicts:
    - env: GOOGLE_GENAI_USE_VERTEXAI
      value: "true"
      with_env: GEMINI_API_KEY
      reason: "Cannot use both Vertex AI and API key mode"
      severity: error

    - env: GOOGLE_CLOUD_PROJECT
      present: true
      with_env: GEMINI_API_KEY
      reason: "API key mode doesn't use project ID"
      severity: warning
```

---

**End of Specification**
