# Troubleshooting Guide - [REDACTED_EMPLOYER] MCP Setup Tool

**Version:** 0.1.0 (Beta)
**Last Updated:** 2025-12-07

---

## Quick Reference

**Before asking for help:**
1. Check this guide for your issue
2. Check setup logs for specific error messages
3. Search #vida-dev Slack history
4. If still stuck, ask in #vida-dev

**Get help:**
- **Slack:** #vida-dev (fastest response)
- **Response Time:** P0 < 4 hours, P1 < 24 hours

---

## Common Issues

### Setup Issues

#### "setup failed: not a work machine"

**Cause:** Hostname doesn't end with `-w`

**Check:**
```bash
hostname
```

**Expected:** Something like `alice-w` or `bob-w`

**Solution:**
- ✅ Run on [REDACTED_EMPLOYER] work machine only (security requirement)
- ❌ DO NOT run on personal laptop or remote machine

---

#### "setup failed: Node.js version too old"

**Cause:** Node.js < 18.0.0

**Check:**
```bash
node --version
```

**Expected:** v18.0.0 or higher (e.g., v18.19.0, v20.0.0, v24.9.0)

**Solution:**
1. Install Node.js 18+ using nvm (recommended):
   ```bash
   # Install nvm (if not installed)
   curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.0/install.sh | bash

   # Install Node.js 18
   nvm install 18
   nvm use 18

   # Verify
   node --version
   ```

2. Or download from https://nodejs.org/ (get LTS version)

---

#### "npm install failed"

**Cause:** Network issues, missing dependencies, or npm cache corruption

**Solution:**
1. **Check internet connection:**
   ```bash
   ping google.com
   ```

2. **Try with verbose output:**
   ```bash
   npm install --verbose
   ```

3. **Clear npm cache and retry:**
   ```bash
   npm cache clean --force
   npm install
   ```

4. **If still fails, ask in #vida-dev** with error message

---

#### "npm run build failed"

**Cause:** TypeScript compilation errors or missing dependencies

**Solution:**
1. **Try with verbose output:**
   ```bash
   npm run build --verbose
   ```

2. **Ensure dependencies are installed:**
   ```bash
   npm install
   npm run build
   ```

3. **If still fails, this may be a bug:**
   - Copy error message
   - Report in #vida-dev
   - Include output of: `npm --version` and `node --version`

---

### OAuth Issues

#### "OAuth timeout (5 minutes expired)"

**Cause:** User took too long to complete GCP Console steps

**Solution:**
1. **Resume setup** (state is saved):
   ```bash
   node bin/mcp-wizard.js setup --resume
   ```

2. **Before starting, prepare:**
   - Have GCP Console open: https://console.cloud.google.com/apis/credentials
   - Read OAuth setup guide first
   - Have 10-15 minutes free (no interruptions)

3. **Alternative:** Run setup with `--skip-auth`, then run setup again later to complete OAuth:
   ```bash
   node bin/mcp-wizard.js setup --skip-auth
   # Later, run setup again to complete OAuth:
   node bin/mcp-wizard.js setup --resume
   ```

---

#### "Invalid credentials.json format"

**Cause:** Downloaded wrong file from GCP Console

**Check:**
```bash
cat ~/mcp-servers/google-docs-mcp/credentials.json
```

**Expected:** File should contain `"installed"` section (NOT `"web"`):
```json
{
  "installed": {
    "client_id": "...",
    "client_secret": "...",
    ...
  }
}
```

**Solution:**
1. **Delete wrong file:**
   ```bash
   rm ~/mcp-servers/google-docs-mcp/credentials.json
   ```

2. **Re-download from GCP Console:**
   - Go to: https://console.cloud.google.com/apis/credentials
   - Select project: `shared-dev-ai-pct45x`
   - Find OAuth client (type: "Desktop app")
   - Click download icon (JSON)
   - Save as `credentials.json`

3. **Verify file:**
   ```bash
   grep "installed" ~/mcp-servers/google-docs-mcp/credentials.json
   ```
   Should show: `"installed": {`

4. **Run setup again:**
   ```bash
   node bin/mcp-wizard.js setup --resume
   ```

---

#### "Token exchange failed"

**Cause:** Network issues, invalid auth code, or API rate limiting

**Solution:**
1. **Tool will retry 3 times automatically** (wait for retries to complete)

2. **If all retries fail:**
   ```bash
   # Check internet connection
   ping accounts.google.com

   # Run setup again (state is saved)
   node bin/mcp-wizard.js setup --resume
   ```

3. **If still fails:**
   - Verify you pasted correct auth code (check for spaces, newlines)
   - Try generating new auth code (run setup again)
   - Ask #vida-dev for help

---

### Config Issues

#### "Config write failed: permission denied"

**Cause:** No write access to `~/.config/claude-code/` directory

**Check:**
```bash
ls -la ~/.config/ | grep claude-code
```

**Solution:**
1. **Create directory with correct permissions:**
   ```bash
   mkdir -p ~/.config/claude-code
   chmod 755 ~/.config/claude-code
   ```

2. **Run setup again:**
   ```bash
   node bin/mcp-wizard.js setup --resume
   ```

3. **If still fails, check parent directory:**
   ```bash
   ls -la ~/.config/
   ```
   Ensure `.config` directory exists and is writable.

---

#### "MCP server not launching in Claude Code"

**Cause:** Config incorrect, MCP server not built, or Claude Code not restarted

**Solution:**
1. **Verify config exists:**
   ```bash
   cat ~/.config/claude-code/mcp.json
   ```

2. **Verify config is valid JSON:**
   ```bash
   cat ~/.config/claude-code/mcp.json | jq .
   ```
   If error: config is malformed (run repair)

3. **Verify MCP server is built:**
   ```bash
   ls ~/mcp-servers/google-docs-mcp/dist/server.js
   ```
   If missing: MCP installation incomplete

4. **Check setup logs for specific error messages**

5. **Restart Claude Code** (required to load new config):
   - Exit Claude Code completely
   - Start Claude Code again
   - MCP should load automatically

6. **If still fails:**
   - Ask #vida-dev for help
   - Include MCP server path and error message

---

### Permission Issues

#### "Token file has wrong permissions (644 instead of 600)"

**Cause:** File permissions not enforced during token save

**Check:**
```bash
ls -l ~/mcp-servers/google-docs-mcp/token.json
```

**Expected:** `-rw-------` (600) - owner read/write only

**Solution:**
1. **Run repair** (automatic fix):
   ```bash
   node bin/mcp-wizard.js repair
   ```

2. **Or manually:**
   ```bash
   chmod 600 ~/mcp-servers/google-docs-mcp/token.json
   chmod 600 ~/mcp-servers/google-docs-mcp/credentials.json
   ```

3. **Verify:**
   ```bash
   ls -l ~/mcp-servers/google-docs-mcp/*.json
   ```
   Both files should show `-rw-------`

---

#### "Credentials tracked in git"

**Cause:** `.gitignore` not applied before `git add`, or credentials added to git before setup

**Check:**
```bash
cd ~/mcp-servers/google-docs-mcp
git status
```

**Expected:** `credentials.json` and `token.json` should NOT appear in output

**If tracked (WARNING - SECURITY ISSUE):**

1. **If staged but NOT committed:**
   ```bash
   git reset credentials.json token.json
   git status  # Verify not staged
   ```

2. **If already committed (CRITICAL - FIX IMMEDIATELY):**
   ```bash
   # Remove from git history
   git filter-branch --index-filter \
     'git rm --cached --ignore-unmatch credentials.json token.json' HEAD

   # Force push (if already pushed to remote)
   git push origin --force

   # ROTATE CREDENTIALS (compromised)
   # See "How to Rotate OAuth Credentials" section below
   ```

3. **Verify .gitignore:**
   ```bash
   cat ~/mcp-servers/google-docs-mcp/.gitignore
   ```
   Should contain:
   ```
   credentials.json
   token.json
   ```

4. **If .gitignore missing, run setup again:**
   ```bash
   node bin/mcp-wizard.js setup --resume
   ```

---

### Multi-Agent Issues

#### "Config not written to Cursor/Cline/Windsurf"

**Cause:** Agent not selected during setup, or config directory doesn't exist

**Check:**
```bash
# Cursor
ls ~/.cursor/mcp.json

# Cline
ls ~/.cline/mcp.json

# Windsurf
ls ~/.codeium/windsurf/mcp.json
```

**Solution:**
1. **Run setup again and select agents:**
   ```bash
   node bin/mcp-wizard.js setup
   # When prompted, select agents (space to toggle, enter to confirm)
   ```

2. **Or manually copy config:**
   ```bash
   # Example for Cursor
   mkdir -p ~/.cursor
   cp ~/.config/claude-code/mcp.json ~/.cursor/mcp.json
   ```

---

## How to Rotate OAuth Credentials

**When to rotate:**
- Credentials accidentally committed to git
- Token file exposed (wrong permissions, shared accidentally)
- Suspected compromise
- Best practice: rotate every 90 days

**Steps:**

1. **Delete old OAuth client in GCP Console:**
   - Go to: https://console.cloud.google.com/apis/credentials
   - Select project: `shared-dev-ai-pct45x`
   - Find your OAuth client (type: "Desktop app")
   - Click delete (trash icon)
   - Confirm deletion

2. **Create new OAuth client:**
   - Follow GCP Console guide from setup wizard
   - Download new `credentials.json`

3. **Delete old credentials/token locally:**
   ```bash
   rm ~/mcp-servers/google-docs-mcp/credentials.json
   rm ~/mcp-servers/google-docs-mcp/token.json
   ```

4. **Run setup again:**
   ```bash
   node bin/mcp-wizard.js setup
   # Follow OAuth wizard with new credentials.json
   ```

5. **Verify new credentials work:**
   ```bash
   node bin/mcp-wizard.js validate
   ```

---

## Diagnostic Commands

### Check Environment
```bash
# Check you're on work machine
hostname
# Expected: ends with -w

# Check Node.js version
node --version
# Expected: v18.0.0 or higher

# Check npm version
npm --version
# Expected: any version

# Check if running as root (should NOT be)
whoami
# Expected: your username (NOT root)
```

### Check Installation
```bash
# Check MCP server exists
ls ~/mcp-servers/google-docs-mcp/
# Expected: credentials.json, token.json, dist/, node_modules/

# Check MCP server built
ls ~/mcp-servers/google-docs-mcp/dist/server.js
# Expected: file exists

# Check config exists
ls ~/.config/claude-code/mcp.json
# Expected: file exists
```

### Check Permissions
```bash
# Check token/credentials permissions
ls -l ~/mcp-servers/google-docs-mcp/*.json
# Expected: -rw------- (600) for both files

# Check .gitignore
cat ~/mcp-servers/google-docs-mcp/.gitignore
# Expected: contains "credentials.json" and "token.json"
```

### Check MCP Configuration
```bash
# Verify config exists and is valid JSON
cat ~/.config/claude-code/mcp.json | jq .

# Check MCP list in Claude Code (requires Claude Code installed)
claude mcp list
```

### Validation/Repair Commands
**Coming soon:** `validate` and `repair` commands are planned for future release.

---

## Getting Help

### Before Asking for Help

1. **Check this troubleshooting guide** (you're here!)
2. **Check setup logs** for specific error messages
3. **Search Slack history** (#vida-dev)
4. **Check GitHub issues** (vida repo - may be known bug)

### When Asking for Help

**Include this information:**

1. **Your environment:**
   ```bash
   # Copy-paste output of these commands:
   hostname
   node --version
   npm --version
   uname -a
   ```

2. **Command you ran:**
   ```bash
   # Example:
   node bin/mcp-wizard.js setup
   ```

3. **Error message** (copy-paste, not screenshot):
   ```
   [Paste error here]
   ```

4. **Config file contents:**
   ```bash
   cat ~/.config/claude-code/mcp.json
   ```

5. **What you've tried:**
   - List troubleshooting steps you've already attempted

### Support Channels

**Slack:**
- **#vida-dev** (VIDA team, fastest response) ← START HERE
- **#devex** (DevEx team, general questions)
- **#ai-tools** (AI tools community, advice)

**Response Times:**
- **P0 (blocking setup):** < 4 hours
- **P1 (workaround available):** < 24 hours
- **P2 (minor annoyance):** Best effort

**GitHub Issues:**
- Report bugs: https://github.com/[REDACTED_EMPLOYER]-src/vida/issues
- Label: `mcp-setup`, `beta`

---

## FAQ

### General

**Q: How long does setup take?**
A: 10-12 minutes (target), up to 15 minutes for first-time users.

**Q: Can I use this on my personal laptop?**
A: ❌ NO - Security requirement: [REDACTED_EMPLOYER] work machine only (hostname ends with `-w`).

**Q: Which AI agents are supported?**
A: Claude Code, Cursor, Cline, Windsurf. You can select multiple during setup.

**Q: Which MCPs are supported?**
A:
- **GoogleDocs MCP** ✅ Full wizard support with OAuth
- **Atlassian MCP** ✅ Auto-configured (OAuth on first use via mcp-remote)
- **Glean MCP** ⏸️ Requires Glean admin token (contact Glean admin)
- **Slack MCP** ⏸️ Requires workspace admin setup (contact #vida-dev)

### Setup

**Q: Can I resume setup if it fails?**
A: ✅ YES - Run: `node bin/mcp-wizard.js setup --resume`

**Q: Can I run setup multiple times?**
A: ✅ YES - Safe to run multiple times. Existing config will be backed up.

**Q: How do I set up for multiple agents?**
A: Select multiple agents during setup (space to toggle, enter to confirm). Tool writes config to all selected agent directories.

### OAuth

**Q: Do I need a Google Workspace account?**
A: ✅ YES - Use your [REDACTED_EMPLOYER] Google Workspace account (firstname.lastname@[REDACTED_DOMAIN]).

**Q: Where do I get credentials.json?**
A: GCP Console - setup wizard will guide you through creating OAuth client and downloading credentials.json.

**Q: How often do I need to re-authenticate?**
A: Tokens expire after 7 days of inactivity. Tool will prompt you to re-authenticate when needed.

**Q: Can I share credentials.json with my team?**
A: ❌ NO - Each user needs their own OAuth client (security best practice).

### Troubleshooting

**Q: Setup failed. What do I do?**
A:
1. Check setup logs for specific error message
2. Check this troubleshooting guide for your error
3. Ask #vida-dev if still stuck

**Q: How do I fix permission issues?**
A: See "Permission Issues" section above for manual fixes

**Q: How do I re-authenticate?**
A: Run setup again with `--resume` flag to restart OAuth flow

**Q: Can I reset everything and start over?**
A: ✅ YES:
```bash
# Delete everything
rm -rf ~/mcp-servers/google-docs-mcp
rm ~/.config/claude-code/mcp.json  # (and other agent configs)

# Run setup again
node bin/mcp-wizard.js setup
```

---

## Known Issues (Beta)

### P1 Issues (Workaround Available)

**Issue:** OAuth timeout if GCP Console steps take >5 minutes
- **Workaround:** Run `setup --resume` to restart OAuth flow
- **Fix:** Planned for v0.2.0 (increase timeout to 10 min)

**Issue:** No migration guide for users with existing MCP setup
- **Workaround:** Run setup with fresh install, then manually merge configs
- **Fix:** Planned for GA (v1.0.0) - migration guide

### P2 Issues (Minor)

**Issue:** No GCP Console screenshots in guide
- **Workaround:** Follow text instructions, ask #vida-dev if stuck
- **Fix:** Planned for future release

**Issue:** Verbose output is very verbose (hard to read)
- **Workaround:** Scroll to bottom for summary
- **Fix:** Planned for v0.2.0 (better formatting)

---

## Feedback

**Found an issue not listed here?**
- Report in #vida-dev
- Or file GitHub issue: https://github.com/[REDACTED_EMPLOYER]-src/vida/issues

**Suggestions for this guide?**
- Post in #vida-dev
- Or DM DevEx team

---

**Document Version:** 1.0 (Beta)
**Last Updated:** 2025-12-07
**Maintained By:** DevEx Team
