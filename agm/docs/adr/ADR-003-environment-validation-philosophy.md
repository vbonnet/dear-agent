# ADR-003: Environment Validation Philosophy ("Validate, Don't Manage")

**Status:** Accepted
**Date:** 2026-01-19
**Deciders:** Foundation Engineering Team, Multi-Persona Review
**Related:** agm-environment-management-spec.md

---

## Context

Users encounter cryptic errors when agent CLIs are misconfigured:
- Missing API keys
- Conflicting environment variables (e.g., Vertex AI vs API key mode)
- Incorrect tool versions
- Path issues

**Example Pain Point**:
```bash
$ agm new --harness gemini-cli research
[Gemini session starts, user types first message]
ApiError: {"error":{"code":403,"message":"Permission denied on resource project..."}}
```

User spends hours debugging, discovers `GOOGLE_GENAI_USE_VERTEXAI=true` was set by Cloud Workstation, conflicts with API key mode.

### Problem Statement

**Question**: Should AGM manage environment variables and tool installation, or validate and guide?

**Stakeholder Tension**:
- **Users**: "Just make it work, don't make me configure things"
- **DevOps**: "Don't overlap with direnv/mise, we already have environment tools"
- **Security**: "Never store API keys in AGM config files"

---

## Decision

We will implement **"Validate, Don't Manage"** philosophy:

**AGM's Role**: Expert diagnostician and guide
- Validates environment before session creation
- Provides actionable error messages with fix guidance
- Generates configuration templates (.envrc, .bashrc)

**User's Role**: Environment configuration
- Sets environment variables (via shell RC, direnv, mise)
- Installs tools (via package managers)
- Manages API keys (via password managers, env vars)

**Existing Tools' Role**: Environment management
- direnv: Per-directory environment
- mise: Multi-runtime management
- Shell RC files: Global environment

**AGM explicitly does NOT**:
- Set environment variables at runtime
- Store API keys in config files
- Install agent CLIs or dependencies
- Replace direnv/mise/shell RC

---

## Alternatives Considered

### Alternative 1: Full Environment Management

**Approach**: AGM sets environment variables when creating sessions, stores API keys in AGM config

**Pros**:
- "Just works" user experience
- No manual configuration needed
- Handles all setup automatically

**Cons**:
- **Security risk**: Storing API keys in plaintext config files
- **Overlap**: Fifth environment layer (after shell RC, /etc/profile.d/, direnv, mise)
- **Portability**: AGM-specific, doesn't work in CI/CD or other contexts
- **Support burden**: AGM becomes responsible for environment debugging
- **Complexity**: Must handle precedence, conflicts, updates

**Multi-Persona Review**: 0/6 personas supported this approach
- Solo Developer: "Too much magic, I lose control"
- Team Lead: "Security violation, can't store API keys like this"
- DevOps: "Overlaps with our direnv setup, creates conflicts"

**Verdict**: Rejected. Security risk and overlap with existing tools are dealbreakers.

---

### Alternative 2: Full Delegation (No Validation)

**Approach**: AGM assumes tools are pre-configured, provides no help

**Pros**:
- Narrow scope (just session management)
- Low maintenance burden
- No overlap with other tools

**Cons**:
- **Poor UX**: Users spend hours debugging environment issues
- **No value-add**: Doesn't differentiate from manual tmux usage
- **Generic errors**: "Command failed" provides no actionable guidance
- **Support burden**: Users file issues saying "Gemini doesn't work"

**Multi-Persona Review**: 1/6 personas supported this approach
- DevOps: "I already have environment management, AGM should stay out of it"

**Verdict**: Rejected. User pain is real, AGM should provide value beyond raw tmux.

---

### Alternative 3: Validate, Don't Manage (CHOSEN)

**Approach**: AGM validates environment and provides guidance, but delegates management to existing tools

**Pros**:
- **Clear errors**: "GEMINI_API_KEY not set" vs "ApiError: 403"
- **Actionable guidance**: Shows how to fix (template generation)
- **No overlap**: Works with direnv/mise/shell RC
- **Security**: Never stores secrets
- **Educational**: Teaches users correct patterns

**Cons**:
- **Manual steps**: Users must configure environment themselves
- **Not "automagic"**: Doesn't "just work" out of box

**Multi-Persona Review**: 5/6 personas supported this approach
- Solo Developer: "I understand what's wrong and how to fix it"
- Multi-Agent User: "Template generation saves me hours"
- Team Lead: "Standardized guidance reduces support burden"
- Security-Conscious User: "No secrets in config files, good"
- DevOps: "Works with our existing direnv setup"

**Verdict**: ACCEPTED. Best balance of UX, security, and compatibility.

---

## Implementation Details

### Validation Workflow

```
1. User: agm new --harness gemini-cli research-task

2. AGM: Validate environment (pre-flight check)
   ✓ Check gemini CLI installed
   ✓ Check GEMINI_API_KEY set
   ✓ Check GOOGLE_GENAI_USE_VERTEXAI=false
   ✓ Detect conflicts (Vertex AI vars)

3. If validation fails:
   → Show clear error message
   → Explain what's wrong and why it matters
   → Provide fix suggestions
   → Offer template generation (agm doctor gemini --generate-envrc)
   → Exit with error code 1

4. If validation passes:
   → Create session
   → Start agent CLI
```

---

### Agent Requirements Schema

**Location**: Shipped with AGM binary (no external dependencies)

```yaml
# Embedded in binary: internal/agents/gemini.yaml
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
      reason: "API key mode doesn't use project ID (can be ignored)"
      severity: warning
```

**Design Rationale**:
- **Declarative**: Requirements expressed as data, not code
- **Versionable**: Schema versioned with AGM releases
- **Testable**: CI validates against real agent CLIs
- **Extensible**: Community can contribute requirement updates

---

### Error Message Structure

**Bad Error** (before validation):
```
Error: exit status 1
```

**Good Error** (after validation):
```
╔══════════════════════════════════════════════════════════╗
║ Gemini Environment Validation Failed                      ║
╚══════════════════════════════════════════════════════════╝

Environment Variables:
  ❌ GEMINI_API_KEY not set
     Required for Gemini API authentication
     Docs: https://ai.google.dev/gemini-api/docs/api-key

  ❌ GOOGLE_GENAI_USE_VERTEXAI=true (expected: false)
     Current value forces Vertex AI mode, conflicts with API key

Recommended Fixes:

Option 1: Use direnv (per-project config)
  1. Create .envrc: agm doctor gemini --generate-envrc > .envrc
  2. Add your API key to .envrc
  3. Allow direnv: direnv allow

Option 2: Use ~/.bashrc (global config)
  1. Generate snippet: agm doctor gemini --generate-bashrc >> ~/.bashrc
  2. Add your API key
  3. Reload: source ~/.bashrc

Run `agm doctor gemini --fix` for interactive setup.
```

**Key Elements**:
- **What's wrong**: Clear, specific error (not generic)
- **Why it matters**: Explain impact ("conflicts with API key")
- **How to fix**: Multiple options with concrete steps
- **Next action**: Pointer to interactive fix (`--fix`)

---

### Template Generation

**Command**: `agm doctor gemini --generate-envrc`

**Output** (.envrc file):
```bash
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

**Features**:
- **Security warnings**: Bold reminder about .gitignore
- **Clear placeholders**: Easy to find what to replace
- **Documentation links**: Where to get API keys
- **Best practices**: Password manager examples
- **Copy-paste ready**: Works immediately after editing

---

## Consequences

### Positive

✅ **Clear Errors**: Users understand what's wrong in seconds, not hours
✅ **Security**: No API keys stored in AGM config files
✅ **Compatibility**: Works with direnv, mise, shell RC (no overlap)
✅ **Educational**: Templates teach correct patterns
✅ **Low Maintenance**: No environment management code to maintain

### Negative

⚠️ **Manual Steps**: Users must configure environment themselves (not "automagic")
⚠️ **Initial Friction**: First-time setup takes 2-5 minutes (vs instant if auto-managed)
⚠️ **Documentation Burden**: Must document each agent's requirements

### Neutral

🔄 **User Responsibility**: Users own their environment (pro for DevOps, con for beginners)
🔄 **Template Maintenance**: Must update templates when agent requirements change

---

## Mitigations

**Manual Steps**:
- Interactive wizard (`agm doctor gemini --fix`) feels like auto-fix (but user approves each step)
- Templates are copy-paste ready (minimal editing needed)
- Clear success criteria ("you're done when...")

**Initial Friction**:
- One-time cost (subsequent sessions instant)
- Template generation amortizes across team (share .envrc in repo, or .bashrc snippet)

**Documentation Burden**:
- Auto-generate from agent requirements YAML
- CI tests validate requirements against real CLIs
- Community can contribute updates via PRs

---

## Validation

**Success Metrics**:
- Time to setup new agent: <5 minutes (baseline: 2-4 hours)
- Environment-related support requests: <5% of users (baseline: 30%)
- Template usage: >60% use `--generate-envrc` (vs manual configuration)

**BDD Scenarios**:
- Create Gemini session without API key → clear error, shows fix
- Run `agm doctor gemini --fix` → interactive setup wizard
- Generate .envrc template → valid, copy-paste ready
- Environment valid → session creation succeeds

**User Testing**:
- Survey: "I understand why the error occurred" (>80% yes)
- Survey: "I know how to fix the error" (>80% yes)
- Survey: "AGM's guidance was helpful" (>4/5 stars)

---

## Related Decisions

- **ADR-001**: Multi-Agent Architecture (agents expose IsAvailable() for validation)
- **ADR-004**: Fail-Fast Error Handling (validate before side effects)
- **Spec**: agm-environment-management-spec.md (detailed requirements)

---

## Future Extensions

**v3.1+**:
- Environment source analysis (where vars are set, precedence issues)
- Password manager integration guidance (pass, vault, 1Password)
- CI/CD mode (`--non-interactive` for scripts)

**v4.0+**:
- Keyring integration (macOS Keychain, gnome-keyring)
- Secret detection (warn if .envrc tracked in git)
- Custom agent requirements (users can define requirements for custom agents)

---

## Rejected Ideas

**Auto-Add .envrc to .gitignore**:
- Pro: Prevents secret leakage
- Con: Modifying user's .gitignore feels invasive
- Decision: Prompt user to add, don't auto-add

**Cache Validation Results**:
- Pro: Faster (don't re-validate every session create)
- Con: Cache invalidation complexity (env vars change)
- Decision: Don't cache, re-validate (fast enough)

**Wrapper Scripts**:
- Pro: Works around Cloud Workstation environment
- Con: Fragile, shell-specific, PATH issues
- Decision: Template generation is simpler and more educational

---

## References

- **Security Best Practices**: https://12factor.net/config (store config in environment)
- **direnv**: https://direnv.net/ (per-directory environment management)
- **mise**: https://mise.jdx.dev/ (multi-runtime management)
- **Password Managers**: pass (https://www.passwordstore.org/), Vault (https://www.vaultproject.io/)

---

**Implementation Status:** ✅ Complete (Shipped in AGM v3.0 for Gemini, Claude/GPT planned)
**Date Completed:** 2026-02-04
