# SSH Port Forwarding for Atlassian MCP

## The Problem

When using Claude Code over SSH, the Atlassian MCP OAuth flow breaks because:
1. Browser opens on your **local machine** (Mac/laptop)
2. OAuth callback server runs on the **remote server** (SSH host)
3. Browser tries to connect to `localhost:5598` on your Mac ❌
4. But the callback server is on the remote ❌

## The Solution: SSH Port Forwarding

Forward port 5598 from the remote server back to your local machine.

---

## For Non-CLI Users: GUI Method

If you're uncomfortable with terminal commands, follow these visual steps:

### Step 1: Open SSH Config in Text Editor

**On Mac:**
1. Open **Finder**
2. Press `Cmd + Shift + G` (Go to Folder)
3. Type: `~/.ssh` and press Enter
4. Double-click `config` file
5. If you get "No application", right-click → Open With → **TextEdit**

**On Windows (WSL/Git Bash):**
1. Open **File Explorer**
2. In address bar, type: `%USERPROFILE%\.ssh` and press Enter
3. Right-click `config` → Open with → **Notepad**

### Step 2: Add These Lines

Scroll to the bottom of the file and add:

```
Host vbonnet-w
    RemoteForward 5598 localhost:5598
```

**Replace `vbonnet-w` with YOUR remote hostname** (the name you use in `ssh your-hostname`)

Or, to apply to all work machines ending in `-w`:

```
Host *-w
    RemoteForward 5598 localhost:5598
```

### Step 3: Save and Close

- **Mac TextEdit**: File → Save (`Cmd + S`)
- **Windows Notepad**: File → Save (`Ctrl + S`)

### Step 4: Reconnect SSH

Close your current SSH session and reconnect. The port forwarding will activate automatically!

---

## For CLI Users: Terminal Method

On your **local machine** (Mac/laptop), edit SSH config:

```bash
# Open in text editor
nano ~/.ssh/config

# Or use your preferred editor
code ~/.ssh/config    # VS Code
vim ~/.ssh/config     # Vim
```

Add:

```
Host vbonnet-w
    RemoteForward 5598 localhost:5598
```

Save (`Ctrl+O`, `Enter`, `Ctrl+X` in nano) and reconnect SSH.

---

## Verification

After reconnecting SSH, verify port forwarding is working:

**On the remote server**, run:

```bash
netstat -an | grep 5598
```

You should see:
```
tcp        0      0 127.0.0.1:5598          0.0.0.0:*               LISTEN
```

This means port 5598 on the remote is now forwarded to your local machine!

---

## Troubleshooting

### "Port already in use"

**On the remote**, kill the old process:

```bash
lsof -i :5598
kill <PID>
```

### "config file not found"

**Create it:**

**Mac/Linux:**
```bash
mkdir -p ~/.ssh
touch ~/.ssh/config
chmod 600 ~/.ssh/config
```

**Windows Git Bash:**
```bash
mkdir -p ~/.ssh
notepad ~/.ssh/config
```

### "Still getting OAuth errors"

1. **Verify you reconnected** SSH (disconnect and reconnect)
2. **Check port forwarding** with `netstat` command above
3. **Try clearing OAuth state**:
   ```bash
   rm -rf ~/.mcp-auth/
   ```
4. **Restart Claude Code** and try Atlassian MCP again

---

## Alternative: Use API Token-Based MCP

If port forwarding doesn't work, use the community alternative:

**Switch to sooperset/mcp-atlassian:**

1. Create API token: https://id.atlassian.com/manage-profile/security/api-tokens
2. Update your Claude Code config:

```json
{
  "Atlassian": {
    "command": "npx",
    "args": ["-y", "@sooperset/mcp-atlassian"],
    "env": {
      "ATLASSIAN_API_TOKEN": "your-token-here",
      "ATLASSIAN_EMAIL": "you@company.com",
      "ATLASSIAN_DOMAIN": "yourcompany.atlassian.net"
    }
  }
}
```

3. Restart Claude Code

**Pros**: Works over SSH, no port forwarding needed
**Cons**: Not the official Atlassian MCP

---

## Why This Works

**Without Port Forwarding:**
```
Mac (Browser) → localhost:5598 ❌ Nothing listening
Remote (SSH) → localhost:5598 ✓ OAuth server running
```

**With Port Forwarding:**
```
Mac (Browser) → localhost:5598 → [SSH tunnel] → Remote localhost:5598 ✓
```

The SSH tunnel forwards your Mac's `localhost:5598` to the remote's `localhost:5598`, so the OAuth callback reaches the right place!

---

## Visual Guide: SSH Config Location

```
Mac/Linux:
~/
└── .ssh/
    └── config  ← Edit this file

Windows:
C:\Users\YourName\
└── .ssh\
    └── config  ← Edit this file
```

---

**Questions?** See full Atlassian MCP docs: `docs/ATLASSIAN-MCP.md`
