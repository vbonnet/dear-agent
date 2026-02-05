# Atlassian MCP Setup Guide

Complete guide for setting up the Atlassian MCP (Model Context Protocol) with OAuth 2.0 authentication.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Setup Instructions](#setup-instructions)
- [Remote Machine Setup](#remote-machine-setup)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)
- [How It Works](#how-it-works)

## Overview

The Atlassian MCP provides Claude with access to:
- Jira issues, projects, boards, and workflows
- Confluence pages, spaces, and content

Authentication uses OAuth 2.0 with PKCE (Proof Key for Code Exchange) for enhanced security.

## Architecture

```
┌─────────────┐      STDIO       ┌──────────────┐      SSE/HTTP      ┌─────────────────┐
│             │ ◄──────────────► │              │ ◄────────────────► │                 │
│ Claude Code │                  │  mcp-remote  │                    │ mcp.atlassian.  │
│             │                  │    (proxy)   │                    │      com        │
└─────────────┘                  └──────────────┘                    └─────────────────┘
                                        │
                                        ▼
                                 OAuth Callback
                               localhost:45454
```

**Components:**
- **Claude Code**: Main AI assistant
- **mcp-remote**: NPM package that proxies between STDIO (Claude) and SSE (Atlassian)
- **mcp.atlassian.com**: Remote MCP server hosted by Atlassian
- **OAuth Callback Server**: Temporary HTTP server on port 45454 for OAuth flow

## Prerequisites

- Node.js and npm installed
- Claude Code configured (`~/.config/claude/mcp.json`)
- Access to an Atlassian account (Jira/Confluence)
- For remote machines: SSH access with port forwarding capability

## Setup Instructions

### 1. Run mcp-wizard

```bash
npx mcp-wizard@latest
```

Select "Atlassian" from the list of MCPs.

### 2. Complete OAuth Flow

The setup will:
1. Start an OAuth callback server on `localhost:45454`
2. Generate an OAuth authorization URL
3. Open your browser (or display URL to copy)
4. Wait for you to complete authentication

**In your browser:**
1. Sign in with your Atlassian account
2. Select your Atlassian site (e.g., `yourcompany.atlassian.net`)
3. Review requested permissions:
   - Read Jira issues, projects, boards
   - Read Confluence pages, spaces
4. Click "Accept" to authorize
5. Wait for "Authorization successful!" message

### 3. Verify Setup

The wizard will automatically test the connection. You should see:
```
✓ Connected to remote server using SSEClientTransport
✓ Local STDIO server running
✓ Proxy established successfully
✓ Atlassian MCP is ready to use
```

## Remote Machine Setup

When running Claude Code on a remote machine (e.g., cloud workstation), you need port forwarding to complete OAuth.

### Why Port Forwarding?

The OAuth callback redirects to `http://localhost:45454/oauth/callback`. This must reach the remote machine where `mcp-remote` is listening.

### Setup Steps

**Terminal 1: Start port forwarding**
```bash
# For gcloud workstations:
gcloud workstations ssh your-workstation \
  --cluster=your-cluster \
  --config=your-config \
  --region=us-central1 \
  --project=your-project \
  -- -L 45454:localhost:45454 "sleep 300"

# For generic SSH:
ssh -L 45454:localhost:45454 user@remote-machine "sleep 300"
```

**Terminal 2: Run setup on remote machine**
```bash
# SSH into remote machine
ssh user@remote-machine

# Run mcp-wizard
npx mcp-wizard@latest
```

**Terminal 3: Complete OAuth on local machine**

When the OAuth URL appears, open it in your local browser. The callback will tunnel through the SSH port forward to the remote machine.

### Alternative: Use screen/tmux

If you prefer a single terminal:
```bash
# Start port forwarding in background
ssh -f -N -L 45454:localhost:45454 user@remote-machine

# SSH into remote machine and run setup
ssh user@remote-machine
npx mcp-wizard@latest

# After OAuth completes, kill the port forward
pkill -f "ssh.*-L.*45454"
```

## Testing

### Manual Connection Test

```bash
# Should connect and show proxy status
timeout 10 npx -y mcp-remote@latest https://mcp.atlassian.com/v1/sse
```

Expected output:
```
[PID] Connected to remote server using SSEClientTransport
[PID] Local STDIO server running
[PID] Proxy established successfully
```

### Test in Claude Code

Start Claude and try:
```
"Show me recent Jira issues"
"Search Confluence for API documentation"
"What Jira issues are assigned to me?"
```

## Troubleshooting

### Port Already in Use

**Error:** `Error: listen EADDRINUSE: address already in use 127.0.0.1:45454`

**Solution:**
```bash
# Find process using port
lsof -i :45454

# Kill the process
kill <PID>

# Or kill all mcp-remote processes
pkill -f mcp-remote

# Wait a moment and retry
sleep 2
npx mcp-wizard@latest
```

### OAuth Timeout

**Error:** Authentication times out before completion

**Solution:**
1. Increase timeout in `~/.config/claude/mcp.json`:
   ```json
   {
     "mcpServers": {
       "Atlassian": {
         "args": ["-y", "mcp-remote@latest", "https://mcp.atlassian.com/v1/sse", "--auth-timeout", "300"]
       }
     }
   }
   ```

2. Restart setup:
   ```bash
   pkill -f mcp-remote
   npx mcp-wizard@latest
   ```

### Connection Test Fails

**Error:** Connection test shows timeout or ECONNREFUSED

**Possible causes:**
1. OAuth not completed successfully
2. Network connectivity issues
3. Firewall blocking SSE connection

**Solutions:**
```bash
# 1. Delete cached auth and retry
rm -rf ~/.mcp-remote/
npx mcp-wizard@latest

# 2. Check OAuth tokens exist
ls -la ~/.mcp-remote/

# 3. Test network connectivity
curl -v https://mcp.atlassian.com/v1/sse
```

### Browser Doesn't Open

**Issue:** `open` command fails or browser doesn't launch

**Solution:**
1. Manually copy the OAuth URL from terminal
2. Paste into your browser
3. Complete authentication
4. Return to terminal and confirm completion

### Remote Machine: Port Forward Not Working

**Error:** OAuth callback times out on remote machine

**Checklist:**
- [ ] Port forward started BEFORE running mcp-wizard
- [ ] Using correct port (45454)
- [ ] SSH connection active during OAuth
- [ ] No firewall blocking local port 45454
- [ ] Browser accessing http://localhost:45454 (not the remote IP)

**Debug:**
```bash
# On local machine, verify port forward is listening
lsof -i :45454

# Should show ssh process with LISTEN state
```

### Re-authentication Required

If OAuth tokens expire or you need to switch Atlassian sites:

```bash
# Delete cached tokens
rm -rf ~/.mcp-remote/

# Restart Claude Code
# Next time it loads Atlassian MCP, OAuth will re-run
```

## How It Works

### Configuration

The Atlassian MCP is configured in `~/.config/claude/mcp.json`:

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
      ],
      "defer": true
    }
  }
}
```

**Key settings:**
- `defer: true` - Loads MCP only when first used (not at Claude startup)
- `--auth-timeout 120` - OAuth callback timeout in seconds (default 120s)

### OAuth Flow

1. **Initial Request**: Claude Code starts `mcp-remote`
2. **Discovery**: mcp-remote fetches OAuth server config from Atlassian
3. **PKCE Generation**: Creates code_challenge for security
4. **Callback Server**: Starts HTTP server on `localhost:45454`
5. **Browser Auth**: User authenticates in browser
6. **Callback**: Browser redirects to `http://localhost:45454/oauth/callback?code=...`
7. **Token Exchange**: mcp-remote exchanges authorization code for access token
8. **Token Storage**: Tokens saved to `~/.mcp-remote/<server-hash>/`
9. **Connection**: Establishes SSE connection to Atlassian MCP server

### Transport Strategy

mcp-remote uses "http-first" transport strategy:
1. Try HTTP connection first
2. Fall back to SSE-only if HTTP fails
3. Maintains persistent connection for real-time updates

### Token Management

- **Access tokens**: Short-lived (usually 1 hour)
- **Refresh tokens**: Long-lived (weeks/months)
- **Auto-refresh**: mcp-remote automatically refreshes expired access tokens
- **Storage**: Tokens in `~/.mcp-remote/<hash>/` (one directory per MCP server)

### First Use vs. Subsequent Use

**First use (no tokens):**
1. mcp-remote starts OAuth flow
2. User completes browser authentication
3. Tokens stored locally
4. Connection established

**Subsequent uses (tokens exist):**
1. mcp-remote loads tokens from disk
2. Validates token expiration
3. Refreshes if needed
4. Connects immediately (no browser required)

## Configuration Reference

### Minimal Configuration

```json
{
  "mcpServers": {
    "Atlassian": {
      "command": "npx",
      "args": ["-y", "mcp-remote@latest", "https://mcp.atlassian.com/v1/sse"]
    }
  }
}
```

### Recommended Configuration

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
      ],
      "defer": true,
      "env": {
        "NODE_ENV": "production"
      }
    }
  }
}
```

### Advanced Configuration

For remote machines or complex setups:

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
        "300",
        "--transport-strategy",
        "http-first"
      ],
      "defer": true,
      "env": {
        "DEBUG": "mcp-remote:*"
      }
    }
  }
}
```

## Security Considerations

### OAuth 2.0 with PKCE

- **PKCE**: Proof Key for Code Exchange prevents authorization code interception
- **No client secret**: Public client (Claude Code) doesn't store secrets
- **State parameter**: Prevents CSRF attacks
- **Redirect to localhost**: Callback only to local machine

### Token Storage

- **Location**: `~/.mcp-remote/<server-hash>/`
- **Permissions**: Only readable by your user account
- **Encryption**: Tokens stored as plain JSON (secure your home directory!)

### Recommended Practices

1. **Use deferred loading**: Set `defer: true` to avoid unnecessary MCP startup
2. **Limit timeouts**: Don't set excessively long auth timeouts
3. **Regular cleanup**: Periodically remove old tokens: `rm -rf ~/.mcp-remote/`
4. **Secure SSH**: For remote machines, use SSH keys (not passwords)
5. **Review permissions**: Regularly audit what access you've granted in Atlassian

## Additional Resources

- [mcp-remote package](https://www.npmjs.com/package/mcp-remote)
- [Atlassian MCP Documentation](https://mcp.atlassian.com/docs)
- [OAuth 2.0 PKCE Specification](https://datatracker.ietf.org/doc/html/rfc7636)
- [Model Context Protocol Specification](https://modelcontextprotocol.io)

## Feedback and Issues

If you encounter issues not covered in this guide:

1. Check existing issues: https://github.com/your-org/mcp-wizard/issues
2. Create a new issue with:
   - Your setup (local/remote, OS, Node version)
   - Error messages
   - Steps to reproduce
   - What you've already tried

---

**Last updated**: Based on production testing with [REDACTED_EMPLOYER].atlassian.net (February 2026)
