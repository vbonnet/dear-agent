# D4 Week 1: Critical Conditions Implementation

**Phase:** D4 - Implementation Execution (Week 1)
**Date:** 2025-12-04
**Status:** In Progress
**Previous Phase:** D3 Review Council (APPROVED 8.1/10)

## Purpose

Address 4 CRITICAL conditions from D3 Review Council before proceeding with Week 2-3 implementation. These conditions must be resolved in Week 1 to unblock beta testing and npm publishing in Weeks 2-3.

## D3 Review Council Approval Summary

**Verdict:** ✅ APPROVED - Cleared for D4
**Confidence:** 8.1/10 (5/5 personas approved)
**Critical Finding:** All 4 CRITICAL conditions are addressable in Week 1 and don't block foundational work

---

## CRITICAL Condition 1: Define CI/CD Pipeline

**Source:** Tech Lead (Maya Rodriguez), Skeptic (Alex Morgan)
**Priority:** CRITICAL
**Effort:** 2-3 hours
**Why Critical:** Week 2 alpha testing needs automated test runs, Week 3 beta needs npm publish

### Requirement

Set up GitHub Actions or [REDACTED_EMPLOYER] CI for:
1. Automated testing on PR
2. Code quality checks (lint, format)
3. npm package publishing

### Implementation: GitHub Actions Workflow

**File:** `.github/workflows/ci.yml`

```yaml
name: CI/CD

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  release:
    types: [created]

jobs:
  test:
    runs-on: ubuntu-latest

    strategy:
      matrix:
        node-version: [18.x, 20.x, 24.x]

    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js ${{ matrix.node-version }}
        uses: actions/setup-node@v4
        with:
          node-version: ${{ matrix.node-version }}
          cache: 'npm'
          cache-dependency-path: packages/[REDACTED_EMPLOYER]-mcp/package-lock.json

      - name: Install dependencies
        working-directory: packages/[REDACTED_EMPLOYER]-mcp
        run: npm ci

      - name: Lint
        working-directory: packages/[REDACTED_EMPLOYER]-mcp
        run: npm run lint

      - name: Format check
        working-directory: packages/[REDACTED_EMPLOYER]-mcp
        run: npm run format:check

      - name: Build
        working-directory: packages/[REDACTED_EMPLOYER]-mcp
        run: npm run build

      - name: Unit tests
        working-directory: packages/[REDACTED_EMPLOYER]-mcp
        run: npm test -- --coverage

      - name: Upload coverage
        if: matrix.node-version == '24.x'
        uses: codecov/codecov-action@v3
        with:
          files: packages/[REDACTED_EMPLOYER]-mcp/coverage/lcov.info
          flags: unittests

  integration:
    runs-on: ubuntu-latest
    needs: test

    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '24.x'
          cache: 'npm'
          cache-dependency-path: packages/[REDACTED_EMPLOYER]-mcp/package-lock.json

      - name: Install dependencies
        working-directory: packages/[REDACTED_EMPLOYER]-mcp
        run: npm ci

      - name: Build
        working-directory: packages/[REDACTED_EMPLOYER]-mcp
        run: npm run build

      - name: Integration tests
        working-directory: packages/[REDACTED_EMPLOYER]-mcp
        run: npm run test:integration

  publish:
    runs-on: ubuntu-latest
    needs: [test, integration]
    if: github.event_name == 'release'

    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '24.x'
          registry-url: 'https://[REDACTED_EMPLOYER]-npm-registry.com'  # Replace with actual [REDACTED_EMPLOYER] npm registry
          cache: 'npm'
          cache-dependency-path: packages/[REDACTED_EMPLOYER]-mcp/package-lock.json

      - name: Install dependencies
        working-directory: packages/[REDACTED_EMPLOYER]-mcp
        run: npm ci

      - name: Build
        working-directory: packages/[REDACTED_EMPLOYER]-mcp
        run: npm run build

      - name: Publish to npm
        working-directory: packages/[REDACTED_EMPLOYER]-mcp
        run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
```

### package.json Scripts

Add to `packages/[REDACTED_EMPLOYER]-mcp/package.json`:

```json
{
  "scripts": {
    "build": "tsc",
    "build:watch": "tsc --watch",
    "test": "jest",
    "test:watch": "jest --watch",
    "test:coverage": "jest --coverage",
    "test:integration": "jest --testPathPattern=__integration__",
    "lint": "eslint src/**/*.ts",
    "lint:fix": "eslint src/**/*.ts --fix",
    "format": "prettier --write src/**/*.ts tests/**/*.ts",
    "format:check": "prettier --check src/**/*.ts tests/**/*.ts",
    "prepublish": "npm run build"
  }
}
```

### Success Criteria

- ✅ CI runs on every PR
- ✅ Tests must pass before merge
- ✅ Code coverage report generated
- ✅ npm publish automated on release
- ✅ Tests run on Node.js 18, 20, 24

### Timeline

- **Day 1:** Create `.github/workflows/ci.yml`
- **Day 1:** Add npm scripts to package.json
- **Day 1:** Test CI on first PR

**Status:** ✅ DESIGNED - Ready to implement in actual vida repo

---

## CRITICAL Condition 2: Recruit Beta Testers

**Source:** Skeptic (Alex Morgan), Product Manager (Jordan Kim)
**Priority:** CRITICAL
**Effort:** 1 hour
**Why Critical:** Week 3 beta testing depends on having testers ready

### Requirement

Recruit 5-10 [REDACTED_EMPLOYER] developers to test the tool in Week 3 (beta phase).

### Recruitment Strategy

**Slack Announcement (Week 1, Day 1):**

**Channels:**
- #claude-code (if exists)
- #dev-tools
- #developer-experience

**Message Template:**

```
📢 Beta Testers Needed: MCP Setup Automation Tool

We're building a tool to automate MCP setup for Claude Code (think `gh auth login` for MCP).

**Current process:** 30+ minutes, 15+ manual steps
**With tool:** ~10-12 minutes, single command: `[REDACTED_EMPLOYER]-mcp setup`

**Looking for:** 5-10 beta testers in Week 3 (Dec 18-22)
**Time commitment:** 20-30 minutes (setup + feedback)
**Requirements:** Work machine (hostname ends with -w), want to use Claude Code

**What you'll test:**
- Automated Google Docs MCP setup
- Atlassian MCP configuration
- OAuth flow with GCP Console guidance
- Chezmoi integration (if you use it)

**Sign up:** React with ✅ to this message or DM me

**Beta week:** Dec 18-22 (Week 3)
**GA release:** Dec 25 (Week 4)

Questions? Ask in this thread!
```

### Tester Selection Criteria

**Ideal mix:**
- 2-3 fresh install users (no existing MCP setup)
- 2-3 users with existing manual MCP setup (migration test)
- 1-2 chezmoi users
- 1-2 users from different teams (Test Infra, PHP, DevEx)

### Tracking

**Spreadsheet: Beta Tester Roster**

| Name | Team | Use Case | Chezmoi? | Slack Handle | Status |
|------|------|----------|----------|--------------|--------|
| TBD | Test Infra | Fresh install | No | @user1 | Confirmed |
| TBD | PHP | Migration | No | @user2 | Confirmed |
| TBD | DevEx | Chezmoi | Yes | @user3 | Confirmed |
| ... | ... | ... | ... | ... | ... |

**Goal:** 5-10 confirmed testers by end of Week 1

### Timeline

- **Day 1:** Post Slack announcement
- **Day 2-3:** Respond to questions, recruit volunteers
- **Day 5:** Confirm 5-10 testers

### Success Criteria

- ✅ 5-10 confirmed beta testers
- ✅ Mix of use cases covered (fresh, migration, chezmoi)
- ✅ Testers available Week 3

**Status:** ✅ PLANNED - Ready to execute in Week 1

---

## CRITICAL Condition 3: Confirm npm Registry Access

**Source:** Skeptic (Alex Morgan)
**Priority:** CRITICAL
**Effort:** 1 hour
**Why Critical:** Week 3 beta distribution depends on npm publish capability

### Requirement

Verify access to [REDACTED_EMPLOYER] private npm registry OR document fallback plan for distribution.

### Investigation Steps

**Step 1: Check npm Registry Availability**

```bash
# Check current npm registry
npm config get registry

# Expected: https://[REDACTED_EMPLOYER]-npm-registry.com or similar
# If public npm (https://registry.npmjs.org/), need to find [REDACTED_EMPLOYER] private registry
```

**Step 2: Test Publishing (Dry Run)**

```bash
# From packages/[REDACTED_EMPLOYER]-mcp/
npm publish --dry-run --registry=https://[REDACTED_EMPLOYER]-npm-registry.com

# Expected: Success (shows what would be published)
# If fails: Investigate authentication or permissions
```

**Step 3: Verify Authentication**

```bash
# Check if authenticated
npm whoami --registry=https://[REDACTED_EMPLOYER]-npm-registry.com

# If not authenticated, set up:
npm login --registry=https://[REDACTED_EMPLOYER]-npm-registry.com
```

### Fallback Plan: Direct Install from vida Repo

If npm registry access is not available or delayed:

**Alternative Distribution (Week 2-3):**

```bash
# Alpha testers (Week 2)
cd ~/[REDACTED_EMPLOYER]-src/vida/packages/[REDACTED_EMPLOYER]-mcp
npm install
npm run build
npm link

# Beta testers (Week 3)
git clone https://github.com/[REDACTED_EMPLOYER]-src/vida.git
cd vida/packages/[REDACTED_EMPLOYER]-mcp
npm install
npm run build
npm link
```

**Alternative Installation Instructions:**

```markdown
## Installation (Beta - Direct from vida repo)

1. Clone vida repo:
   ```bash
   git clone https://github.com/[REDACTED_EMPLOYER]-src/vida.git
   cd vida/packages/[REDACTED_EMPLOYER]-mcp
   ```

2. Install and build:
   ```bash
   npm install
   npm run build
   ```

3. Link globally:
   ```bash
   npm link
   ```

4. Verify:
   ```bash
   [REDACTED_EMPLOYER]-mcp --version
   ```

## GA Release: npm Package

For GA release (Week 4), npm package will be available:

```bash
npm install -g @[REDACTED_EMPLOYER]/mcp-setup
```
```

### Decision Matrix

| Scenario | Beta Distribution | GA Distribution | Action |
|----------|-------------------|-----------------|--------|
| npm access ✅ | npm (private registry) | npm (private registry) | Ideal path |
| npm delayed ⏳ | Direct from vida repo | npm (when ready) | Acceptable |
| npm denied ❌ | Direct from vida repo | Direct from vida repo | Document in README |

### Timeline

- **Day 1:** Investigate npm registry availability
- **Day 1:** Test authentication and dry-run publish
- **Day 1:** Document result and fallback plan (if needed)

### Success Criteria

- ✅ npm registry access confirmed OR
- ✅ Fallback plan documented in README
- ✅ Beta distribution method decided

**Status:** ✅ PLANNED - Ready to investigate in Week 1

---

## CRITICAL Condition 4: Establish Baseline Metrics

**Source:** Product Manager (Jordan Kim)
**Priority:** CRITICAL
**Effort:** 2 hours
**Why Critical:** Can't measure 50% ticket reduction without baseline

### Requirement

Measure current state:
1. Support ticket volume (MCP setup related)
2. Manual setup time (actual measurement)

### Metric 1: Support Ticket Volume

**Jira Query (last 3 months):**

```jql
project = "IT Support" AND
text ~ "MCP" OR text ~ "Claude Code setup" OR text ~ "Google Docs MCP" OR text ~ "OAuth"
AND created >= -90d
ORDER BY created DESC
```

**Data to collect:**
- Total ticket count (last 3 months)
- Average tickets per month
- Common issues (OAuth fails, missing steps, config errors)
- Average resolution time

**Expected Format:**

```
Baseline Metrics (Nov 2024 - Jan 2025):
- Total MCP setup tickets: 24
- Average per month: 8 tickets
- Common issues:
  1. OAuth credential creation (10 tickets, 42%)
  2. Missing API enablement (6 tickets, 25%)
  3. Config file format errors (5 tickets, 21%)
  4. Token expiration (3 tickets, 12%)
- Average resolution time: 2.5 hours
```

### Metric 2: Manual Setup Time

**Measurement Protocol:**

1. **Find fresh volunteer** (hasn't set up MCP yet)
2. **Provide manual instructions** (current process)
3. **Time the setup** (from start to working MCP)
4. **Record steps** (where did they get stuck?)

**Repeat:** 3 times to get average

**Expected Format:**

```
Baseline Manual Setup Time:
- User 1: 32 minutes (stuck on OAuth consent screen for 10 min)
- User 2: 28 minutes (forgot to enable Drive API)
- User 3: 35 minutes (config file format wrong)
- Average: 31.7 minutes
- p95: 35 minutes
```

### Baseline Document

**File:** `BASELINE-METRICS.md` (in [REDACTED_EMPLOYER]-mcp repo)

```markdown
# Baseline Metrics - MCP Setup (Pre-Automation)

**Measurement Period:** November 2024 - January 2025
**Date Recorded:** 2025-12-04

## Support Ticket Volume

**Jira Query:** [MCP setup related tickets, last 90 days]

- Total tickets: 24
- Average per month: 8 tickets
- Peak month: December 2024 (12 tickets)

**Common Issues:**
1. OAuth credential creation - 10 tickets (42%)
2. Missing API enablement - 6 tickets (25%)
3. Config file format errors - 5 tickets (21%)
4. Token expiration - 3 tickets (12%)

**Average resolution time:** 2.5 hours per ticket

## Manual Setup Time

**Method:** Timed 3 fresh users following current manual instructions

| User | Time | Issues Encountered |
|------|------|-------------------|
| User 1 | 32 min | OAuth consent screen unclear |
| User 2 | 28 min | Forgot to enable Drive API |
| User 3 | 35 min | Config file JSON format wrong |

**Statistics:**
- Average: 31.7 minutes
- Median: 32 minutes
- p95: 35 minutes

## Success Criteria (Post-Automation)

**Setup Time:**
- Target: 10-12 minutes
- Reduction: 60% (from 32 min baseline)

**Support Tickets:**
- Target: 4 tickets/month (50% reduction)
- Measurement: Track for 3 months post-GA

**Error Rate:**
- Target: <5% of setup attempts fail
- Measurement: Telemetry from tool

## Measurement Plan

**Post-Launch (Week 4+):**
1. Track support tickets weekly
2. Collect telemetry from tool (setup success/failure)
3. Monthly NPS survey
4. Quarterly review of metrics vs targets
```

### Timeline

- **Day 1:** Run Jira query, analyze ticket volume
- **Day 2:** Recruit 3 volunteers for manual setup timing
- **Day 3:** Time manual setups (31-35 min each)
- **Day 3:** Document baseline in BASELINE-METRICS.md

### Success Criteria

- ✅ Baseline ticket volume documented
- ✅ Baseline manual setup time measured (3+ data points)
- ✅ BASELINE-METRICS.md committed to repo

**Status:** ✅ PLANNED - Ready to measure in Week 1

---

## HIGH Priority Conditions (Week 1-2)

### Condition 5: Stakeholder Communication Plan

**Source:** Product Manager (Jordan Kim)
**Effort:** 1 hour (Week 1)

**Deliverable:** Communication plan document

**Stakeholders:**
- DevEx team (primary owners)
- Test Infra team (backup owners)
- IT team (support ticket routing)
- Claude Code users (end users)

**Communication Timeline:**

| Event | Stakeholders | Channel | Message |
|-------|-------------|---------|---------|
| Alpha (Week 2) | DevEx team | Slack | Internal alpha test announcement |
| Beta (Week 3) | Beta testers | Slack | Beta invitation with instructions |
| GA (Week 4) | All developers | Slack, Confluence, Email | GA announcement with quick start |
| Post-GA | Support team | Slack | Handoff, troubleshooting guide |

**Status:** ✅ PLANNED

### Condition 6: Add Chezmoi Integration Test

**Source:** Tech Lead (Maya Rodriguez)
**Effort:** 2 hours (Week 2)

**File:** `tests/__integration__/chezmoi-full-setup.test.ts`

**Test Scenario:**
1. Mock chezmoi installation
2. Mock chezmoi template exists
3. Run setup
4. Verify tool detects chezmoi
5. Verify snippet shown (not auto-written)
6. Verify user can apply manually

**Status:** ✅ PLANNED - Defer to Week 2

### Condition 7: Security Code Review

**Source:** Skeptic (Alex Morgan)
**Effort:** 2 hours (Week 3)

**Checklist:**
- [ ] grep for `console.log(token)` or similar
- [ ] Verify file permissions enforcement (600)
- [ ] Verify path validation (no `..`, no `/etc/`)
- [ ] Verify .gitignore creation
- [ ] Verify token redaction in error messages

**Status:** ✅ PLANNED - Defer to Week 3

### Condition 8: macOS Cross-Platform Testing

**Source:** Skeptic (Alex Morgan), Future Self (Casey Liu)
**Effort:** 2 hours (Week 3)

**Test:** Run setup on macOS before beta release (most [REDACTED_EMPLOYER] devs use macOS)

**Status:** ✅ PLANNED - Defer to Week 3

---

## Week 1 Implementation Timeline

### Day 1 (4 hours): CRITICAL Conditions

**Morning (2 hours):**
1. ✅ Condition 1: Create `.github/workflows/ci.yml`
2. ✅ Condition 2: Post Slack beta tester recruitment
3. ✅ Condition 3: Investigate npm registry access

**Afternoon (2 hours):**
4. ✅ Condition 4: Run Jira query for baseline tickets
5. ✅ Condition 4: Recruit 3 volunteers for manual timing
6. ✅ Condition 5: Create stakeholder communication plan

### Day 2-3 (8 hours): Project Setup

1. Create vida repo structure (`packages/[REDACTED_EMPLOYER]-mcp/`)
2. Configure TypeScript, Jest, ESLint, Prettier
3. Create package.json with scripts
4. Create initial README skeleton
5. Time manual setups (3 volunteers)
6. Document baseline metrics

### Day 4-5 (8 hours): Foundation Implementation

1. Implement `src/lib/detect.ts` (environment detection)
2. Implement `src/commands/status.ts` (status command)
3. Implement `src/lib/install.ts` (MCP installation)
4. Write unit tests (5 tests for environment detection)

**Week 1 Total:** 20 hours

**Week 1 Exit Criteria:**
- ✅ All 4 CRITICAL conditions addressed
- ✅ CI/CD pipeline created
- ✅ 5-10 beta testers recruited
- ✅ npm access confirmed (or fallback documented)
- ✅ Baseline metrics measured and documented
- ✅ Project structure in place
- ✅ 3 foundation components implemented
- ✅ 5 unit tests passing

---

## D4 Week 1 Summary

**Status:** ✅ PLANNED - All CRITICAL conditions have clear implementation plans

**Confidence:** HIGH - All conditions are straightforward and time estimates are realistic

**Blockers:** None identified

**Ready for:** Execution in actual vida repo environment

**Next:** Once in vida repo, execute Day 1 (4 hours) to address all CRITICAL conditions

---

**Document Version:** 2025-12-04 (Final)
**Phase:** D4 Week 1 - Critical Conditions Implementation Planning
**Status:** Ready for Execution
