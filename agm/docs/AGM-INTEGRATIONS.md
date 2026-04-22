# AGM Integrations Guide

Guide for integrating AGM (AI/Agent Session Manager) with third-party tools and workflows.

## Table of Contents

- [IDE Integrations](#ide-integrations)
- [Version Control](#version-control)
- [CI/CD Pipelines](#cicd-pipelines)
- [Notification Systems](#notification-systems)
- [Cloud Platforms](#cloud-platforms)
- [MCP Integration](#mcp-integration)
- [Custom Integrations](#custom-integrations)

## IDE Integrations

### VS Code

**AGM Extension (Future)**

Planned VS Code extension features:
- Session picker in command palette
- Status bar showing active sessions
- Quick session creation from explorer
- Integrated terminal with AGM sessions

**Current workaround:**
```json
// .vscode/tasks.json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "AGM: New Session",
      "type": "shell",
      "command": "agm",
      "args": ["new", "${input:sessionName}"],
      "problemMatcher": []
    },
    {
      "label": "AGM: Resume Session",
      "type": "shell",
      "command": "agm",
      "args": ["resume", "${input:sessionName}"],
      "problemMatcher": []
    }
  ],
  "inputs": [
    {
      "id": "sessionName",
      "type": "promptString",
      "description": "Session name"
    }
  ]
}
```

**Keyboard shortcuts:**
```json
// .vscode/keybindings.json
[
  {
    "key": "ctrl+alt+a n",
    "command": "workbench.action.tasks.runTask",
    "args": "AGM: New Session"
  },
  {
    "key": "ctrl+alt+a r",
    "command": "workbench.action.tasks.runTask",
    "args": "AGM: Resume Session"
  }
]
```

### Neovim

**AGM integration plugin (Community)**

```lua
-- ~/.config/nvim/lua/agm.lua
local M = {}

function M.new_session()
  local name = vim.fn.input("Session name: ")
  if name ~= "" then
    vim.cmd("!agm new " .. name)
  end
end

function M.resume_session()
  -- Show session picker
  vim.cmd("!agm")
end

function M.list_sessions()
  vim.cmd("!agm list")
end

return M
```

**Key mappings:**
```lua
-- ~/.config/nvim/init.lua
local agm = require('agm')

vim.keymap.set('n', '<leader>an', agm.new_session, { desc = 'AGM: New session' })
vim.keymap.set('n', '<leader>ar', agm.resume_session, { desc = 'AGM: Resume session' })
vim.keymap.set('n', '<leader>al', agm.list_sessions, { desc = 'AGM: List sessions' })
```

### JetBrains IDEs (IntelliJ, PyCharm, etc.)

**External tool configuration:**

1. Go to: Settings → Tools → External Tools → Add
2. Create tool:
   - **Name:** AGM New Session
   - **Program:** `agm`
   - **Arguments:** `new $Prompt$`
   - **Working directory:** `$ProjectFileDir$`

3. Assign keyboard shortcut:
   - Settings → Keymap → External Tools → AGM New Session
   - Add keyboard shortcut: `Ctrl+Alt+A N`

## Version Control

### Git Hooks

**Pre-commit hook (Session validation):**
```bash
#!/bin/bash
# .git/hooks/pre-commit

# Check if committing session changes
if git diff --cached --name-only | grep -q "sessions/"; then
  echo "Detected session changes, validating..."

  # Run session health check
  agm doctor --validate --json > /tmp/agm-health.json

  if [ $? -ne 0 ]; then
    echo "❌ Session validation failed!"
    echo "Run 'agm doctor --validate --fix' to repair"
    exit 1
  fi

  echo "✓ Session validation passed"
fi

exit 0
```

**Post-merge hook (Session sync):**
```bash
#!/bin/bash
# .git/hooks/post-merge

# Check if session manifests changed
if git diff --name-only HEAD@{1} HEAD | grep -q "sessions/.*manifest.yaml"; then
  echo "Session manifests changed, syncing..."

  # Restore archived sessions that were added
  for manifest in $(git diff --name-only HEAD@{1} HEAD | grep "manifest.yaml"); then
    session=$(basename $(dirname $manifest))
    agm unarchive $session --force
  done
fi
```

### GitHub Actions

**Session health check workflow:**
```yaml
# .github/workflows/agm-health.yml
name: AGM Health Check

on:
  push:
    paths:
      - 'sessions/**'
  pull_request:
    paths:
      - 'sessions/**'

jobs:
  health-check:
    runs-on: ubuntu-latest

    steps:
    - uses: actions/checkout@v3

    - name: Install AGM
      run: |
        go install github.com/vbonnet/ai-tools/agm/cmd/agm@latest

    - name: Install tmux
      run: sudo apt-get install -y tmux

    - name: Run health check
      run: agm doctor --validate --json

    - name: Upload results
      if: always()
      uses: actions/upload-artifact@v3
      with:
        name: agm-health-report
        path: /tmp/agm-health.json
```

**Automated session backup:**
```yaml
# .github/workflows/backup-sessions.yml
name: Backup AGM Sessions

on:
  schedule:
    - cron: '0 2 * * *'  # Daily at 2 AM
  workflow_dispatch:

jobs:
  backup:
    runs-on: ubuntu-latest

    steps:
    - name: Checkout sessions
      uses: actions/checkout@v3
      with:
        path: sessions

    - name: Create backup
      run: |
        tar czf sessions-$(date +%Y%m%d).tar.gz sessions/

    - name: Upload to release
      uses: softprops/action-gh-release@v1
      with:
        tag_name: backup-$(date +%Y%m%d)
        files: sessions-*.tar.gz
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### GitLab CI

```yaml
# .gitlab-ci.yml
stages:
  - test
  - deploy

agm-health-check:
  stage: test
  image: golang:1.24
  before_script:
    - go install github.com/vbonnet/ai-tools/agm/cmd/agm@latest
    - apt-get update && apt-get install -y tmux
  script:
    - agm doctor --validate --json
  artifacts:
    when: always
    paths:
      - /tmp/agm-health.json
    reports:
      junit: /tmp/agm-health.json
  only:
    changes:
      - sessions/**
```

## CI/CD Pipelines

### Jenkins

**Jenkinsfile example:**
```groovy
pipeline {
    agent any

    environment {
        ANTHROPIC_API_KEY = credentials('anthropic-api-key')
    }

    stages {
        stage('Setup AGM') {
            steps {
                sh 'go install github.com/vbonnet/ai-tools/agm/cmd/agm@latest'
            }
        }

        stage('Create Session') {
            steps {
                sh 'agm new --harness claude-code ci-build-${BUILD_NUMBER}'
            }
        }

        stage('Run Tests') {
            steps {
                sh 'agm session send ci-build-${BUILD_NUMBER} --prompt "Run test suite"'
            }
        }

        stage('Cleanup') {
            steps {
                sh 'agm kill ci-build-${BUILD_NUMBER}'
            }
            post {
                always {
                    sh 'agm archive ci-build-${BUILD_NUMBER} --force'
                }
            }
        }
    }
}
```

### CircleCI

```yaml
# .circleci/config.yml
version: 2.1

jobs:
  agm-test:
    docker:
      - image: cimg/go:1.24
    steps:
      - checkout
      - run:
          name: Install AGM
          command: go install github.com/vbonnet/ai-tools/agm/cmd/agm@latest
      - run:
          name: Install tmux
          command: sudo apt-get update && sudo apt-get install -y tmux
      - run:
          name: Health check
          command: agm doctor --validate

workflows:
  version: 2
  test:
    jobs:
      - agm-test
```

## Notification Systems

### Slack

**Session event notifications:**
```bash
#!/bin/bash
# agm-slack-notify.sh

SESSION=$1
EVENT=$2  # created, archived, failed
WEBHOOK_URL="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"

MESSAGE=""
case $EVENT in
  created)
    MESSAGE="✅ Session \`$SESSION\` created"
    ;;
  archived)
    MESSAGE="📦 Session \`$SESSION\` archived"
    ;;
  failed)
    MESSAGE="❌ Session \`$SESSION\` health check failed"
    ;;
esac

curl -X POST -H 'Content-type: application/json' \
  --data "{\"text\":\"$MESSAGE\"}" \
  $WEBHOOK_URL
```

**Hook into AGM commands:**
```bash
# ~/.bashrc or wrapper script
agm() {
  local cmd=$1
  shift

  # Run AGM command
  command agm $cmd "$@"
  local result=$?

  # Notify on specific events
  case $cmd in
    new|create)
      ./agm-slack-notify.sh "$1" "created"
      ;;
    archive)
      ./agm-slack-notify.sh "$1" "archived"
      ;;
  esac

  return $result
}
```

### Discord

**Discord webhook integration:**
```python
#!/usr/bin/env python3
# agm-discord-notify.py

import sys
import requests

WEBHOOK_URL = "https://discord.com/api/webhooks/YOUR/WEBHOOK/URL"

def send_notification(session, event):
    messages = {
        "created": f"✅ Session `{session}` created",
        "archived": f"📦 Session `{session}` archived",
        "failed": f"❌ Session `{session}` health check failed"
    }

    data = {
        "content": messages.get(event, f"Session `{session}`: {event}"),
        "username": "AGM Bot"
    }

    requests.post(WEBHOOK_URL, json=data)

if __name__ == "__main__":
    session = sys.argv[1]
    event = sys.argv[2]
    send_notification(session, event)
```

### Email

**Email notifications via SMTP:**
```bash
#!/bin/bash
# agm-email-notify.sh

SESSION=$1
EVENT=$2
TO_EMAIL="admin@example.com"

# Create email body
BODY="AGM Session Event\n\nSession: $SESSION\nEvent: $EVENT\nTime: $(date)\n"

# Send email (requires mail command)
echo -e "$BODY" | mail -s "AGM: $EVENT - $SESSION" $TO_EMAIL
```

## Cloud Platforms

### Google Cloud

**Vertex AI integration (Semantic search):**
```bash
# Setup
gcloud auth application-default login
export GOOGLE_CLOUD_PROJECT=my-project

# Use semantic search
agm search "OAuth integration work"
```

**Cloud Storage backup:**
```bash
#!/bin/bash
# backup-to-gcs.sh

BUCKET="gs://my-agm-backups"
DATE=$(date +%Y%m%d)

# Backup sessions
tar czf /tmp/sessions-$DATE.tar.gz ~/sessions

# Upload to GCS
gsutil cp /tmp/sessions-$DATE.tar.gz $BUCKET/

# Cleanup
rm /tmp/sessions-$DATE.tar.gz

# Cleanup old backups (keep 30 days)
gsutil ls $BUCKET/ | while read -r file; do
  age=$(( ($(date +%s) - $(gsutil stat "$file" | grep "Creation time" | awk '{print $3" "$4}' | xargs -I{} date -d {} +%s)) / 86400 ))
  if [ $age -gt 30 ]; then
    gsutil rm "$file"
  fi
done
```

### AWS

**S3 backup:**
```bash
#!/bin/bash
# backup-to-s3.sh

BUCKET="s3://my-agm-backups"
DATE=$(date +%Y%m%d)

# Backup sessions
tar czf /tmp/sessions-$DATE.tar.gz ~/sessions

# Upload to S3
aws s3 cp /tmp/sessions-$DATE.tar.gz $BUCKET/

# Cleanup
rm /tmp/sessions-$DATE.tar.gz

# Lifecycle policy (configure once)
aws s3api put-bucket-lifecycle-configuration \
  --bucket my-agm-backups \
  --lifecycle-configuration '{
    "Rules": [{
      "Id": "DeleteOldBackups",
      "Status": "Enabled",
      "Filter": { "Prefix": "sessions-" },
      "Expiration": { "Days": 30 }
    }]
  }'
```

## MCP Integration

**Model Context Protocol (MCP) integration:**

AGM includes an experimental MCP server for integration with MCP-compatible tools.

**Start MCP server:**
```bash
# Start AGM MCP server
agm mcp-server --port 3000

# Or via systemd
systemctl --user start agm-mcp-server
```

**MCP configuration (`mcp.json`):**
```json
{
  "servers": {
    "agm": {
      "url": "http://localhost:3000",
      "capabilities": [
        "session-management",
        "agent-routing",
        "semantic-search"
      ]
    }
  }
}
```

**Using MCP with VS Code:**
```json
// .vscode/settings.json
{
  "mcp.servers": [
    {
      "name": "agm",
      "url": "http://localhost:3000"
    }
  ]
}
```

## Custom Integrations

### REST API (Future)

**Planned REST API for AGM control:**

```bash
# Start API server (future feature)
agm api-server --port 8080

# Create session via API
curl -X POST http://localhost:8080/sessions \
  -H "Content-Type: application/json" \
  -d '{"name": "api-session", "agent": "claude"}'

# List sessions
curl http://localhost:8080/sessions

# Get session details
curl http://localhost:8080/sessions/api-session
```

### Webhook Support (Future)

**Planned webhook configuration:**
```yaml
# ~/.config/agm/config.yaml
webhooks:
  - event: session.created
    url: https://myapp.com/webhooks/agm
    headers:
      Authorization: "Bearer token123"

  - event: session.failed
    url: https://monitoring.com/alerts
    method: POST
```

### Custom Scripts

**Session lifecycle hooks:**
```bash
# ~/.config/agm/hooks/on-session-create.sh
#!/bin/bash
SESSION=$1

# Custom logic on session creation
echo "Session $SESSION created at $(date)" >> ~/agm-audit.log

# Send to monitoring
curl -X POST https://monitoring.com/events \
  -d "session=$SESSION&event=created&timestamp=$(date +%s)"
```

**Enable hooks:**
```yaml
# ~/.config/agm/config.yaml
hooks:
  on_session_create: "~/.config/agm/hooks/on-session-create.sh"
  on_session_archive: "~/.config/agm/hooks/on-session-archive.sh"
  on_health_check_fail: "~/.config/agm/hooks/on-health-fail.sh"
```

## Examples

### Example 1: Automated CI Session

```bash
#!/bin/bash
# ci-test-session.sh

# Create CI session
SESSION="ci-test-$(date +%Y%m%d-%H%M%S)"
agm new --harness claude-code "$SESSION" --detached

# Send test prompt
agm session send "$SESSION" --prompt "Run full test suite and report results"

# Wait for completion (poll for status)
while true; do
  STATUS=$(agm logs query --session "$SESSION" --last 1 | jq -r '.status')
  if [ "$STATUS" == "completed" ]; then
    break
  fi
  sleep 10
done

# Archive session
agm archive "$SESSION" --force

# Send results to Slack
./agm-slack-notify.sh "$SESSION" "completed"
```

### Example 2: Daily Research Session

```bash
#!/bin/bash
# daily-research.sh

# Create or resume daily research session
DATE=$(date +%Y-%m-%d)
SESSION="research-$DATE"

agm resume "$SESSION" || agm new --harness gemini-cli "$SESSION"

# Send research query
agm session send "$SESSION" --prompt-file ~/research/daily-topics.txt

# Log session activity
agm logs query --session "$SESSION" > ~/research/logs/$DATE.log
```

## Getting Help

- **Documentation:** [docs/INDEX.md](docs/INDEX.md)
- **Examples:** [docs/EXAMPLES.md](docs/EXAMPLES.md)
- **Issues:** https://github.com/vbonnet/ai-tools/issues

---

**Last Updated:** 2026-02-04
**AGM Version:** 3.0
**Maintainer:** Foundation Engineering Team
