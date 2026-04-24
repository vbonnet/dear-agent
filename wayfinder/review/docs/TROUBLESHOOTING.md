# Troubleshooting Guide

Common issues and solutions for Multi-Persona Review.

---

## Table of Contents

- [Installation Issues](#installation-issues)
- [API / Authentication Issues](#api--authentication-issues)
- [Persona Issues](#persona-issues)
- [Performance Issues](#performance-issues)
- [Cost Issues](#cost-issues)
- [Output Issues](#output-issues)
- [CI/CD Issues](#cicd-issues)
- [Error Codes Reference](#error-codes-reference)

---

## Installation Issues

### npm install fails

**Symptoms**:
```
npm ERR! code EACCES
npm ERR! Permission denied
```

**Solutions**:

1. **Use sudo** (not recommended):
   ```bash
   sudo npm install -g @wayfinder/multi-persona-review
   ```

2. **Fix npm permissions** (recommended):
   ```bash
   mkdir ~/.npm-global
   npm config set prefix '~/.npm-global'
   echo 'export PATH=~/.npm-global/bin:$PATH' >> ~/.bashrc
   source ~/.bashrc
   npm install -g @wayfinder/multi-persona-review
   ```

3. **Use npx** (no install needed):
   ```bash
   npx @wayfinder/multi-persona-review src/
   ```

### Command not found after install

**Symptoms**:
```
bash: multi-persona-review: command not found
```

**Solutions**:

1. **Check PATH**:
   ```bash
   echo $PATH | grep npm
   ```

2. **Find install location**:
   ```bash
   npm list -g @wayfinder/multi-persona-review
   # Note the path, add bin/ to PATH
   ```

3. **Use full path**:
   ```bash
   $(npm bin -g)/multi-persona-review src/
   ```

---

## API / Authentication Issues

### "Missing API key" Error

**Symptoms**:
```
Error: ANTHROPIC_API_KEY environment variable not set
```

**Solutions**:

1. **Set environment variable**:
   ```bash
   export ANTHROPIC_API_KEY=sk-ant-api03-...
   ```

2. **Add to shell profile** (persist across sessions):
   ```bash
   echo 'export ANTHROPIC_API_KEY=sk-ant-api03-...' >> ~/.bashrc
   source ~/.bashrc
   ```

3. **Use .env file** (with dotenv):
   ```bash
   echo 'ANTHROPIC_API_KEY=sk-ant-api03-...' > .env
   ```

4. **Pass via CLI**:
   ```bash
   multi-persona-review src/ --api-key sk-ant-api03-...
   ```

### "Invalid API key" Error

**Symptoms**:
```
Error: 401 Unauthorized - Invalid API key
```

**Solutions**:

1. **Verify API key** is correct:
   ```bash
   curl https://api.anthropic.com/v1/messages \
     -H "x-api-key: $ANTHROPIC_API_KEY" \
     -H "anthropic-version: 2023-06-01" \
     -H "content-type: application/json" \
     -d '{"model":"claude-3-5-sonnet-20241022","max_tokens":10,"messages":[{"role":"user","content":"test"}]}'
   ```

2. **Regenerate API key** at https://console.anthropic.com/

3. **Check for trailing whitespace**:
   ```bash
   echo -n $ANTHROPIC_API_KEY | wc -c
   # Should be exactly 108 characters
   ```

### Vertex AI authentication fails

**Symptoms**:
```
Error: Application Default Credentials not found
```

**Solutions**:

1. **Authenticate with gcloud**:
   ```bash
   gcloud auth application-default login
   ```

2. **Set service account key**:
   ```bash
   export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json
   ```

3. **Verify project ID**:
   ```bash
   gcloud config get-value project
   # Should match VERTEX_PROJECT_ID
   ```

4. **Enable Vertex AI API**:
   ```bash
   gcloud services enable aiplatform.googleapis.com --project YOUR_PROJECT_ID
   ```

---

## Persona Issues

### "No personas found" Warning

**Symptoms**:
```
Warning: No personas found in search paths
Using default personas from built-in library
```

**Solutions**:

1. **List available personas**:
   ```bash
   multi-persona-review --list-personas
   ```

2. **Check search paths**:
   ```bash
   multi-persona-review --verbose src/ 2>&1 | grep "Searching for personas"
   ```

3. **Verify persona files exist**:
   ```bash
   ls -la ~/.wayfinder/personas/
   ls -la .wayfinder/personas/
   ```

4. **Check file permissions**:
   ```bash
   chmod 644 ~/.wayfinder/personas/*.ai.md
   ```

### Persona file parsing fails

**Symptoms**:
```
Error: Failed to parse persona file: invalid YAML
```

**Solutions**:

1. **Validate YAML syntax**:
   ```bash
   yamllint ~/.wayfinder/personas/my-persona.ai.md
   ```

2. **Check frontmatter delimiters**:
   ```markdown
   ---
   name: my-persona
   ...
   ---
   ```
   (Must be exactly three dashes)

3. **Verify required fields**:
   - `name` (kebab-case)
   - `displayName`
   - `version` (semver)
   - `description`
   - `focusAreas` (array)

4. **Test parsing**:
   ```bash
   multi-persona-review --verbose --dry-run src/ 2>&1 | grep "Loaded persona"
   ```

### Persona not cache-eligible

**Symptoms**:
```
Warning: Persona 'my-persona' below cache threshold: 512 tokens (min: 1024)
```

**Solutions**:

1. **Check token count**:
   ```bash
   wc -c < .wayfinder/personas/my-persona.ai.md
   # Divide by 4 to estimate tokens
   # Need ≥4,096 characters (≥1,024 tokens)
   ```

2. **Expand persona content**:
   - Add more examples
   - Detailed methodology
   - Comprehensive checklist
   - Code snippets

3. **Disable caching** (if small persona is intentional):
   ```bash
   multi-persona-review src/ --no-cache
   ```

---

## Performance Issues

### Reviews are too slow

**Symptoms**:
Reviews taking >30 seconds for small files

**Solutions**:

1. **Enable parallel execution** (default):
   ```bash
   multi-persona-review src/ --parallel
   ```

2. **Reduce persona count**:
   ```bash
   multi-persona-review src/ --personas security-engineer,tech-lead
   ```

3. **Use quick mode**:
   ```bash
   multi-persona-review src/ --mode quick
   ```

4. **Review changed files only**:
   ```bash
   multi-persona-review src/ --scan changed
   ```

5. **Use faster model**:
   ```bash
   multi-persona-review src/ --model claude-3-5-haiku-20241022
   ```

### High memory usage

**Symptoms**:
```
FATAL ERROR: Reached heap limit Allocation failed - JavaScript heap out of memory
```

**Solutions**:

1. **Increase Node.js heap size**:
   ```bash
   export NODE_OPTIONS="--max-old-space-size=4096"
   multi-persona-review src/
   ```

2. **Review fewer files at once**:
   ```bash
   multi-persona-review src/api/
   multi-persona-review src/db/
   ```

3. **Disable deduplication** (uses less memory):
   ```bash
   multi-persona-review src/ --no-dedupe
   ```

---

## Cost Issues

### Costs are too high

**Symptoms**:
Reviews costing >$1 per run

**Solutions**:

1. **Use caching** (86% savings):
   ```bash
   export MULTI_PERSONA_BATCH_MODE=true
   multi-persona-review src/ --show-cache-metrics
   ```

2. **Use cheaper model**:
   ```bash
   # Haiku: $0.25 / $1.25 per 1M tokens
   multi-persona-review src/ --model claude-3-5-haiku-20241022

   # Gemini Flash: $0.075 / $0.30 per 1M tokens
   multi-persona-review src/ --provider vertexai --model gemini-2.5-flash
   ```

3. **Review changed files only**:
   ```bash
   multi-persona-review src/ --scan changed --mode quick
   ```

4. **Reduce persona count**:
   ```bash
   multi-persona-review src/ --personas security-engineer
   ```

5. **Track costs to identify patterns**:
   ```bash
   multi-persona-review src/ --cost-sink file --cost-file costs.jsonl
   cat costs.jsonl | jq '.totalCost' | awk '{sum+=$1} END {print sum}'
   ```

### Cache not working

**Symptoms**:
```
Cache Performance:
  Persona: security-engineer | Hits: 0 | Miss: 5 | Hit Rate: 0%
```

**Solutions**:

1. **Enable batch mode**:
   ```bash
   export MULTI_PERSONA_BATCH_MODE=true
   ```

2. **Review within 5-minute window**:
   ```bash
   multi-persona-review src/file1.ts
   # Immediately review next file (< 5 minutes)
   multi-persona-review src/file2.ts  # Cache hit!
   ```

3. **Ensure personas are cache-eligible** (≥1,024 tokens):
   ```bash
   multi-persona-review --list-personas | grep tokens
   ```

4. **Use same personas consistently**:
   ```bash
   # Good: Same personas = cache hits
   multi-persona-review src/api.ts --personas security-engineer,tech-lead
   multi-persona-review src/db.ts --personas security-engineer,tech-lead

   # Bad: Different personas = cache miss
   multi-persona-review src/api.ts --personas security-engineer
   multi-persona-review src/db.ts --personas performance-reviewer
   ```

---

## Output Issues

### No color in output

**Symptoms**:
Output is plain text, no ANSI colors

**Solutions**:

1. **Enable colors explicitly**:
   ```bash
   multi-persona-review src/ --colors
   ```

2. **Check terminal support**:
   ```bash
   echo $TERM
   # Should be xterm-256color or similar
   ```

3. **Force color mode**:
   ```bash
   FORCE_COLOR=1 multi-persona-review src/
   ```

### JSON output is invalid

**Symptoms**:
```
Error: Unexpected token in JSON at position 0
```

**Solutions**:

1. **Ensure clean JSON output**:
   ```bash
   multi-persona-review src/ --format json --no-colors > review.json
   ```

2. **Redirect stderr separately**:
   ```bash
   multi-persona-review src/ --format json 2>/dev/null > review.json
   ```

3. **Validate JSON**:
   ```bash
   jq . < review.json
   ```

### GitHub format not rendering

**Symptoms**:
Markdown not displaying properly in PR comments

**Solutions**:

1. **Use GitHub format explicitly**:
   ```bash
   multi-persona-review src/ --format github > comment.md
   ```

2. **Check for Unicode issues**:
   ```bash
   file comment.md
   # Should be: UTF-8 Unicode text
   ```

3. **Preview markdown**:
   ```bash
   glow comment.md  # Using glow CLI
   ```

---

## CI/CD Issues

### GitHub Actions workflow fails

**Symptoms**:
```
Error: Process completed with exit code 1
```

**Solutions**:

1. **Check secrets are set**:
   - Go to: Repo → Settings → Secrets → Actions
   - Verify `ANTHROPIC_API_KEY` exists

2. **Allow failures for findings**:
   ```yaml
   - name: Run Review
     run: multi-persona-review src/ || true  # Don't fail workflow
   ```

3. **Check logs**:
   ```bash
   gh run view --log  # Using GitHub CLI
   ```

4. **Test locally first**:
   ```bash
   export ANTHROPIC_API_KEY=...
   multi-persona-review src/ --format github
   ```

### Cost too high in CI/CD

**Symptoms**:
CI/CD costs >$10/month

**Solutions**:

1. **Review changed files only**:
   ```yaml
   run: multi-persona-review src/ --scan changed
   ```

2. **Use quick mode**:
   ```yaml
   run: multi-persona-review src/ --mode quick
   ```

3. **Run on PR only** (not every push):
   ```yaml
   on:
     pull_request:
       branches: [main]
   # Remove: push
   ```

4. **Use Haiku model**:
   ```yaml
   run: multi-persona-review src/ --model claude-3-5-haiku-20241022
   ```

---

## Error Codes Reference

### CONFIG_xxxx

- **CONFIG_1001**: Configuration file not found
  - Solution: Run `multi-persona-review init`

- **CONFIG_1002**: Invalid YAML syntax
  - Solution: Validate `.engram/config.yml` with yamllint

- **CONFIG_1003**: Missing required field
  - Solution: Add required fields to config

### PERSONA_xxxx

- **PERSONA_2001**: Persona file not found
  - Solution: Check persona search paths

- **PERSONA_2002**: Invalid persona format
  - Solution: Validate frontmatter YAML

- **PERSONA_2003**: Missing required persona field
  - Solution: Add `name`, `version`, `description`, `focusAreas`

- **PERSONA_2004**: Persona below cache threshold
  - Solution: Expand persona to ≥1,024 tokens

### API_xxxx

- **API_3001**: API key not set
  - Solution: Export `ANTHROPIC_API_KEY`

- **API_3002**: Invalid API key
  - Solution: Regenerate key at console.anthropic.com

- **API_3003**: Rate limit exceeded
  - Solution: Reduce concurrency or wait

- **API_3004**: Model not found
  - Solution: Check model name spelling

### REVIEW_xxxx

- **REVIEW_4001**: No files to review
  - Solution: Check file paths and glob patterns

- **REVIEW_4002**: Git repository not found
  - Solution: Run from git repository root

- **REVIEW_4003**: Review timeout
  - Solution: Increase timeout or reduce scope

---

## Getting Help

If your issue isn't covered here:

1. **Check documentation**:
   - [README.md](../README.md)
   - [Getting Started](GETTING-STARTED.md)
   - [Advanced Usage](ADVANCED-USAGE.md)
   - [API Documentation](API.md)

2. **Search issues**:
   - https://github.com/wayfinder/multi-persona-review/issues

3. **Ask questions**:
   - GitHub Discussions: https://github.com/wayfinder/multi-persona-review/discussions

4. **Report bugs**:
   - Use bug report template: https://github.com/wayfinder/multi-persona-review/issues/new?template=bug_report.md

5. **Debug mode**:
   ```bash
   multi-persona-review --verbose src/ 2>&1 | tee debug.log
   ```

---

## Diagnostic Checklist

Before reporting issues, gather this information:

```bash
# Version information
multi-persona-review --version
node --version
npm --version

# Environment
echo $ANTHROPIC_API_KEY | wc -c  # Should be 108
echo $VERTEX_PROJECT_ID
echo $NODE_OPTIONS

# Persona availability
multi-persona-review --list-personas

# Test with dry-run
multi-persona-review --dry-run --verbose src/ 2>&1 | head -50

# Check configuration
cat .engram/config.yml

# Check file permissions
ls -la .engram/personas/

# Test API connectivity
curl -I https://api.anthropic.com/v1/messages
```

Include this output when reporting issues for faster resolution.
