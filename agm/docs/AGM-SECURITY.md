# Security Policy

## Supported Versions

We release patches for security vulnerabilities in the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 3.0.x   | :white_check_mark: |
| 2.5.x   | :white_check_mark: |
| 2.0.x   | :x:                |
| < 2.0   | :x:                |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

### Preferred Reporting Method

Send an email to: **security@ai-tools.dev** (replace with actual security contact)

Include:
- Description of the vulnerability
- Steps to reproduce the issue
- Potential impact
- Suggested fix (if any)

### Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial assessment**: Within 7 days
- **Fix timeline**: Depends on severity
  - Critical: 7-14 days
  - High: 14-30 days
  - Medium: 30-60 days
  - Low: Next release cycle

### Disclosure Policy

- We will confirm receipt of your report
- We will investigate and validate the issue
- We will develop and test a fix
- We will release the fix and publicly disclose the vulnerability

**Coordinated disclosure**: We ask that you do not publicly disclose the vulnerability until we have released a fix.

## Security Best Practices

### API Key Management

**DO:**
- ✅ Store API keys in environment variables
- ✅ Use `.bashrc` or `.zshrc` for persistent keys
- ✅ Set restrictive file permissions (`chmod 600 ~/.bashrc`)
- ✅ Use different keys for development and production
- ✅ Rotate keys regularly

**DON'T:**
- ❌ Commit API keys to version control
- ❌ Share API keys via email or Slack
- ❌ Store keys in plain text files
- ❌ Use the same key across multiple systems
- ❌ Expose keys in logs or error messages

**Example:**
```bash
# Good: Environment variables in ~/.bashrc
export ANTHROPIC_API_KEY="sk-ant-..."
export GEMINI_API_KEY="AIza..."
chmod 600 ~/.bashrc

# Bad: Keys in scripts
./script.sh --api-key sk-ant-...  # DON'T DO THIS
```

### Session Data Security

**Session storage location:**
- `~/sessions/` - Current unified storage
- `~/.claude-sessions/` - Legacy storage
- `~/.agm/logs/messages/` - Message logs

**File permissions:**
```bash
# Secure your session directories
chmod 700 ~/sessions
chmod 700 ~/.agm/logs

# Verify permissions
ls -la ~/sessions
# Should show: drwx------ (700)
```

**What's stored in sessions:**
- Conversation history (potentially sensitive)
- Project directory paths
- Agent UUIDs
- Metadata (creation time, last updated, agent type)

**Data retention:**
- Active sessions: Indefinite
- Archived sessions: User-controlled
- Message logs: Configurable rotation (default: 30 days)

### Network Security

**AGM communicates with:**
- **Anthropic API** (api.anthropic.com) - Claude agent
- **Google AI API** (generativelanguage.googleapis.com) - Gemini agent
- **OpenAI API** (api.openai.com) - GPT agent
- **Google Cloud Vertex AI** (optional) - Semantic search

**All communications use:**
- HTTPS/TLS 1.2+
- API key authentication
- No data stored on remote servers (local-first architecture)

**Firewall recommendations:**
```bash
# Allow outbound HTTPS to AI providers
# Claude
api.anthropic.com:443

# Gemini
generativelanguage.googleapis.com:443
aiplatform.googleapis.com:443

# OpenAI
api.openai.com:443
```

### Tmux Security

**Tmux socket permissions:**
```bash
# Verify tmux socket is user-only
ls -la /tmp/tmux-$(id -u)/
# Should show: srwxrwx--- (770 or stricter)

# If permissions are too open:
chmod 700 /tmp/tmux-$(id -u)/
```

**Session isolation:**
- Each AGM session runs in a separate tmux session
- Sessions are isolated from each other
- No cross-session data leakage
- User-level isolation (cannot access other users' sessions)

### Configuration File Security

**Configuration location:**
- `~/.config/agm/config.yaml`

**Sensitive data in config:**
- ❌ Never store API keys in config.yaml
- ✅ Use environment variables for secrets
- ✅ Set restrictive permissions: `chmod 600 ~/.config/agm/config.yaml`

**Example secure config:**
```yaml
# Good: No secrets in config
defaults:
  interactive: true
  auto_associate_uuid: true

ui:
  theme: "agm"

# Bad: DO NOT store keys here
# NEVER DO THIS:
# api_keys:
#   anthropic: "sk-ant-..."  # WRONG!
```

### Audit Logging

**Message logging:**
- All agent interactions logged to `~/.agm/logs/messages/`
- Logs contain timestamps, sender, recipient, message content
- Default retention: 30 days
- Rotation: Size-based (10MB per file)

**Audit queries:**
```bash
# Review recent interactions
agm logs query --since 2026-02-01

# Find specific sessions
agm logs query --session my-session

# Detect suspicious activity
agm logs query --sender astrocyte --level error
```

**Log security:**
```bash
# Secure log directory
chmod 700 ~/.agm/logs
chmod 600 ~/.agm/logs/messages/*.jsonl

# Verify permissions
ls -la ~/.agm/logs/messages/
```

### Multi-User Environments

**Shared systems:**
- Each user has isolated session storage
- Sessions stored in user home directory
- Tmux sockets user-specific
- No shared state between users

**Recommendations for shared systems:**
1. Use user lingering for persistent sessions:
   ```bash
   loginctl enable-linger $USER
   ```

2. Set umask for restrictive default permissions:
   ```bash
   umask 077  # New files: 600, new dirs: 700
   ```

3. Regular security audits:
   ```bash
   agm doctor --validate
   ```

## Known Security Considerations

### 1. Local Storage

**Issue:** Conversation history stored locally in plain text
**Mitigation:**
- Use full disk encryption (LUKS, FileVault, BitLocker)
- Set restrictive file permissions
- Regular backups to encrypted storage

### 2. API Keys in Environment

**Issue:** API keys visible in process environment
**Mitigation:**
- Keys only visible to user's own processes
- Use `chmod 600` on shell config files
- Avoid echoing keys in scripts

### 3. Tmux Session Hijacking

**Issue:** Local users could potentially attach to tmux sessions
**Mitigation:**
- Tmux sockets have user-only permissions
- Session locking prevents concurrent access
- Use `agm unlock` if session stuck

### 4. Message Logs

**Issue:** Logs contain conversation content
**Mitigation:**
- Logs stored in user directory with 700 permissions
- Automatic rotation and cleanup
- Configure retention: `agm logs clean --older-than 30`

### 5. Semantic Search (Google Vertex AI)

**Issue:** Search queries sent to Google Cloud
**Mitigation:**
- Search is opt-in (requires explicit `agm search` command)
- Queries cached for 5 minutes (local cache)
- Rate limited: 10 searches/minute
- Use `gcloud auth application-default login` for authentication

## Security Updates

We will announce security updates through:
1. GitHub Security Advisories
2. Release notes in CHANGELOG.md
3. Tagged releases with CVE references (if applicable)

**Subscribe to updates:**
- Watch the repository on GitHub
- Star the repository for release notifications
- Follow the project for important announcements

## Vulnerability Disclosure History

### CVE-XXXX-XXXX (Example - None currently)
**Severity:** Critical/High/Medium/Low
**Affected versions:** X.X.X
**Fixed in:** X.X.X
**Description:** Brief description
**Workaround:** Temporary mitigation steps
**Fix:** Update to version X.X.X

---

**Last Updated:** 2026-02-04
**Contact:** security@ai-tools.dev (replace with actual contact)
**PGP Key:** [Link to PGP key if applicable]
