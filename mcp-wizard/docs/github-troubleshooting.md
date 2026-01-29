# GitHub MCP Troubleshooting Guide

**Version:** 0.1.0 (Beta)
**Last Updated:** 2026-01-29

---

## Quick Reference

**Before asking for help:**
1. Check this guide for your issue
2. Verify your GitHub PAT has correct scopes
3. Test GitHub API connectivity
4. Check Claude Code MCP server status

---

## Common Issues

### Authentication Issues

#### "Invalid GitHub PAT format"

**Error Message:**
```
❌ Invalid GitHub PAT format. Tokens must start with "ghp_" (classic) or "github_pat_" (fine-grained).
Generate a PAT at https://github.com/settings/tokens/new
```

**Cause:** GitHub Personal Access Token doesn't match expected format

**Solution:**
1. **Classic PAT (starts with `ghp_`):**
   - Visit: https://github.com/settings/tokens/new
   - Click "Generate new token (classic)"
   - Select required scopes (see below)
   - Copy token immediately (won't be shown again)

2. **Fine-grained PAT (starts with `github_pat_`):**
   - Visit: https://github.com/settings/personal-access-tokens/new
   - Configure repository access and permissions
   - Generate and copy token

3. **Re-run setup:**
   ```bash
   mcp-wizard setup
   # Select GitHub MCP
   # Paste new token when prompted
   ```

---

#### "Token appears invalid (too short)"

**Error Message:**
```
❌ Token appears invalid (too short). GitHub PATs are typically 40+ characters.
```

**Cause:** Token is incomplete or corrupted (missing characters)

**Common Mistakes:**
- Copied only part of the token
- Included extra spaces or newlines
- Token expired and was revoked

**Solution:**
1. **Generate new token:**
   ```bash
   # Visit token generation page
   open https://github.com/settings/tokens/new
   ```

2. **Copy carefully:**
   - Click "Copy" button (don't manually select)
   - Verify no spaces at start/end
   - Classic PAT: typically 40 characters (`ghp_` + 36 chars)
   - Fine-grained PAT: typically 80+ characters (`github_pat_` + ...)

3. **Re-run setup with new token**

---

#### "Missing required scopes: repo, read:org"

**Error Message:**
```
❌ GitHub MCP requires scopes: repo, read:org
Your token is missing: read:org
```

**Cause:** PAT created without required permissions

**Required Scopes (MUST HAVE):**
- `repo` - Full repository access (read/write code, issues, PRs)
- `read:org` - Read organization data (team membership, org repos)

**Optional Scopes (RECOMMENDED):**
- `read:packages` - Access GitHub Packages
- `workflow` - Access GitHub Actions workflows

**Solution:**
1. **Check current token scopes:**
   - Visit: https://github.com/settings/tokens
   - Find your token (named "mcp-wizard" or similar)
   - Check "Scopes" column

2. **If scopes are wrong:**
   ```bash
   # Delete old token (can't edit scopes)
   # Visit: https://github.com/settings/tokens
   # Click token → Delete

   # Generate new token with correct scopes
   # Visit: https://github.com/settings/tokens/new
   # ✅ Check: repo
   # ✅ Check: read:org
   # ✅ Check: read:packages (optional)
   # ✅ Check: workflow (optional)

   # Generate token → Copy

   # Re-run setup
   mcp-wizard setup
   ```

---

### Enterprise Server Issues

#### "Enterprise URL must start with https://"

**Error Message:**
```
❌ Enterprise URL must start with https://
```

**Cause:** Entered HTTP URL instead of HTTPS

**Solution:**
1. **Use HTTPS:**
   ```
   ✅ Correct: https://github.company.com
   ❌ Wrong:   http://github.company.com
   ❌ Wrong:   github.company.com (missing protocol)
   ```

2. **Verify URL:**
   ```bash
   # Test connectivity
   curl -I https://github.company.com
   # Should return HTTP 200 or 302
   ```

3. **Re-run setup with correct URL**

---

#### "Cannot connect to GitHub Enterprise Server"

**Cause:** Network connectivity issue, firewall, or VPN required

**Check Connectivity:**
```bash
# Test basic connectivity
ping github.company.com

# Test HTTPS
curl -I https://github.company.com/api/v3

# Expected: HTTP 200 OK or 302 redirect
```

**Common Causes:**
1. **VPN required** - Connect to corporate VPN first
2. **Firewall blocking** - Check with IT/security team
3. **Wrong URL** - Verify exact URL with GitHub admin
4. **SSL certificate issues** - Enterprise cert not trusted

**Solution:**
1. **If VPN required:**
   ```bash
   # Connect to VPN first
   # Then run setup
   mcp-wizard setup
   ```

2. **If firewall issue:**
   - Contact IT to whitelist GitHub Enterprise domain
   - Verify ports 443 (HTTPS) and 22 (SSH) are open

3. **If SSL cert issue:**
   ```bash
   # Check cert
   openssl s_client -connect github.company.com:443 -showcerts

   # If self-signed, may need to install cert
   # Contact IT for cert installation instructions
   ```

---

### Feature Selection Issues

#### "Please select at least one feature"

**Error Message:**
```
❌ Please select at least one feature
```

**Cause:** No features selected during setup

**Solution:**
1. **Select at minimum:**
   - ✅ Repositories (recommended - core functionality)
   - ✅ Issues (recommended - issue tracking)

2. **Optional features:**
   - Pull Requests (PR review and status)
   - GitHub Actions (workflow monitoring)
   - Code Security (vulnerability scanning)

3. **Re-run setup and select features using spacebar**

---

#### "GitHub MCP not appearing in Claude Code"

**Cause:** Configuration not loaded, MCP server not started, or Claude Code not restarted

**Solution:**
1. **Verify config exists:**
   ```bash
   cat ~/.config/claude-code/mcp.json
   ```
   Should contain `"github"` section

2. **Verify config is valid JSON:**
   ```bash
   cat ~/.config/claude-code/mcp.json | jq .
   ```
   If error: config is malformed

3. **Check token stored:**
   ```bash
   ls -la ~/mcp-servers/github-mcp/.github-token
   # Should exist with permissions: -rw------- (600)
   ```

4. **Restart Claude Code:**
   - Exit Claude Code completely (Cmd+Q / Ctrl+Q)
   - Start Claude Code again
   - MCP should load automatically

5. **Check Claude Code logs:**
   ```bash
   # macOS
   tail -f ~/Library/Logs/Claude/mcp.log

   # Linux
   tail -f ~/.config/Claude/logs/mcp.log
   ```
   Look for GitHub MCP startup messages or errors

---

### Token Storage Issues

#### "Token file has wrong permissions"

**Error Message:**
```
⚠️  Token file has wrong permissions (644 instead of 600)
```

**Cause:** File created with default permissions (world-readable)

**Security Risk:** Token may be accessible to other users on system

**Solution:**
1. **Fix permissions immediately:**
   ```bash
   chmod 600 ~/mcp-servers/github-mcp/.github-token
   ```

2. **Verify:**
   ```bash
   ls -l ~/mcp-servers/github-mcp/.github-token
   # Should show: -rw------- (owner read/write only)
   ```

3. **If already compromised:**
   - Revoke old token: https://github.com/settings/tokens
   - Generate new token
   - Re-run setup

---

#### "Token tracked in git"

**Cause:** `.gitignore` not applied or token added before `.gitignore`

**Security Risk:** ⚠️  CRITICAL - Token exposed in git history

**Check:**
```bash
cd ~/mcp-servers/github-mcp
git status
```

**If `.github-token` appears:**

1. **If staged but NOT committed:**
   ```bash
   git reset .github-token
   git status  # Verify not staged
   ```

2. **If already committed (CRITICAL):**
   ```bash
   # Remove from git history
   git filter-branch --index-filter \
     'git rm --cached --ignore-unmatch .github-token' HEAD

   # Force push (if pushed to remote)
   git push origin --force

   # REVOKE TOKEN IMMEDIATELY
   # Visit: https://github.com/settings/tokens
   # Delete the compromised token

   # Generate new token and re-run setup
   ```

3. **Verify `.gitignore`:**
   ```bash
   cat ~/mcp-servers/github-mcp/.gitignore
   ```
   Should contain:
   ```
   .github-token
   .github-enterprise-url
   .github-toolsets
   ```

---

### OAuth Issues (VS Code 1.101+)

#### "OAuth requires VS Code 1.101+"

**Error Message:**
```
⚠️  OAuth requires VS Code 1.101+ (detected: 1.95.3)
Falling back to PAT authentication
```

**Cause:** VS Code version too old for OAuth support

**Solution:**
1. **Check VS Code version:**
   ```bash
   code --version
   # Example output: 1.101.0
   ```

2. **If < 1.101:**
   - **Option A:** Upgrade VS Code:
     ```bash
     # macOS (Homebrew)
     brew upgrade --cask visual-studio-code

     # Linux (manual)
     # Download from https://code.visualstudio.com/
     ```
   - **Option B:** Use PAT authentication (works on any version)
     - Select PAT when prompted during setup

3. **Re-run setup after upgrade**

---

#### "OAuth flow not yet implemented"

**Error Message:**
```
⚠️  OAuth flow not yet implemented - falling back to PAT
```

**Cause:** OAuth feature detected but not fully implemented yet

**Solution:**
- **Current:** Use PAT authentication (fully supported)
- **Future:** OAuth support planned for future release
- No action needed - setup will continue with PAT

---

## Diagnostic Commands

### Check GitHub API Connectivity

```bash
# Test GitHub.com API
curl -I https://api.github.com
# Expected: HTTP 200 OK

# Test with token (replace TOKEN)
curl -H "Authorization: token ghp_YOUR_TOKEN" https://api.github.com/user
# Expected: JSON with your user info

# Test GitHub Enterprise (replace URL)
curl -I https://github.company.com/api/v3
# Expected: HTTP 200 OK
```

### Check Token Validity

```bash
# Verify token format
cat ~/mcp-servers/github-mcp/.github-token
# Should start with ghp_ or github_pat_

# Test token with GitHub API
TOKEN=$(cat ~/mcp-servers/github-mcp/.github-token)
curl -H "Authorization: token $TOKEN" https://api.github.com/user
# Expected: JSON with your user info
# Error: 401 Unauthorized = token invalid/expired
```

### Check MCP Configuration

```bash
# Verify GitHub MCP in config
cat ~/.config/claude-code/mcp.json | jq '.mcpServers.github'

# Check token file permissions
ls -l ~/mcp-servers/github-mcp/.github-token
# Expected: -rw------- (600)

# Check feature selection
cat ~/mcp-servers/github-mcp/.github-toolsets
# Expected: comma-separated list (e.g., "repos,issues,pull_requests")

# Check Enterprise URL (if configured)
cat ~/mcp-servers/github-mcp/.github-enterprise-url 2>/dev/null
# Expected: URL or file not found (if using GitHub.com)
```

### Check Claude Code MCP Status

```bash
# List active MCP servers (requires Claude Code CLI)
claude mcp list
# Expected: GitHub MCP in list

# Check MCP logs
tail -f ~/Library/Logs/Claude/mcp.log  # macOS
tail -f ~/.config/Claude/logs/mcp.log  # Linux
```

---

## How to Rotate GitHub Token

**When to rotate:**
- Token accidentally committed to git
- Token file exposed (wrong permissions, shared accidentally)
- Suspected compromise
- Best practice: rotate every 90 days

**Steps:**

1. **Revoke old token:**
   - Visit: https://github.com/settings/tokens
   - Find your token (named "mcp-wizard" or check creation date)
   - Click "Delete" → Confirm

2. **Generate new token:**
   - Visit: https://github.com/settings/tokens/new
   - Token description: "mcp-wizard - [current date]"
   - Select scopes: `repo`, `read:org` (+ optional)
   - Click "Generate token"
   - Copy immediately (won't be shown again)

3. **Delete old token locally:**
   ```bash
   rm ~/mcp-servers/github-mcp/.github-token
   ```

4. **Re-run setup:**
   ```bash
   mcp-wizard setup
   # Select GitHub MCP
   # Paste new token when prompted
   ```

5. **Verify new token works:**
   ```bash
   # Test with GitHub API
   TOKEN=$(cat ~/mcp-servers/github-mcp/.github-token)
   curl -H "Authorization: token $TOKEN" https://api.github.com/user
   # Should return your user info
   ```

6. **Restart Claude Code to reload config**

---

## Getting Help

### Before Asking for Help

1. **Check this troubleshooting guide** (you're here!)
2. **Test GitHub API connectivity** (see diagnostic commands above)
3. **Verify token format and scopes**
4. **Check Claude Code MCP logs**

### When Asking for Help

**Include this information:**

1. **Your environment:**
   ```bash
   # Copy-paste output of these commands:
   node --version
   code --version  # If using VS Code
   uname -a        # OS version
   ```

2. **GitHub setup type:**
   - [ ] GitHub.com
   - [ ] GitHub Enterprise Server (specify URL)

3. **Token type:**
   - [ ] Classic PAT (`ghp_...`)
   - [ ] Fine-grained PAT (`github_pat_...`)

4. **Error message** (copy-paste, not screenshot):
   ```
   [Paste error here]
   ```

5. **Diagnostic output:**
   ```bash
   # Test GitHub API
   curl -I https://api.github.com

   # Check config
   cat ~/.config/claude-code/mcp.json | jq '.mcpServers.github'

   # Check token file
   ls -l ~/mcp-servers/github-mcp/.github-token
   ```

6. **What you've tried:**
   - List troubleshooting steps you've already attempted

---

## FAQ

### General

**Q: Which GitHub features does the MCP support?**
A:
- ✅ Repositories (file search, navigation)
- ✅ Issues (search, create, comment)
- ✅ Pull Requests (review, status, comment)
- ✅ GitHub Actions (workflow monitoring)
- ✅ Code Security (vulnerability scanning)

**Q: Does it work with GitHub Enterprise?**
A: ✅ YES - Both GitHub.com and GitHub Enterprise Server are supported.

**Q: Which authentication methods are supported?**
A:
- ✅ Personal Access Token (PAT) - Classic and Fine-grained
- ⏸️  OAuth (VS Code 1.101+) - Experimental, falls back to PAT

### Authentication

**Q: Do I need a GitHub account?**
A: ✅ YES - GitHub.com account or GitHub Enterprise account.

**Q: Can I use fine-grained PATs?**
A: ✅ YES - Both classic (`ghp_...`) and fine-grained (`github_pat_...`) tokens work.

**Q: How often do PATs expire?**
A: Depends on your token settings:
- No expiration (if configured)
- 30/60/90 days (if configured)
- Check expiration: https://github.com/settings/tokens

**Q: Can I share my PAT with my team?**
A: ❌ NO - Each user needs their own token (security best practice).

### Troubleshooting

**Q: Setup succeeded but GitHub MCP not showing in Claude Code?**
A:
1. Restart Claude Code completely
2. Check config: `cat ~/.config/claude-code/mcp.json | jq .`
3. Check Claude Code logs for errors

**Q: How do I re-authenticate?**
A: Run setup again - it will prompt for new token:
```bash
mcp-wizard setup
# Select GitHub MCP
# Enter new token
```

**Q: Can I reset everything and start over?**
A: ✅ YES:
```bash
# Delete GitHub MCP config
rm -rf ~/mcp-servers/github-mcp

# Re-run setup
mcp-wizard setup
```

---

## Known Issues (Beta)

### P1 Issues (Workaround Available)

**Issue:** OAuth not fully implemented
- **Workaround:** Use PAT authentication (fully supported)
- **Fix:** Planned for future release

**Issue:** No token expiration monitoring
- **Workaround:** Manually check token expiration at https://github.com/settings/tokens
- **Fix:** Planned for future release (automatic expiration warnings)

### P2 Issues (Minor)

**Issue:** No visual feedback during token validation
- **Workaround:** Wait for validation to complete (~2-5 seconds)
- **Fix:** Planned for future release (spinner/progress indicator)

---

## Feedback

**Found an issue not listed here?**
- File GitHub issue: https://github.com/[REDACTED_EMPLOYER]-src/ai-tools/issues
- Label: `mcp-wizard`, `github-mcp`, `beta`

**Suggestions for this guide?**
- Submit PR or open issue

---

**Document Version:** 1.0 (Beta)
**Last Updated:** 2026-01-29
**Maintained By:** MCP Wizard Team
