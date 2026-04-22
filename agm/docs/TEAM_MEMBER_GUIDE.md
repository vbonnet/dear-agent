# Team MCP Configuration - Member Guide

**Audience**: Team Members
**Version**: AGM v3.1
**Last Updated**: 2026-02-15

## Quick Start (2 minutes)

Your team lead has set up shared MCP servers. Follow these steps to start using them:

### Step 1: Join Your Team

```bash
# Create config directory
mkdir -p ~/.config/agm

# Join your team (replace 'engineering' with your team name)
echo "engineering" > ~/.config/agm/team
```

**That's it!** Team MCPs are now available in all your AGM sessions.

### Step 2: Verify Setup

```bash
# Check MCP status
agm mcp-status

# You should see team MCPs like:
# NAME           STATUS        URL
# ----           ------        ---
# team-github    AVAILABLE     https://mcp-gateway.example.com/github
# team-jira      AVAILABLE     https://mcp-gateway.example.com/jira
```

### Step 3: Start Using Team MCPs

```bash
# Create a new session
agm session new my-project

# Team MCPs are automatically available in your session
# No additional configuration needed!
```

## What Are Team MCPs?

**Team MCPs** are shared MCP servers configured by your team lead. They provide:

- **Shared infrastructure**: Everyone uses the same MCP servers
- **Consistent setup**: No need to configure MCPs individually
- **Centralized management**: Team lead maintains configurations
- **Easy access**: Automatically available in all sessions

## Common Team MCPs

Your team may have configured these MCPs:

| MCP Name | Purpose | Example Usage |
|----------|---------|---------------|
| `team-github` | Access team GitHub repositories | View PRs, create issues |
| `team-jira` | Access team Jira board | Check sprint tasks, update tickets |
| `team-docs` | Access shared Google Docs | Read team documentation |
| `team-slack` | Access team Slack workspace | Send messages, check notifications |
| `team-data` | Access team datasets | Query internal databases |

**Ask your team lead** for a list of available team MCPs and their purposes.

## Configuration Hierarchy

When you use AGM, configurations are loaded in this order:

1. **Team Config** (lowest priority) - Shared by all team members
2. **Your User Config** - Your personal MCP configurations
3. **Session Config** (highest priority) - Project-specific configurations

**What this means:**
- Team MCPs are available by default
- You can add your own MCPs in your user config
- You can override team MCPs for specific projects

### Example

**Team Config** (set by team lead):
- `team-github` → https://mcp-gateway.example.com/github

**Your User Config** (`~/.config/agm/mcp.yaml`):
- `personal-notes` → http://localhost:8001

**Session Config** (`<project>/.agm/mcp.yaml`):
- `team-github` → http://localhost:8002 (for testing)

**Result in session:**
- `team-github` uses localhost:8002 (session override)
- `personal-notes` uses localhost:8001 (user config)
- Other team MCPs work as configured by team lead

## Personalizing Your Setup

### Add Your Own MCPs

You can add personal MCPs without affecting team configuration:

```bash
# Create your user config
cat > ~/.config/agm/mcp.yaml << 'EOF'
mcp_servers:
  - name: my-local-tools
    url: http://localhost:8001
    type: mcp

  - name: my-notes
    url: http://localhost:8002
    type: mcp
EOF
```

Your personal MCPs are **only available to you** and don't affect other team members.

### Override Team MCPs (For Testing)

To test a different version of a team MCP:

```bash
# In your project directory
mkdir -p .agm

cat > .agm/mcp.yaml << 'EOF'
mcp_servers:
  - name: team-github  # Same name as team MCP
    url: http://localhost:8002  # Your test server
    type: mcp
EOF
```

This override **only affects this project**. Other projects still use the team MCP.

**Warning**: Overriding team MCPs should only be done for testing. Always switch back to team MCPs for production work.

## Switching Teams

If you're a member of multiple teams:

### Method 1: Environment Variable (Recommended)

```bash
# Add to ~/.bashrc or ~/.zshrc
alias team-eng='export AGM_TEAM=engineering'
alias team-research='export AGM_TEAM=research'
alias team-none='unset AGM_TEAM'
```

Usage:
```bash
team-eng        # Use engineering team MCPs
agm session new eng-project

team-research   # Use research team MCPs
agm session new research-project

team-none       # Use only your personal MCPs
agm session new personal-project
```

### Method 2: Team File (Simpler)

```bash
# Switch to engineering team
echo "engineering" > ~/.config/agm/team

# Switch to research team
echo "research" > ~/.config/agm/team

# Leave all teams
rm ~/.config/agm/team
```

## Troubleshooting

### Team MCPs Not Showing Up

**Check team membership:**
```bash
cat ~/.config/agm/team
# Should show your team name
```

**If empty or wrong:**
```bash
echo "your-team-name" > ~/.config/agm/team
```

**Verify team config exists:**
```bash
ls ~/.config/agm/teams/$(cat ~/.config/agm/team)/mcp.yaml
# Should exist and not show error
```

**If missing:**
Contact your team lead. They need to set up the team configuration.

### Team MCP Connection Failed

**Check MCP status:**
```bash
agm mcp-status

# If shows UNAVAILABLE:
# NAME           STATUS        URL
# team-github    UNAVAILABLE   https://mcp-gateway.example.com/github
```

**Possible causes:**
1. MCP server is down - Contact team lead
2. VPN required - Connect to VPN first
3. Network issues - Check internet connection
4. Authentication failed - Verify environment variables

**For authentication issues:**
```bash
# Check if required environment variables are set
env | grep -i token
env | grep -i api

# If missing, ask team lead which variables are needed
```

### Wrong Team MCPs Loading

**Check which team you're in:**
```bash
cat ~/.config/agm/team
echo $AGM_TEAM
```

**If both are set:**
Environment variable (`$AGM_TEAM`) takes precedence over file.

**To fix:**
```bash
# Use environment variable
export AGM_TEAM=correct-team

# Or use file only
unset AGM_TEAM
echo "correct-team" > ~/.config/agm/team
```

### Can't Override Team MCP

**Verify your override is in session config:**
```bash
cat .agm/mcp.yaml
# Should contain your override
```

**Check YAML syntax:**
```bash
# YAML is sensitive to indentation
# Use spaces, not tabs
# Example:
mcp_servers:
  - name: team-github    # Note: 2 spaces for indent
    url: http://localhost:8002
    type: mcp
```

### Permission Issues

**Error reading team config:**
```bash
# Fix permissions
chmod 644 ~/.config/agm/teams/*/mcp.yaml
chmod 755 ~/.config/agm/teams/*
```

## Best Practices

### 1. Don't Modify Team Configs

Team configs are managed by your team lead. Changes will be overwritten.

**If you need changes:**
1. Ask your team lead to update team config
2. Or add to your user config instead
3. Or use session config for project-specific overrides

### 2. Use Descriptive Names

When adding personal MCPs, use clear names:

```yaml
# Good
mcp_servers:
  - name: my-local-dev-server
  - name: personal-testing-mcp

# Bad (conflicts with team MCPs)
mcp_servers:
  - name: github
  - name: jira
```

### 3. Document Project Overrides

If you override team MCPs in a project:

```yaml
# .agm/mcp.yaml
mcp_servers:
  # OVERRIDE: Using local GitHub for testing PR #123
  # TODO: Remove after testing complete
  - name: team-github
    url: http://localhost:8002
    type: mcp
```

### 4. Keep Secrets Out of Configs

**Never put API keys in config files:**

```yaml
# BAD - Don't do this
mcp_servers:
  - name: my-mcp
    url: https://api.example.com?api_key=secret123

# GOOD - Use environment variables
mcp_servers:
  - name: my-mcp
    url: https://api.example.com
    # API key from environment variable in MCP server config
```

### 5. Stay Updated

Team configs may change. Stay informed:

- Watch team communication channels
- Read team lead announcements
- Check team config changelog (if maintained)
- Ask questions when unsure

## FAQs

### Q: How do I know what team MCPs are available?

**A:** Run `agm mcp-status` to see all available MCPs, including team MCPs.

Or ask your team lead for documentation.

### Q: Can I use team MCPs in my personal projects?

**A:** Yes, as long as you're a team member. Team MCPs are available in all your sessions.

### Q: What if I don't want to use team MCPs?

**A:** Leave the team:
```bash
rm ~/.config/agm/team
unset AGM_TEAM
```

Or override them in your user config with different servers using the same names.

### Q: Can I share my user config with teammates?

**A:** Yes, but be careful:
- Don't share configs with API keys or secrets
- Your config might not work for others (different paths, etc.)
- Better to ask team lead to add to team config

### Q: How do I test a new MCP before suggesting it to the team?

**A:** Add it to your user config first:

```yaml
# ~/.config/agm/mcp.yaml
mcp_servers:
  - name: proposed-new-mcp
    url: https://new-mcp.example.com
    type: mcp
```

Test it thoroughly, then suggest to team lead.

### Q: What environment variables do I need?

**A:** Ask your team lead. Common ones:
- `GITHUB_TOKEN` - For GitHub MCP
- `JIRA_TOKEN` - For Jira MCP
- `SLACK_TOKEN` - For Slack MCP
- `INTERNAL_API_KEY` - For internal MCPs

### Q: Can I see the team config file?

**A:** Yes:
```bash
cat ~/.config/agm/teams/$(cat ~/.config/agm/team)/mcp.yaml
```

But don't edit it - changes will be lost when team lead updates.

### Q: How do I join a different team's MCP servers temporarily?

**A:** Use environment variable:
```bash
AGM_TEAM=other-team agm session new temp-project
```

This only affects that one command.

## Getting Help

**Team-Specific Questions:**
- Ask in team communication channel
- Contact your team lead
- Check team documentation

**Technical Issues:**
- Run `agm doctor` for diagnostics
- Check AGM documentation
- File issue in team's issue tracker

**Configuration Help:**
- See [Team Configuration Guide](TEAM_CONFIGURATION.md) (for team leads)
- See [AGM User Guide](USER-GUIDE.md)
- See [Global MCP Quick Start](GLOBAL_MCP_QUICKSTART.md)

## Related Documentation

- [Team Configuration Guide](TEAM_CONFIGURATION.md) - For team leads
- [Global MCP Integration](global-mcp-integration.md) - Technical details
- [AGM User Guide](USER-GUIDE.md) - General AGM usage
- [Configuration Reference](CONFIGURATION.md) - All config options

---

**Remember:** Team MCPs make your life easier. If you have questions or issues, don't hesitate to ask your team lead!
