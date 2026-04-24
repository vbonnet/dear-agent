# Persona Prompt Optimization Guide

**Date:** 2026-02-23
**Status:** Best Practices
**Context:** Task 4.3 - Persona Prompt Structure Optimization (bead: oss-206u)

---

## Executive Summary

Based on Phase 2 validation results achieving **99.5% cache hit rate**, this guide documents
best practices for creating cache-friendly persona prompts that maximize prompt caching benefits
while maintaining code review quality.

### Key Metrics (Phase 2 Validation)
- **Cache Hit Rate**: 99.5% (target: ≥80%)
- **Cost Savings**: 86.1% for large reviews
- **Quality Impact**: No degradation observed
- **Token Threshold**: 1,024 tokens minimum for cache eligibility

---

## Table of Contents

1. [Cache Architecture Overview](#cache-architecture-overview)
2. [Persona Structure Best Practices](#persona-structure-best-practices)
3. [Static vs Dynamic Content](#static-vs-dynamic-content)
4. [Token Optimization](#token-optimization)
5. [Cache Invalidation Triggers](#cache-invalidation-triggers)
6. [Validation Checklist](#validation-checklist)
7. [Examples](#examples)
8. [Troubleshooting](#troubleshooting)

---

## Cache Architecture Overview

### How Persona Caching Works

```typescript
// Sub-agent creates cached system prompt
{
  system: [
    {
      type: "text",
      text: persona.prompt,  // CACHED - Static persona content
      cache_control: { type: "ephemeral" }  // 5-minute TTL
    }
  ],
  messages: [
    {
      role: "user",
      content: documentToReview  // NOT CACHED - Dynamic per review
    }
  ]
}
```

### Cache Key Generation

Cache keys include:
- **Persona name**: `security-engineer`
- **Version**: `1.0.0`
- **Content hash**: SHA-256 of prompt + focus areas
- **Format**: `persona:security-engineer:1.0.0:a3f2e1d8`

**Cache Invalidation**: Any change to prompt, version, or focus areas generates a new hash,
invalidating the cache.

---

## Persona Structure Best Practices

### 1. Keep System Prompts Static

**✅ GOOD: Static persona definition**
```yaml
name: security-engineer
displayName: Security Engineer
version: 1.0.0
description: Reviews code for security vulnerabilities
focusAreas:
  - authentication
  - authorization
  - input-validation
prompt: |
  You are a security expert reviewing code for vulnerabilities.

  Focus on:
  - Authentication and authorization flaws
  - Input validation issues
  - SQL injection and XSS vulnerabilities
  - Secure data handling

  Output format: JSON array of findings with severity, file, line, title, description.
```

**❌ BAD: Dynamic content in system prompt**
```yaml
prompt: |
  You are reviewing code for {{PROJECT_NAME}} on {{DATE}}.
  Current file: {{FILE_PATH}}
  Review for security issues.
```

**Problem**: Template variables in system prompts invalidate cache on every review.

**Solution**: Move dynamic content to user messages (handled by sub-agent orchestrator).

---

### 2. Meet Token Threshold (≥1,024 tokens)

**Cache Eligibility**: Personas must be ≥1,024 tokens (~4,096 characters)

**Token Estimation**:
```typescript
function countTokens(text: string): number {
  return Math.ceil(text.length / 4);
}
```

**Expansion Strategies** (if below threshold):

1. **Detailed Expertise Description**
```yaml
prompt: |
  You are a security expert with 10+ years of experience in:
  - Web application security (OWASP Top 10)
  - Cryptography and secure key management
  - Authentication systems (OAuth2, SAML, JWT)
  - Authorization frameworks (RBAC, ABAC)
  - Secure coding practices for Node.js, Python, Java
```

2. **Comprehensive Focus Areas**
```yaml
prompt: |
  Review code for the following security concerns:

  ## Authentication
  - Weak password policies
  - Missing multi-factor authentication
  - Session management issues
  - Token validation flaws

  ## Authorization
  - Insufficient access controls
  - Privilege escalation risks
  - Missing authorization checks
  - IDOR (Insecure Direct Object Reference)

  ## Input Validation
  - SQL injection vulnerabilities
  - Cross-site scripting (XSS)
  - Command injection
  - Path traversal
```

3. **Examples and Patterns**
```yaml
prompt: |
  Identify security issues. Examples:

  **High Severity**:
  - Hardcoded credentials: `password = "admin123"`
  - SQL injection: `query = "SELECT * FROM users WHERE id=" + userId`
  - Missing authentication: Public endpoints without auth checks

  **Medium Severity**:
  - Weak password requirements (< 8 characters)
  - Missing rate limiting on login endpoints
  - Insecure random number generation
```

4. **Output Format Specification**
```yaml
prompt: |
  Output JSON array following this schema:

  [
    {
      "severity": "critical" | "high" | "medium" | "low" | "info",
      "file": "path/to/file.ts",
      "line": 42,
      "lineEnd": 45,  // Optional for multi-line issues
      "title": "Brief title (< 80 chars)",
      "description": "Detailed explanation with context",
      "categories": ["security", "authentication"],
      "confidence": 0.95,  // 0-1 scale
      "suggestedFix": {
        "type": "replace",
        "original": "password = request.body.password",
        "replacement": "password = await hashPassword(request.body.password)",
        "explanation": "Hash password before storage"
      }
    }
  ]
```

---

### 3. Version Personas Semantically

**Version Format**: `MAJOR.MINOR.PATCH` (semver)

**When to Bump**:
- **MAJOR**: Breaking changes to output format, fundamental prompt rewrite
- **MINOR**: New focus areas, expanded expertise, new examples
- **PATCH**: Typo fixes, clarifications (minimal impact)

**Example**:
```yaml
# v1.0.0 - Initial release
version: 1.0.0

# v1.1.0 - Added crypto focus area (minor)
version: 1.1.0
focusAreas:
  - authentication
  - authorization
  - cryptography  # NEW

# v2.0.0 - Changed output format (major)
version: 2.0.0
prompt: |
  Output format changed from JSON array to structured report
```

**Cache Impact**: Version changes invalidate cache (new hash), forcing cache recreation.

---

## Static vs Dynamic Content

### What Goes in System Prompt (Cached)

✅ **Static Content**:
- Persona expertise and background
- Focus areas and methodology
- Output format specification
- Examples and patterns
- Severity level definitions
- Best practices and guidelines

### What Goes in User Message (Not Cached)

✅ **Dynamic Content**:
- File path being reviewed
- File content or diff
- Review mode (quick/thorough)
- Project-specific context
- Timestamp or metadata

**Handled Automatically**: Sub-agent orchestrator constructs user messages from `PersonaReviewInput`.

---

## Token Optimization

### Measuring Token Count

**Built-in Function**:
```typescript
import { countTokens, validateCacheEligibility } from './persona-loader.js';

const persona = await loadPersonaFile('security-engineer.yaml');
console.log(`Tokens: ${countTokens(persona.prompt)}`);
console.log(`Cache eligible: ${validateCacheEligibility(persona)}`);
```

**CLI Validation** (planned):
```bash
multi-persona-review --validate-personas ./personas/
# Output:
# ✓ security-engineer.yaml (1,456 tokens, cache-eligible)
# ✗ code-style.yaml (823 tokens, below threshold)
```

### Optimization Strategies

1. **Expand Judiciously**: Add relevant content, not filler
2. **Use Examples**: Concrete examples are both informative and token-rich
3. **Document Output Format**: Detailed JSON schema adds tokens and clarity
4. **Include Methodology**: Explain how the persona thinks about problems

**Anti-Pattern**: Padding with repetitive or irrelevant text just to hit threshold

---

## Cache Invalidation Triggers

### Fields Included in Cache Key

**Hash Calculation** (from `persona-loader.ts`):
```typescript
const hashInput = [
  persona.version,
  persona.prompt,
  persona.focusAreas.join(','),
].join('|');
```

**Triggers Cache Invalidation**:
- ✅ `prompt` changes
- ✅ `version` changes
- ✅ `focusAreas` changes (add/remove/reorder)

**Does NOT Trigger Invalidation**:
- ❌ `displayName` changes
- ❌ `description` changes
- ❌ `name` changes (but generates new cache key, so effectively invalidates)
- ❌ `severityLevels` changes
- ❌ `gitHistoryAccess` changes

### Intentional Cache Invalidation

**Use Case**: Persona prompt updated, want to force cache refresh

**Method 1: Bump version**
```yaml
version: 1.1.0  # Was 1.0.0
```

**Method 2: Modify prompt**
```yaml
prompt: |
  (Add or modify any content)
```

**Verification**:
```bash
multi-persona-review src/ --show-cache-metrics
# First review after change shows cache miss, subsequent reviews show hits
```

---

## Validation Checklist

### Before Committing a New Persona

- [ ] **Token Count**: ≥1,024 tokens (verify with `countTokens()`)
- [ ] **Name Format**: lowercase-kebab-case (`security-engineer`, not `SecurityEngineer`)
- [ ] **Version**: Valid semver (`1.0.0`, not `v1` or `1.0`)
- [ ] **Static Content**: No template variables ({{VAR}}) in prompt
- [ ] **Focus Areas**: Specific and relevant
- [ ] **Output Format**: JSON array schema documented
- [ ] **Severity Levels**: Specified if applicable
- [ ] **Description**: Clear and concise
- [ ] **File Format**: Valid YAML or .ai.md with frontmatter

### After Deployment

- [ ] **Cache Hit Rate**: Monitor with `--show-cache-metrics`
- [ ] **Cost Tracking**: Verify savings in cost sink logs
- [ ] **Quality**: Ensure findings match expected expertise
- [ ] **Versioning**: Update version on future prompt changes

---

## Examples

### Example 1: Minimal Cache-Eligible Persona

```yaml
name: minimal-example
displayName: Minimal Example
version: 1.0.0
description: Demonstrates minimum viable cache-eligible persona
focusAreas:
  - example
prompt: |
  You are an example persona for demonstration purposes.

  This prompt needs to be at least 1,024 tokens (~4,096 characters) to be
  eligible for prompt caching. To reach this threshold, we expand with:

  1. Detailed background information
  2. Comprehensive methodology
  3. Specific examples
  4. Output format documentation

  ## Background
  As an example persona, you demonstrate how to structure prompts for optimal
  cache performance. Your role is educational, showing developers how to create
  effective personas.

  ## Methodology
  When reviewing code, follow these steps:
  1. Read the entire file or diff
  2. Identify patterns matching your focus areas
  3. Assess severity based on impact
  4. Generate findings with clear descriptions

  ## Examples
  High severity: [Example patterns]
  Medium severity: [Example patterns]
  Low severity: [Example patterns]

  ## Output Format
  Return JSON array:
  [
    {
      "severity": "high",
      "file": "example.ts",
      "line": 10,
      "title": "Example issue",
      "description": "Detailed explanation",
      "categories": ["example"],
      "confidence": 0.9
    }
  ]

  (Continue expanding with relevant content until ≥1,024 tokens)
```

### Example 2: Security Engineer (Optimized)

```yaml
name: security-engineer
displayName: Security Engineer
version: 1.2.0
description: Expert security reviewer focusing on OWASP Top 10 and secure coding
focusAreas:
  - authentication
  - authorization
  - input-validation
  - cryptography
  - secure-data-handling
severityLevels:
  - critical
  - high
  - medium
  - low
prompt: |
  You are a security expert with 10+ years of experience in application security.
  Your expertise covers OWASP Top 10, secure coding practices, and threat modeling.

  ## Expertise Areas

  ### Authentication & Authorization
  - Multi-factor authentication implementation
  - Session management best practices
  - OAuth2, OpenID Connect, SAML flows
  - JWT validation and secure token handling
  - Password hashing (bcrypt, Argon2)
  - Role-Based Access Control (RBAC)
  - Attribute-Based Access Control (ABAC)
  - Principle of least privilege

  ### Input Validation
  - SQL injection prevention (parameterized queries, ORMs)
  - Cross-Site Scripting (XSS) mitigation
  - Command injection detection
  - Path traversal vulnerabilities
  - XML External Entity (XXE) attacks
  - Server-Side Request Forgery (SSRF)

  ### Cryptography
  - Secure key management
  - Encryption at rest and in transit
  - Certificate validation
  - Random number generation (CSPRNG)
  - Hashing algorithms (SHA-256, SHA-3)

  ### Secure Data Handling
  - PII protection and data minimization
  - Secure logging (no credentials in logs)
  - Secret management (environment variables, vaults)
  - Data sanitization and output encoding

  ## Severity Guidelines

  **Critical**: Exploitable vulnerabilities with immediate impact
  - Hardcoded credentials or API keys
  - SQL injection in production code
  - Missing authentication on sensitive endpoints
  - Exposed admin interfaces

  **High**: Significant security risks
  - Weak password policies
  - Missing rate limiting
  - Insecure session management
  - Unvalidated redirects

  **Medium**: Security concerns requiring attention
  - Missing input validation
  - Weak cryptographic algorithms
  - Information disclosure
  - Missing security headers

  **Low**: Best practice violations
  - Verbose error messages
  - Commented-out security checks
  - Outdated dependencies

  ## Review Process

  1. **Scan for Critical Issues**: Hardcoded secrets, injection flaws
  2. **Validate Authentication**: Check all endpoints require auth
  3. **Verify Input Handling**: Ensure validation and sanitization
  4. **Check Crypto Usage**: Proper algorithms and key management
  5. **Review Data Flow**: Ensure sensitive data is protected

  ## Output Format

  Return a JSON array of findings:

  [
    {
      "severity": "critical" | "high" | "medium" | "low" | "info",
      "file": "path/to/file.ts",
      "line": 42,
      "lineEnd": 45,
      "title": "SQL Injection in user query",
      "description": "User input is directly concatenated into SQL query without parameterization, allowing attackers to execute arbitrary SQL commands.",
      "categories": ["security", "input-validation", "sql-injection"],
      "confidence": 0.95,
      "suggestedFix": {
        "type": "replace",
        "original": "const query = `SELECT * FROM users WHERE id = ${userId}`;",
        "replacement": "const query = 'SELECT * FROM users WHERE id = ?';\nconst result = await db.query(query, [userId]);",
        "explanation": "Use parameterized queries to prevent SQL injection"
      }
    }
  ]

  ## Common Patterns to Flag

  **SQL Injection**:
  ```javascript
  // BAD
  const query = `SELECT * FROM users WHERE name = '${name}'`;

  // GOOD
  const query = 'SELECT * FROM users WHERE name = ?';
  ```

  **XSS**:
  ```javascript
  // BAD
  element.innerHTML = userInput;

  // GOOD
  element.textContent = userInput;
  ```

  **Hardcoded Secrets**:
  ```javascript
  // BAD
  const apiKey = "sk_live_abc123xyz";

  // GOOD
  const apiKey = process.env.API_KEY;
  ```

  **Missing Authentication**:
  ```javascript
  // BAD
  app.get('/admin/users', (req, res) => { ... });

  // GOOD
  app.get('/admin/users', requireAuth, requireAdmin, (req, res) => { ... });
  ```
```

**Token Count**: ~1,850 tokens (cache-eligible ✓)

---

## Troubleshooting

### Issue: Persona Below Cache Threshold

**Symptom**: Warning message: `Persona 'code-style' below cache threshold: 823 tokens (min: 1024)`

**Diagnosis**:
```typescript
import { countTokens } from './persona-loader.js';
const tokens = countTokens(persona.prompt);
console.log(`Current: ${tokens} tokens, Need: ${1024 - tokens} more`);
```

**Solutions**:
1. Add detailed methodology section
2. Include concrete examples
3. Expand output format documentation
4. Add background/expertise description

### Issue: Low Cache Hit Rate

**Symptom**: Cache metrics show <80% hit rate

**Diagnosis**:
```bash
multi-persona-review src/ --show-cache-metrics
# Check cache hit rate per persona
```

**Possible Causes**:
1. **Dynamic Content in Prompt**: Remove template variables
2. **Frequent Persona Updates**: Version thrashing
3. **Short TTL**: Reviews spaced >5 minutes apart
4. **Different Persona Instances**: Check cache key consistency

**Solutions**:
1. Ensure prompts are fully static
2. Batch reviews within 5-minute windows
3. Verify same persona version used across reviews

### Issue: Cache Invalidation After Deployment

**Symptom**: Cache miss rate spikes after persona update

**Expected Behavior**: First review after update is cache miss, subsequent reviews hit cache

**Verification**:
```bash
# First review (cache miss)
multi-persona-review src/auth.ts --show-cache-metrics
# Cache Performance: 0% hit rate (0 hits, 1 miss)

# Second review (cache hit)
multi-persona-review src/db.ts --show-cache-metrics
# Cache Performance: 50% hit rate (1 hit, 1 miss)
```

---

## Monitoring and Metrics

### Recommended Metrics

1. **Cache Hit Rate**: `cacheHits / (cacheHits + cacheMisses)` - Target: ≥80%
2. **Cost Savings**: `(costWithoutCache - costWithCache) / costWithoutCache` - Target: ≥85%
3. **Token Efficiency**: `cacheReadTokens / totalInputTokens` - Higher is better
4. **Cache Coverage**: `cacheEligiblePersonas / totalPersonas` - Target: 100%

### GCP Monitoring

**Custom Metrics** (if using GCP cost sink):
```yaml
costTracking:
  type: gcp
  projectId: my-project
  metricType: custom.googleapis.com/ai/review_cache_performance
```

**Queries**:
- Cache hit rate: `cache_read_tokens / (cache_read_tokens + input_tokens)`
- Cost savings: `(cache_creation_tokens * 0.30 + cache_read_tokens * 0.03) / baseline_cost`

See [docs/cache-metrics-dashboard.md](./cache-metrics-dashboard.md) for dashboard setup.

---

## References

- **ADR 002**: [Sub-Agent Caching Architecture](./adr/002-sub-agent-caching-architecture.md)
- **Implementation**: [CACHE_METRICS_IMPLEMENTATION.md](../CACHE_METRICS_IMPLEMENTATION.md)
- **Source Code**: [src/persona-loader.ts](../src/persona-loader.ts)
- **Tests**: [tests/unit/persona-loader.test.ts](../tests/unit/persona-loader.test.ts)

---

## Changelog

- **v1.0.0** (2026-02-23): Initial guide based on Phase 2 validation (99.5% hit rate)
