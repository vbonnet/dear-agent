# Codex (OpenAI API) Agent Guide

Complete guide for using Codex agent with AGM (AI-tools Generative Multimodal).

---

## Overview

Codex is an AGM agent that provides access to OpenAI models (GPT-4, GPT-3.5-Turbo, etc.) via the OpenAI API. Unlike CLI-based agents (Claude, Gemini), Codex is **API-based**, meaning it doesn't require tmux sessions or terminal wrappers.

### Key Features

- **API-Based**: Direct OpenAI API integration (no CLI required)
- **Multiple Models**: Supports GPT-4, GPT-3.5-Turbo, and other OpenAI models
- **Azure OpenAI**: Compatible with Azure OpenAI Service
- **Synthetic Hooks**: File-based lifecycle notifications
- **Full Parity**: Same session management as Claude/Gemini

### When to Use Codex

- ✅ Need GPT-4 or GPT-3.5-Turbo specifically
- ✅ Want API-based interaction (no terminal sessions)
- ✅ Using Azure OpenAI Service
- ✅ Prefer simpler session management (no tmux)

---

## Setup

### 1. Get OpenAI API Key

**Option A: Standard OpenAI**

1. Visit [https://platform.openai.com/api-keys](https://platform.openai.com/api-keys)
2. Sign in or create account
3. Click "Create new secret key"
4. Copy the key (starts with `sk-...`)
5. **Important**: Save the key securely (you can't view it again)

**Option B: Azure OpenAI**

1. Access Azure Portal: [https://portal.azure.com](https://portal.azure.com)
2. Navigate to your Azure OpenAI resource
3. Go to "Keys and Endpoint"
4. Copy API key and endpoint URL

### 2. Configure Environment

Add API key to your environment:

**Bash/Zsh** (`~/.bashrc` or `~/.zshrc`):
```bash
export OPENAI_API_KEY='sk-your-key-here'
```

**Fish** (`~/.config/fish/config.fish`):
```fish
set -gx OPENAI_API_KEY 'sk-your-key-here'
```

**Temporary (current session only)**:
```bash
export OPENAI_API_KEY='sk-your-key-here'
```

**Verify setup**:
```bash
echo $OPENAI_API_KEY
# Should output: sk-...
```

### 3. Create Codex Session

**Interactive (with dropdown)**:
```bash
agm session new my-session
# Select "Codex (OpenAI API)" from dropdown
```

**Command-line (direct)**:
```bash
agm session new --harness=codex-cli my-session
```

**With working directory**:
```bash
agm session new --harness=codex-cli --work-dir ~/projects/myapp coding-session
```

### 4. Verify Session

List active sessions:
```bash
agm session list
```

Attach to session:
```bash
agm session attach my-session
```

---

## Model Selection

### Default Model

Codex uses **gpt-4-turbo-preview** by default (configurable via backend adapter).

### Available Models

| Model | Context | Speed | Cost | Best For |
|-------|---------|-------|------|----------|
| gpt-4-turbo-preview | 128K tokens | Medium | $$$ | Complex reasoning, latest features |
| gpt-4 | 8K tokens | Slow | $$$$ | Highest quality, complex tasks |
| gpt-3.5-turbo | 16K tokens | Fast | $ | Quick tasks, simple queries |
| gpt-3.5-turbo-16k | 16K tokens | Fast | $ | Longer conversations |

### Model Configuration

Models are configured in the OpenAI adapter backend. To change the default model, see `internal/agent/openai_adapter.go` configuration.

---

## Azure OpenAI Support

### Configuration

Azure OpenAI requires additional configuration:

1. **Set Azure-specific environment variables**:
```bash
export OPENAI_API_KEY='your-azure-api-key'
export AZURE_OPENAI_ENDPOINT='https://your-resource.openai.azure.com'
export AZURE_OPENAI_DEPLOYMENT='your-deployment-name'
```

2. **Backend configuration** (see `internal/agent/openai_adapter.go`):
```go
// Azure OpenAI configuration
IsAzure: true,
BaseURL: os.Getenv("AZURE_OPENAI_ENDPOINT"),
DeploymentName: os.Getenv("AZURE_OPENAI_DEPLOYMENT"),
```

### Azure vs Standard OpenAI

| Feature | Standard OpenAI | Azure OpenAI |
|---------|-----------------|--------------|
| Endpoint | api.openai.com | your-resource.openai.azure.com |
| Auth | API key | API key + deployment |
| Models | All OpenAI models | Deployed models only |
| Pricing | Pay-as-you-go | Azure pricing |
| Compliance | OpenAI terms | Azure enterprise terms |

---

## Usage

### Basic Operations

**Create session**:
```bash
agm session new --harness=codex-cli my-session
```

**Send message** (within session):
```
> Hello, Codex!
```

**View history**:
```bash
agm session export my-session --format=jsonl
```

**Pause session**:
```bash
agm session pause my-session
```

**Resume session**:
```bash
agm session resume my-session
```

**Archive session**:
```bash
agm session archive my-session
```

### Export Formats

Codex supports the same export formats as Claude/Gemini:

**JSONL** (machine-readable):
```bash
agm session export my-session --format=jsonl > conversation.jsonl
```

**Markdown** (human-readable):
```bash
agm session export my-session --format=markdown > conversation.md
```

### Import Sessions

Import from JSONL:
```bash
agm session import --harness=codex-cli --file=conversation.jsonl imported-session
```

---

## Hooks

Codex uses **synthetic hooks** (file-based) instead of shell script hooks.

### Hook System

**Hook Directory**: `~/.agm/openai-hooks/`

**Hook Files**:
- `SessionStart-{timestamp}.json` - Created when session starts
- `SessionEnd-{timestamp}.json` - Created when session terminates

**Example SessionStart Hook**:
```json
{
  "event": "SessionStart",
  "timestamp": "2026-03-09T10:30:00Z",
  "session_id": "abc-123-def",
  "agent": "codex",
  "metadata": {
    "session_name": "my-session",
    "api_key_present": true,
    "working_directory": "~/projects/myapp"
  }
}
```

### Hook Watchers

Create external watchers to process hooks:

**Example Bash Watcher**:
```bash
#!/bin/bash
# Watch for Codex hooks and send notifications

inotifywait -m ~/.agm/openai-hooks/ -e create |
while read dir action file; do
  if [[ "$file" == SessionStart-*.json ]]; then
    session_id=$(jq -r '.session_id' "$dir$file")
    notify-send "Codex Session Started" "Session ID: $session_id"
  elif [[ "$file" == SessionEnd-*.json ]]; then
    session_id=$(jq -r '.session_id' "$dir$file")
    notify-send "Codex Session Ended" "Session ID: $session_id"
  fi
done
```

**Why Synthetic Hooks?**
- No security concerns (no code execution)
- Graceful failure (don't block sessions)
- Machine-readable (JSON format)
- Platform-independent

---

## Troubleshooting

### API Key Issues

**Problem**: "OPENAI_API_KEY not set" error

**Solution**:
1. Check if key is set:
   ```bash
   echo $OPENAI_API_KEY
   ```

2. If empty, set it:
   ```bash
   export OPENAI_API_KEY='sk-your-key-here'
   ```

3. For persistence, add to shell profile:
   ```bash
   echo "export OPENAI_API_KEY='sk-your-key-here'" >> ~/.bashrc
   source ~/.bashrc
   ```

**Verification**:
```bash
# Test with curl
curl https://api.openai.com/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY" | jq '.data[0].id'
```

---

### Rate Limiting

**Problem**: "Rate limit exceeded" error

**Causes**:
- Free tier rate limits (3 requests/minute)
- Tier limits exceeded (see [OpenAI rate limits](https://platform.openai.com/account/rate-limits))
- Burst request patterns

**Solutions**:

1. **Wait and retry**:
   ```bash
   # Wait 60 seconds
   sleep 60
   agm session resume my-session
   ```

2. **Reduce request frequency**:
   - Batch messages instead of rapid-fire
   - Add delays between requests

3. **Upgrade tier**:
   - Visit [OpenAI usage limits](https://platform.openai.com/account/limits)
   - Add payment method to increase tier
   - Tier 1: $5 spent → 3,500 RPM

4. **Use slower model**:
   - Switch from gpt-4 to gpt-3.5-turbo (higher limits)

**Check current limits**:
```bash
curl https://api.openai.com/v1/rate_limits \
  -H "Authorization: Bearer $OPENAI_API_KEY" | jq
```

---

### Azure OpenAI Issues

**Problem**: Azure endpoint not working

**Checklist**:
1. ✅ AZURE_OPENAI_ENDPOINT set correctly
2. ✅ Endpoint format: `https://{resource-name}.openai.azure.com`
3. ✅ AZURE_OPENAI_DEPLOYMENT matches deployed model
4. ✅ API key is Azure key (not standard OpenAI key)

**Test Azure endpoint**:
```bash
curl https://your-resource.openai.azure.com/openai/deployments/your-deployment/chat/completions?api-version=2023-05-15 \
  -H "api-key: $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"Hello"}]}'
```

---

### Model Availability

**Problem**: "Model not available" or "Model not found"

**Common Causes**:
1. **Model deprecated**: OpenAI retired the model
2. **Access restricted**: Account doesn't have access
3. **Azure-specific**: Model not deployed in Azure resource

**Solutions**:

1. **Check OpenAI status**:
   - Visit [https://status.openai.com](https://status.openai.com)
   - Check for model deprecations/outages

2. **List available models**:
   ```bash
   curl https://api.openai.com/v1/models \
     -H "Authorization: Bearer $OPENAI_API_KEY" | jq '.data[].id'
   ```

3. **Try alternative model**:
   - `gpt-4` → `gpt-4-turbo-preview`
   - `gpt-3.5-turbo` → `gpt-3.5-turbo-16k`

4. **Azure**: Check deployed models in Azure Portal

---

### Session Not Found

**Problem**: "Session {id} not found" error

**Solutions**:

1. **List all sessions**:
   ```bash
   agm session list --all
   ```

2. **Check session name**:
   - Session names are case-sensitive
   - Use exact name or session ID

3. **Check sessions directory**:
   ```bash
   ls ~/.agm/codex-sessions/
   ```

4. **Search by pattern**:
   ```bash
   agm session search "my-session"
   ```

---

### Permission Issues

**Problem**: Hook files can't be created

**Error**: `Failed to write hook file: permission denied`

**Solutions**:

1. **Check directory permissions**:
   ```bash
   ls -ld ~/.agm/openai-hooks/
   ```

2. **Fix permissions**:
   ```bash
   mkdir -p ~/.agm/openai-hooks
   chmod 755 ~/.agm/openai-hooks
   ```

3. **Check disk space**:
   ```bash
   df -h ~
   ```

**Note**: Hook failures are graceful - session still works even if hooks fail.

---

## Comparison: Codex vs Claude

Understanding when to use each agent:

| Feature | Codex (OpenAI API) | Claude (Anthropic CLI) |
|---------|-------------------|------------------------|
| **Backend** | OpenAI API | Anthropic CLI |
| **Session Type** | API-based | tmux terminal session |
| **Setup** | API key | CLI installation + config |
| **Models** | GPT-4, GPT-3.5 | Claude 3 (Opus, Sonnet, Haiku) |
| **Hooks** | Synthetic (files) | Shell scripts |
| **Context** | 128K tokens (GPT-4) | 200K tokens (Claude 3) |
| **Pricing** | OpenAI pricing | Anthropic pricing |
| **Best For** | API integration, GPT-4 access | Terminal workflows, large context |

### When to Use Codex
- ✅ Need GPT-4 specifically
- ✅ Want API-based sessions (no tmux)
- ✅ Already using OpenAI ecosystem
- ✅ Need Azure OpenAI integration

### When to Use Claude
- ✅ Need very large context (200K tokens)
- ✅ Prefer terminal-based workflows
- ✅ Want native tmux integration
- ✅ Need Anthropic-specific features

---

## Advanced Configuration

### Custom API Endpoint

For OpenAI-compatible APIs (LocalAI, OpenRouter, etc.):

```bash
# Set custom endpoint
export OPENAI_API_BASE='https://custom-endpoint.com/v1'
export OPENAI_API_KEY='your-custom-key'
```

**Note**: Requires backend configuration in `openai_adapter.go`

### Timeout Configuration

Default timeouts can be adjusted in backend:

```go
// In openai_adapter.go
httpClient := &http.Client{
    Timeout: 60 * time.Second, // Adjust as needed
}
```

### Logging

Enable debug logging:

```bash
AGM_DEBUG=1 agm session new --harness=codex-cli my-session
```

View logs:
```bash
tail -f ~/.agm/logs/codex-$(date +%Y-%m-%d).log
```

---

## Best Practices

### 1. API Key Security

✅ **DO**:
- Store key in environment variable (not code)
- Use secrets manager for production
- Rotate keys periodically
- Limit key permissions (if available)

❌ **DON'T**:
- Commit keys to git
- Share keys via email/chat
- Use same key for dev and prod
- Store keys in plain text files

### 2. Cost Management

✅ **DO**:
- Monitor usage: [https://platform.openai.com/account/usage](https://platform.openai.com/account/usage)
- Set spending limits in OpenAI dashboard
- Use gpt-3.5-turbo for simple tasks
- Archive inactive sessions

❌ **DON'T**:
- Leave sessions running indefinitely
- Use gpt-4 for trivial queries
- Ignore rate limits (wastes money on retries)

### 3. Session Management

✅ **DO**:
- Use descriptive session names (`project-feature-task`)
- Archive completed sessions
- Export important conversations
- Clean up old sessions monthly

❌ **DON'T**:
- Create duplicate sessions for same task
- Keep sensitive data in session history
- Exceed context limits (causes truncation)

---

## FAQ

**Q: Can I use Codex without an API key?**
A: No, OPENAI_API_KEY is required. Get one at [https://platform.openai.com/api-keys](https://platform.openai.com/api-keys).

**Q: Does Codex work offline?**
A: No, Codex requires internet connection to access OpenAI API.

**Q: Can I use my organization's API key?**
A: Yes, organization keys work. Ensure you have appropriate permissions.

**Q: Are hooks required?**
A: No, hooks are optional. Sessions work fine without them.

**Q: Can I switch from Claude to Codex mid-session?**
A: No, agent is set at session creation. Create new session with different agent.

**Q: Does Codex support streaming responses?**
A: Backend supports streaming via OpenAI API. Frontend integration depends on AGM implementation.

**Q: How do I update the model?**
A: Model selection is configured in backend adapter (`openai_adapter.go`).

---

## Support & Resources

### Documentation
- [OpenAI API Docs](https://platform.openai.com/docs)
- [Azure OpenAI Docs](https://learn.microsoft.com/en-us/azure/ai-services/openai/)
- [AGM Documentation](../README.md)

### Status & Monitoring
- [OpenAI Status](https://status.openai.com)
- [OpenAI Usage Dashboard](https://platform.openai.com/account/usage)
- [Rate Limits](https://platform.openai.com/account/rate-limits)

### Community
- [OpenAI Community](https://community.openai.com)
- [AGM GitHub Issues](https://github.com/vbonnet/dear-agent/issues)

---

**Last Updated**: 2026-03-09
**AGM Version**: Compatible with AGM 1.0+
**OpenAI API Version**: 2023-05-15+
