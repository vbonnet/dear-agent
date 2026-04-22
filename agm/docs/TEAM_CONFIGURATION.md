# Team MCP Configuration Guide

**Audience**: Team Leads and Infrastructure Administrators
**Version**: AGM v3.1
**Last Updated**: 2026-02-15

## Overview

Team MCP configuration allows you to define shared MCP servers for your team. All team members automatically inherit these configurations, ensuring consistent infrastructure access across the team while allowing individual customization when needed.

## Table of Contents

- [Quick Start](#quick-start)
- [Configuration Hierarchy](#configuration-hierarchy)
- [Setting Up Team Configuration](#setting-up-team-configuration)
- [Managing Team Membership](#managing-team-membership)
- [Example Configurations](#example-configurations)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Quick Start

### For Team Leads

1. **Create team configuration directory**:
   ```bash
   mkdir -p ~/.config/agm/teams/my-team
   ```

2. **Create team MCP config**:
   ```bash
   cat > ~/.config/agm/teams/my-team/mcp.yaml << 'EOF'
   team:
     name: my-team
     description: My Team Shared Infrastructure
     owner: team-lead@example.com

   mcp_servers:
     - name: team-github
       url: https://mcp-gateway.example.com/github
       type: mcp
   EOF
   ```

3. **Share team name with members**: Tell team members to run:
   ```bash
   echo "my-team" > ~/.config/agm/team
   ```

### For Team Members

1. **Join team**:
   ```bash
   mkdir -p ~/.config/agm
   echo "my-team" > ~/.config/agm/team
   ```

2. **Verify team config is loaded**:
   ```bash
   agm mcp-status
   # Should show team-github and other team MCPs
   ```

3. **Start using team MCPs**:
   ```bash
   agm session new my-project
   # Team MCPs are automatically available
   ```

## Configuration Hierarchy

AGM uses a hierarchical configuration system with the following precedence (highest to lowest):

1. **Session Config**: `<project>/.agm/mcp.yaml` or `<project>/.config/claude-code/mcp.json`
2. **User Config**: `~/.config/agm/mcp.yaml`
3. **Team Config**: `~/.config/agm/teams/<team-name>/mcp.yaml`
4. **Global Config**: `AGM_MCP_SERVERS` environment variable

### How Merging Works

- Configs are merged by **MCP server name**
- Higher-priority configs **override** lower-priority ones
- If a session config defines a server with the same name as a team config, the session config takes precedence

### Example

**Team Config** (`~/.config/agm/teams/research/mcp.yaml`):
```yaml
mcp_servers:
  - name: github
    url: https://team-mcp.example.com/github
    type: mcp
```

**User Config** (`~/.config/agm/mcp.yaml`):
```yaml
mcp_servers:
  - name: local-tools
    url: http://localhost:8001
    type: mcp
```

**Session Config** (`<project>/.agm/mcp.yaml`):
```yaml
mcp_servers:
  - name: github
    url: http://localhost:8002  # Overrides team GitHub
    type: mcp
```

**Result**: The session will use:
- `github` from **session config** (http://localhost:8002)
- `local-tools` from **user config** (http://localhost:8001)

## Setting Up Team Configuration

### Step 1: Create Team Configuration

Create a team configuration directory and file:

```bash
# Create team directory
mkdir -p ~/.config/agm/teams/engineering

# Create team config
cat > ~/.config/agm/teams/engineering/mcp.yaml << 'EOF'
team:
  name: engineering
  description: Engineering Team Shared Infrastructure
  owner: eng-lead@example.com

mcp_servers:
  - name: team-github
    url: https://mcp-gateway.example.com/github
    type: mcp

  - name: team-jira
    url: https://mcp-gateway.example.com/jira
    type: mcp

  - name: team-docs
    url: https://mcp-gateway.example.com/googledocs
    type: mcp
EOF
```

### Step 2: Distribute to Team

**Option A: Manual Setup** (simplest)

Share instructions with team members:

```bash
# Team members run this command
mkdir -p ~/.config/agm
echo "engineering" > ~/.config/agm/team
```

**Option B: Environment Variable** (flexible)

Team members can set in their shell RC file:

```bash
# Add to ~/.bashrc or ~/.zshrc
export AGM_TEAM=engineering
```

**Option C: Git-based Distribution** (scalable)

1. Create a shared repository for team configs:
   ```bash
   git clone git@github.com:yourorg/team-agm-configs.git ~/team-agm-configs
   ```

2. Symlink team configs:
   ```bash
   ln -s ~/team-agm-configs/engineering ~/.config/agm/teams/engineering
   ```

3. Team members clone and symlink:
   ```bash
   git clone git@github.com:yourorg/team-agm-configs.git ~/team-agm-configs
   ln -s ~/team-agm-configs/engineering ~/.config/agm/teams/engineering
   echo "engineering" > ~/.config/agm/team
   ```

### Step 3: Verify Setup

Team members should verify their setup:

```bash
# Check team membership
cat ~/.config/agm/team
# Expected output: engineering

# Check team config exists
ls ~/.config/agm/teams/engineering/mcp.yaml
# Should exist

# Check team MCPs are loaded
agm mcp-status
# Should show team-github, team-jira, team-docs
```

## Managing Team Membership

### Join a Team

```bash
# Manual method
echo "team-name" > ~/.config/agm/team

# Using environment variable
export AGM_TEAM=team-name
```

### Leave a Team

```bash
# Remove team membership file
rm ~/.config/agm/team

# Or unset environment variable
unset AGM_TEAM
```

### List Available Teams

```bash
# List all team configs in ~/.config/agm/teams/
ls -1 ~/.config/agm/teams/
```

### Check Current Team

```bash
# Method 1: Check team file
cat ~/.config/agm/team

# Method 2: Check environment variable
echo $AGM_TEAM

# Method 3: Check which team MCPs are loaded
agm mcp-status
```

## Example Configurations

### Example 1: Simple Team (Small Startup)

```yaml
# ~/.config/agm/teams/startup/mcp.yaml
team:
  name: startup
  description: Startup Team Infrastructure
  owner: cto@startup.com

mcp_servers:
  - name: github
    url: https://api.github.com/mcp
    type: mcp

  - name: slack
    url: https://slack.com/api/mcp
    type: mcp
```

### Example 2: Enterprise Team (Department)

```yaml
# ~/.config/agm/teams/platform-eng/mcp.yaml
team:
  name: platform-eng
  description: Platform Engineering Team
  owner: platform-lead@enterprise.com

mcp_servers:
  - name: team-github
    url: https://mcp-gateway.internal/github
    type: mcp

  - name: team-jira
    url: https://mcp-gateway.internal/jira
    type: mcp

  - name: team-pagerduty
    url: https://mcp-gateway.internal/pagerduty
    type: mcp

  - name: team-datadog
    url: https://mcp-gateway.internal/datadog
    type: mcp

  - name: internal-api
    url: https://api.internal:8443/mcp
    type: mcp
```

### Example 3: Research Team (Academia)

```yaml
# ~/.config/agm/teams/ml-research/mcp.yaml
team:
  name: ml-research
  description: ML Research Lab Infrastructure
  owner: prof-smith@university.edu

mcp_servers:
  - name: team-github
    url: https://github-mcp.cs.university.edu/mcp
    type: mcp

  - name: team-arxiv
    url: https://arxiv-mcp.cs.university.edu/mcp
    type: mcp

  - name: team-datasets
    url: https://data-mcp.cs.university.edu/mcp
    type: mcp

  - name: team-compute
    url: https://hpc-mcp.cs.university.edu/mcp
    type: mcp
```

### Example 4: Multi-Team Member

A user can be in multiple teams by using environment variables for different projects:

```bash
# ~/.bashrc
alias work-platform='export AGM_TEAM=platform-eng'
alias work-ml='export AGM_TEAM=ml-research'
alias work-personal='unset AGM_TEAM'
```

Then switch teams:
```bash
work-platform  # Use platform-eng team config
agm session new platform-project

work-ml       # Use ml-research team config
agm session new ml-experiment
```

## Best Practices

### 1. Team Configuration Management

- **Version Control**: Store team configs in a Git repository
- **Code Review**: Require PR reviews for team config changes
- **Documentation**: Document each MCP server's purpose
- **Access Control**: Restrict write access to team leads
- **Backup**: Regularly backup team configs

### 2. MCP Server Naming

- **Prefix with `team-`**: Makes it clear these are team resources
- **Use descriptive names**: `team-github` not `gh`
- **Avoid conflicts**: Don't use names that users might use (`local`, `test`, etc.)
- **Document conventions**: Create a naming guide for your team

### 3. Security

- **No Secrets in Configs**: Never put API keys in team configs
- **Use Internal URLs**: Prefer `https://mcp-gateway.internal` over public URLs
- **Rotate Credentials**: Regularly rotate MCP server credentials
- **Audit Access**: Monitor which team members use which MCPs
- **Principle of Least Privilege**: Only expose MCPs team members need

### 4. Testing Changes

Before rolling out team config changes:

```bash
# Test in your user config first
cat > ~/.config/agm/mcp.yaml << 'EOF'
mcp_servers:
  - name: team-github-test
    url: https://new-mcp-gateway.example.com/github
    type: mcp
EOF

# Verify it works
agm mcp-status
agm session new test-project

# Then promote to team config if successful
```

### 5. Communication

- **Announce Changes**: Notify team before updating team configs
- **Migration Path**: Provide clear upgrade instructions
- **Support Channel**: Set up a Slack channel for MCP questions
- **Office Hours**: Hold regular Q&A sessions
- **Documentation**: Maintain team-specific MCP docs

### 6. Monitoring

- **Health Checks**: Monitor team MCP server uptime
- **Usage Metrics**: Track which MCPs are most used
- **Error Rates**: Alert on MCP connection failures
- **Performance**: Monitor MCP response times
- **Capacity**: Plan for team growth

## Troubleshooting

### Team Config Not Loading

**Symptom**: `agm mcp-status` doesn't show team MCPs

**Check**:
1. Verify team membership:
   ```bash
   cat ~/.config/agm/team
   echo $AGM_TEAM
   ```

2. Verify team config exists:
   ```bash
   ls ~/.config/agm/teams/$(cat ~/.config/agm/team)/mcp.yaml
   ```

3. Verify config is valid YAML:
   ```bash
   cat ~/.config/agm/teams/$(cat ~/.config/agm/team)/mcp.yaml
   ```

4. Check for parsing errors:
   ```bash
   agm mcp-status --verbose
   ```

### Team Name Mismatch

**Symptom**: Error: "team config mismatch: expected X, got Y"

**Cause**: Team membership file has different name than config

**Fix**:
```bash
# Check team membership
cat ~/.config/agm/team

# Check team config name
grep "name:" ~/.config/agm/teams/*/mcp.yaml

# Update team membership to match
echo "correct-team-name" > ~/.config/agm/team
```

### MCP Server Conflicts

**Symptom**: Wrong MCP server being used

**Cause**: Config precedence (session/user overriding team)

**Debug**:
1. Check session config:
   ```bash
   cat <project>/.agm/mcp.yaml
   ```

2. Check user config:
   ```bash
   cat ~/.config/agm/mcp.yaml
   ```

3. Check team config:
   ```bash
   cat ~/.config/agm/teams/$(cat ~/.config/agm/team)/mcp.yaml
   ```

4. Identify which config defines the conflicting MCP

**Fix**: Either:
- Remove MCP from higher-priority config, OR
- Rename MCP in team config to avoid conflict

### Permission Denied

**Symptom**: Error reading team config file

**Cause**: File permissions too restrictive

**Fix**:
```bash
chmod 644 ~/.config/agm/teams/*/mcp.yaml
chmod 755 ~/.config/agm/teams/*
```

### Team Config Not Found

**Symptom**: Team membership set but config doesn't exist

**Cause**: Team lead hasn't set up config yet

**Fix** (Team Lead):
```bash
mkdir -p ~/.config/agm/teams/team-name
cat > ~/.config/agm/teams/team-name/mcp.yaml << 'EOF'
team:
  name: team-name
  description: Team Description
  owner: lead@example.com

mcp_servers:
  - name: example-mcp
    url: https://mcp.example.com
    type: mcp
EOF
```

## Advanced Topics

### Dynamic Team Selection

Use shell functions to switch teams dynamically:

```bash
# Add to ~/.bashrc or ~/.zshrc
agm-team() {
  if [ -z "$1" ]; then
    # Show current team
    if [ -n "$AGM_TEAM" ]; then
      echo "Current team (env): $AGM_TEAM"
    elif [ -f ~/.config/agm/team ]; then
      echo "Current team (file): $(cat ~/.config/agm/team)"
    else
      echo "No team set"
    fi
  else
    # Set team
    export AGM_TEAM="$1"
    echo "Team set to: $1"
  fi
}
```

Usage:
```bash
agm-team                # Show current team
agm-team engineering    # Switch to engineering team
agm-team personal       # Switch to personal team
```

### Team Config Validation

Validate team config before deploying:

```bash
# Check YAML syntax
cat ~/.config/agm/teams/team-name/mcp.yaml | python3 -c "import sys, yaml; yaml.safe_load(sys.stdin)"

# Check required fields
grep -q "team:" ~/.config/agm/teams/team-name/mcp.yaml && \
grep -q "name:" ~/.config/agm/teams/team-name/mcp.yaml && \
grep -q "mcp_servers:" ~/.config/agm/teams/team-name/mcp.yaml && \
echo "Config valid" || echo "Config missing required fields"
```

### Centralized Team Config Distribution

For large organizations, use a config management system:

```bash
# Ansible playbook example
- name: Deploy team MCP config
  copy:
    src: team-configs/{{ team_name }}/mcp.yaml
    dest: ~/.config/agm/teams/{{ team_name }}/mcp.yaml
    mode: 0644

- name: Set team membership
  copy:
    content: "{{ team_name }}\n"
    dest: ~/.config/agm/team
    mode: 0644
```

### Team-Specific MCP Policies

Implement policies in your team config:

```yaml
team:
  name: security-team
  description: Security Team Infrastructure
  owner: security-lead@example.com
  policies:
    # Document policies (not enforced by AGM currently)
    require_tls: true
    require_auth: true
    max_sessions: 10

mcp_servers:
  - name: security-scanner
    url: https://secure-mcp.internal:8443/scanner
    type: mcp
    # Policy notes
    # - Requires VPN connection
    # - Rate limited to 10 req/sec per user
    # - Audit logged to SIEM
```

## Support and Resources

- **Documentation**: `main/agm/docs/`
- **Examples**: `main/agm/examples/`
- **Issues**: File bug reports in your team's issue tracker
- **Questions**: Ask in your team's communication channel

## Related Documentation

- [Global MCP Quick Start](GLOBAL_MCP_QUICKSTART.md)
- [Global MCP Integration](global-mcp-integration.md)
- [AGM Configuration Guide](CONFIGURATION.md)
- [AGM User Guide](USER-GUIDE.md)
