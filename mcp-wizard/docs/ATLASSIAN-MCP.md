# Atlassian MCP Setup

## How It Works

The Atlassian MCP uses `mcp-remote` which **handles OAuth automatically** - no manual setup needed!

## Setup Process

### 1. Run Wizard
```bash
[REDACTED_EMPLOYER]-mcp setup
```

Select "Atlassian" when prompted. The wizard will:
- ✅ Add Atlassian MCP to your config
- ✅ Configure mcp-remote connection

**That's it!** No OAuth steps in the wizard.

### 2. First Use (OAuth Happens Here)

When you first use the Atlassian MCP in Claude Code:

1. **Browser opens automatically**: `https://mcp.atlassian.com/v1/authorize?...`
2. **You authorize**:
   - Select your Atlassian site (Jira/Confluence instance)
   - Review permissions (read access to Jira, Confluence)
   - Click "Allow"
3. **Redirect to localhost**: `http://localhost:5598/oauth/callback`
4. **mcp-remote handles it**: Receives OAuth code, exchanges for token, stores it
5. **Connection complete**: Atlassian MCP is now authenticated

**This happens automatically - just follow the browser prompts.**

## Troubleshooting

### "Not Found" Error After OAuth

**Symptom**: Browser redirects to `localhost:5598/oauth/callback` and shows "Not Found"

**Cause**: The OAuth callback server timed out (default 30 seconds). The server shut down before you finished authorizing.

**Solutions**:

1. **Clear incomplete OAuth state**:
   ```bash
   rm -rf ~/.mcp-auth/
   ```

2. **Update your Claude Code config** with increased timeout:
   ```bash
   # Edit ~/.claude.json (or ~/.config/claude-code/mcp.json)
   # Change Atlassian args from:
   #   "args": ["-y", "mcp-remote@latest", "https://mcp.atlassian.com/v1/sse"]
   # To:
   #   "args": ["-y", "mcp-remote@latest", "https://mcp.atlassian.com/v1/sse", "--auth-timeout", "120"]
   ```

3. **Restart Claude Code** and authorize quickly (you now have 2 minutes instead of 30 seconds)

4. **Check firewall** (if still failing): Ensure `localhost:5598` is not blocked
   ```bash
   curl http://localhost:5598
   ```

### SSH Environment - OAuth Callback Fails

**Symptom**: Using Claude Code over SSH, browser opens but OAuth callback fails with "invalid_token" or "connection refused"

**Cause**: Browser opens on your local machine (Mac/laptop), but OAuth callback server runs on the remote SSH host. The callback to `localhost:5598` goes to the wrong machine.

**Solution**: Use SSH port forwarding to tunnel the OAuth callback from local to remote.

**Quick Fix** - On your LOCAL machine (Mac), edit `~/.ssh/config`:

```
Host vbonnet-w
    RemoteForward 5598 localhost:5598
```

Then reconnect SSH.

**Detailed Guide**: See [SSH Port Forwarding Guide](SSH-PORT-FORWARDING-GUIDE.md) for:
- Step-by-step GUI instructions (no terminal commands needed!)
- Screenshots and visual guides
- Troubleshooting steps
- Alternative API token-based solution

**Alternative**: Use the community MCP [@sooperset/mcp-atlassian](https://github.com/sooperset/mcp-atlassian) which supports API tokens (no OAuth, no port forwarding needed).

---

### Browser Doesn't Open

**Symptom**: No browser opens when using Atlassian MCP

**Cause**: mcp-remote can't launch browser, or OAuth already completed

**Solutions**:

1. **Manually open URL**: Check Claude Code logs for the authorize URL
2. **Check if already authenticated**:
   ```bash
   # Check if mcp-remote stored credentials
   ls ~/.mcp-remote/  # (or wherever mcp-remote stores tokens)
   ```

### "OAuth timeout" or "Auth failed"

**Symptom**: OAuth process times out or fails

**Solutions**:

1. **Check network**: Can you reach Atlassian?
   ```bash
   curl https://mcp.atlassian.com/v1/authorize
   ```

2. **Restart Claude Code**: Retry OAuth flow
3. **Ask #vida-dev**: May be Atlassian API issue

## Configuration

The wizard adds this to your Claude Code config:

```json
{
  "mcpServers": {
    "Atlassian": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote@latest",
        "https://mcp.atlassian.com/v1/sse",
        "--auth-timeout",
        "120"
      ]
    }
  }
}
```

**Config notes**:
- `--auth-timeout 120`: Gives you 2 minutes to complete OAuth (default is 30 seconds)
- **No env vars needed** - mcp-remote stores OAuth credentials internally in `~/.mcp-auth/`

## How mcp-remote OAuth Works

1. **First connection**: Claude Code starts `npx mcp-remote@latest https://mcp.atlassian.com/v1/sse`
2. **mcp-remote checks**: Do I have cached OAuth credentials?
3. **If no credentials**: Start OAuth flow
   - Generate PKCE challenge
   - Start local HTTP server on port 5598
   - Open browser to authorize URL with callback to localhost:5598
4. **User authorizes in browser**
5. **Atlassian redirects**: `localhost:5598/oauth/callback?code=...`
6. **mcp-remote receives callback**: Exchange code for access token
7. **Credentials cached**: Stored for future connections
8. **MCP connects**: Using OAuth access token

## OAuth Token Storage

**Location**: mcp-remote stores tokens in its own cache (implementation detail)

**Expiration**: Tokens refresh automatically via OAuth refresh tokens

**Revocation**: To revoke access:
1. Go to your Atlassian account settings
2. Find "Connected apps" or "OAuth authorizations"
3. Revoke access to "MCP Atlassian"
4. mcp-remote will re-prompt for OAuth on next use

## Security

- ✅ **PKCE flow**: mcp-remote uses PKCE (Proof Key for Code Exchange) for secure OAuth
- ✅ **Local callback**: OAuth callback to `localhost:5598` (not exposed to internet)
- ✅ **Scopes**: Read-only access to Jira and Confluence
- ✅ **No credentials in config**: OAuth tokens not in `~/.claude.json` (stored by mcp-remote)

## FAQ

**Q: Do I need to create an Atlassian OAuth app?**
A: ❌ NO - mcp-remote uses a pre-configured OAuth client

**Q: Can I skip OAuth and use API token instead?**
A: ❌ NO - mcp-remote requires OAuth (doesn't support API tokens)

**Q: How often do I need to re-authenticate?**
A: Rarely - mcp-remote uses refresh tokens to maintain access

**Q: Can I use multiple Atlassian sites?**
A: You'll be prompted to select your site during OAuth - only one site per setup

**Q: What permissions does it request?**
A: Read-only access to:
  - Jira issues
  - Confluence pages
  - User profile

---

**See Also**:
- Main TROUBLESHOOTING guide: `~/libraries/[REDACTED_EMPLOYER]-mcp/TROUBLESHOOTING.md`
- mcp-remote package: https://github.com/modelcontextprotocol/servers
