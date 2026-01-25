# W0: Project Charter - [REDACTED_EMPLOYER] MCP Setup Tool

**Project Name:** `[REDACTED_EMPLOYER]-mcp` - Automated MCP Setup CLI
**Domain:** Developer Tools / Infrastructure
**Estimated Effort:** 2-3 weeks (Phase 1: MVP)
**Project Type:** New standalone tool

## Vision Statement

Create an automated CLI tool that simplifies Model Context Protocol (MCP) setup for [REDACTED_EMPLOYER] employees, reducing setup time from 30+ minutes of manual OAuth configuration to a single `[REDACTED_EMPLOYER]-mcp setup` command, similar to `gh auth login` or `gcloud auth login`.

## Problem Statement

[REDACTED_EMPLOYER] employees using Claude Code need to manually configure MCP servers for accessing internal resources (Atlassian, Google Docs, future internal tools). The current process involves:

1. Navigating Google Cloud Console to create OAuth credentials
2. Manually configuring OAuth consent screens
3. Downloading and placing credentials files
4. Cloning and building MCP server repositories
5. Running manual authentication flows
6. Configuring MCP config files (potentially with chezmoi)

This 30+ step process is error-prone, poorly documented, and creates a high barrier to adoption of Claude Code at [REDACTED_EMPLOYER].

## Goals

### Primary Goals
1. **Reduce setup time**: From 30+ minutes to <5 minutes
2. **Improve UX**: Match quality of `gh auth login` / `gcloud auth login`
3. **Increase adoption**: Make Claude Code + MCPs accessible to all [REDACTED_EMPLOYER] employees
4. **Reduce support burden**: Self-service tool reduces tickets to DevEx team

### Secondary Goals
1. **Extensibility**: Plugin architecture for future MCP servers
2. **Maintainability**: Clear code structure, good documentation
3. **Reliability**: Handle errors gracefully, provide repair functionality

## Non-Goals (Out of Scope for v1)

- Creating new MCP servers (only configures existing ones)
- Managing Claude Code installation (assumes already installed)
- Supporting non-[REDACTED_EMPLOYER] use cases ([REDACTED_EMPLOYER]-first, though extensible)
- GUI/web interface (CLI only for v1)
- Automated updates/version management (future consideration)

## Success Criteria

**Quantitative:**
- Setup time: <5 minutes from start to authenticated
- Error rate: <5% of setup attempts fail
- Support tickets: 50% reduction in MCP-related tickets

**Qualitative:**
- Users can complete setup without documentation
- Clear, actionable error messages
- Works on both fresh installs and repair scenarios
- Positive feedback from early adopters

## Stakeholders

**Primary:**
- [REDACTED_EMPLOYER] employees using Claude Code
- Developer Experience team (DevEx)
- Test Infrastructure team (owns some MCP work per Jira)

**Secondary:**
- Security team (OAuth/credentials review)
- Platform Engineering (shared-dev-ai-pct45x project owners)
- Data Science teams (heavy Claude Code users)

**Key Contacts (from Jira research):**
- yyangv@[REDACTED_DOMAIN] - TESTENG-4611 (MCP findings)
- matted@[REDACTED_DOMAIN] - PHP-104513 (Claude Code activation)
- wulinhncs@[REDACTED_DOMAIN] - PHP-104386 (MCP prototyping)

## Current State

**Existing MCPs at [REDACTED_EMPLOYER]:**
1. **Atlassian MCP** (Jira/Confluence)
   - Official remote MCP
   - OAuth handled remotely
   - Documented in Atlassian User Guide

2. **Google Docs MCP** (Google Drive/Docs)
   - Community MCP (`a-bonus/google-docs-mcp`)
   - Requires manual OAuth setup
   - Uses `shared-dev-ai-pct45x` GCP project

**Current Workaround ([REDACTED_EMPLOYER]1 PR #93432):**
- Users are using **playwright MCP** with browser extension for Google Docs
- Combined with **acli** for Jira
- Implemented as Claude skill in `.claude/skills/context-fetcher/`
- Works but requires browser automation, not native MCP integration
- Shows demand for easier Google Docs access

**Configuration:**
- MCP config: `~/.config/claude-code/mcp.json`
- Managed by chezmoi templates (work machine detection via hostname)
- Work machines: Hostname ends with `-w`

**Infrastructure:**
- GCP Project: `shared-dev-ai-pct45x` (shared AI/Claude infrastructure)
- Node.js: Required for MCP servers
- gcloud: Available and authenticated

## Proposed Solution Summary

**Tool Name:** `[REDACTED_EMPLOYER]-mcp`

**Architecture:** Node.js CLI tool (not bash)
- Reuses existing OAuth libraries
- Better JSON handling
- Interactive prompts with inquirer
- Cross-platform compatible

**Core Commands:**
```bash
[REDACTED_EMPLOYER]-mcp setup            # Main wizard
[REDACTED_EMPLOYER]-mcp status           # Show current state
[REDACTED_EMPLOYER]-mcp auth google-docs # Re-authenticate
[REDACTED_EMPLOYER]-mcp validate         # Check setup
[REDACTED_EMPLOYER]-mcp repair           # Fix issues
```

**User Flow:**
1. Run `[REDACTED_EMPLOYER]-mcp setup`
2. Tool detects environment and existing setup
3. Guides through GCP Console OAuth creation (semi-automated)
4. Runs OAuth flow and saves tokens
5. Validates configuration
6. Reports status and next steps

## Repository Location Decision

**✅ DECISION: Start in `[REDACTED_EMPLOYER]-src/vida`, migrate later if needed**

**Options Considered:**

1. **New repo: `[REDACTED_EMPLOYER]-src/[REDACTED_EMPLOYER]-mcp`**
   - Pros: Clean separation, clear ownership, independent versioning
   - Cons: Requires approval process, slower to start

2. **devex-scripts** (`[REDACTED_EMPLOYER]-src/devex-scripts`)
   - Pros: Already has Python/TypeScript/Shell mix, DevEx team ownership
   - Cons: May get lost among other scripts, less discoverable

3. **test-infra-internal** (`[REDACTED_EMPLOYER]-src/test-infra-internal`)
   - Pros: Team has MCP experience (per Jira tickets)
   - Cons: Scope is broader than "test infrastructure", Go/Python focused

4. **vida** (`[REDACTED_EMPLOYER]-src/vida`) ⭐ **SELECTED**
   - Pros: TypeScript-focused (943KB TypeScript), developer tools context
   - Pros: Already experimental/iterative - fits our approach
   - Pros: No approval needed to add tool
   - Pros: Can migrate to standalone repo later with evidence of usage
   - Cons: Marked as "Experimental, not production ready" (acceptable for v1)

**Rationale:**
- Start fast without approval overhead
- Prove value before requesting new repo
- TypeScript ecosystem already established
- Easy migration path once tool is proven

**Migration Path:**
- v1: Build in vida, gather feedback
- v2: If successful, propose standalone repo with usage data
- Maintain backward compatibility during migration

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| GCP Console UI changes | High | Medium | Guide with screenshots, validate steps, provide fallback manual instructions |
| OAuth flow breaks over SSH | High | Medium | Test extensively, provide manual code entry, document port forwarding |
| Chezmoi template conflicts | Medium | Medium | Detect chezmoi, inform user, don't overwrite |
| Google Auth library changes | Medium | Low | Pin versions, test on updates |
| Users lack GCP project permissions | High | Low | Document required permissions, provide clear error messages |
| Low adoption | Medium | Medium | Good UX, marketing via DevEx, include in onboarding |

## Open Questions

1. **Ownership:** Which team should own this?
   - DevEx team (developer experience)
   - Test Infra team (has MCP work)
   - Platform Engineering (owns shared-dev-ai-pct45x)

2. **Distribution:** How to distribute?
   - npm package (global install)
   - Bundled with [REDACTED_EMPLOYER] onboarding
   - Part of Workbench

3. **Future MCPs:** What other MCPs should we support?
   - Linear MCP
   - Notion MCP
   - Internal [REDACTED_EMPLOYER] MCPs (per TESTENG tickets)

4. **Service Account Option:** Should we support service accounts?
   - Simpler for users (no browser flow)
   - Would require centralized service account creation
   - Security review needed

## Dependencies

**Hard Dependencies:**
- Node.js >=18.0.0 (already required for MCPs)
- npm (comes with Node.js)
- Git (for cloning MCP repos)

**Soft Dependencies:**
- gcloud CLI (for enhanced GCP integration)
- chezmoi (for managed configs)
- Claude Code (the reason for MCPs)

## Timeline Estimate

**Phase 1: MVP (2 weeks)**
- W1: Environment detection, status command
- W1: Google Docs MCP installation
- W2: OAuth flow, GCP guide
- W2: Setup wizard, validation

**Phase 2: Polish (1 week)**
- W3: Enhanced UX, error handling
- W3: Documentation, testing
- W3: Initial release

**Phase 3: Adoption (ongoing)**
- Gather feedback
- Add new MCPs
- Iterate based on usage

## Next Waypoints

- **W1 (Discovery):** Research existing patterns, similar tools, technical constraints
- **W2 (Scoping):** Define exact features for v1, create detailed requirements
- **W3 (Architecture):** Technical design, API specification, plugin system
- **W4 (Review Council):** Get approval from stakeholders and security
- **W5 (Research):** Investigate unknowns, prototype risky parts
- **W6 (Design):** Detailed implementation plan, file structure
- **W7+ (Implementation):** Build, test, deploy, iterate

## Appendix: Research Summary

**Documentation Found:**
- [REDACTED_EMPLOYER] Atlassian User Guide (4.3MB, comprehensive)
- MCP Proposal doc (internal design)
- Claude Code in Workbench (LAUNCH-13594)
- Multiple Jira tickets (TESTENG, PHP epics)

**Technical Details:**
- OAuth flow in `~/mcp-servers/google-docs-mcp/src/auth.ts`
- Chezmoi template in `~/.local/share/chezmoi/`
- Work machine detection via hostname suffix `-w`

**Key Findings:**
- `shared-dev-ai-pct45x` is the approved GCP project
- Docs & Drive APIs already enabled
- Security review exists for Claude Code (approved)
- Remote MCP for Atlassian requires no local setup

---

**Status:** ✅ Charter Complete - Ready for W1 Discovery
**Next Step:** Begin W1 discovery to explore technical implementation details
**Owner:** TBD (needs team assignment)
**Created:** 2025-12-04
