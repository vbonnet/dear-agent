# AGM Deployment Guide

Comprehensive guide for deploying AGM (AI/Agent Session Manager) in various environments.

## Table of Contents

- [Installation Methods](#installation-methods)
- [System Requirements](#system-requirements)
- [Single User Installation](#single-user-installation)
- [System-Wide Installation](#system-wide-installation)
- [Container Deployment](#container-deployment)
- [Configuration Management](#configuration-management)
- [Production Considerations](#production-considerations)
- [Troubleshooting](#troubleshooting)

## Installation Methods

### Quick Install (Recommended)

```bash
# Install via Go
go install github.com/vbonnet/dear-agent/agm/cmd/agm@latest

# Verify installation
agm --version

# Enable bash completion
cd ~/src/ai-tools/agm
./scripts/setup-completion.sh
```

### From Source

```bash
# Clone repository
git clone https://github.com/vbonnet/dear-agent.git
cd ai-tools/agm

# Build
go build -o agm ./cmd/agm

# Install to $GOPATH/bin
go install ./cmd/agm

# Or install to custom location
cp agm /usr/local/bin/
chmod +x /usr/local/bin/agm
```

### Binary Release (Coming Soon)

```bash
# Download latest release
wget https://github.com/vbonnet/dear-agent/releases/download/v3.0.0/agm-linux-amd64
chmod +x agm-linux-amd64
sudo mv agm-linux-amd64 /usr/local/bin/agm

# Verify
agm --version
```

## System Requirements

### Minimum Requirements

- **OS:** Linux (Ubuntu 20.04+, Debian 11+, Fedora 35+) or macOS 11+
- **CPU:** 1 core
- **RAM:** 512MB
- **Disk:** 100MB for AGM + storage for session data
- **Go:** 1.24+ (if building from source)
- **tmux:** 2.8+

### Recommended Requirements

- **OS:** Ubuntu 22.04 LTS or macOS 13+
- **CPU:** 2+ cores
- **RAM:** 2GB+
- **Disk:** 1GB+ for sessions and logs
- **Go:** Latest stable version
- **tmux:** 3.0+

### Dependencies

**Required:**
```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install tmux git

# Fedora/RHEL
sudo dnf install tmux git

# macOS
brew install tmux git
```

**Optional (for specific agents):**
```bash
# Claude CLI (official Anthropic CLI)
# Installation instructions: https://docs.anthropic.com/claude/reference/cli

# Gemini CLI
npm install -g @google/generative-ai-cli

# OpenAI CLI
pip install openai-cli
```

## Single User Installation

### Standard Installation

```bash
# 1. Install AGM
go install github.com/vbonnet/dear-agent/agm/cmd/agm@latest

# 2. Configure API keys
cat >> ~/.bashrc << EOF
# AI Agent API Keys
export ANTHROPIC_API_KEY="sk-ant-..."
export GEMINI_API_KEY="AIza..."
export OPENAI_API_KEY="sk-..."
EOF

source ~/.bashrc

# 3. Create configuration
mkdir -p ~/.config/agm
cat > ~/.config/agm/config.yaml << EOF
defaults:
  interactive: true
  auto_associate_uuid: true
  confirm_destructive: true

ui:
  theme: "agm"
  fuzzy_search: true

advanced:
  tmux_timeout: "5s"
EOF

# 4. Enable bash completion
cd ~/go/src/github.com/vbonnet/dear-agent/agm
./scripts/setup-completion.sh

# 5. Verify installation
agm doctor
```

### Directory Structure

AGM creates the following directories:

```
~/.config/agm/          # Configuration files
  └── config.yaml       # User configuration

~/sessions/             # Session storage (unified, v3+)
  └── <session-name>/
      ├── manifest.yaml # Session metadata
      ├── session-env/  # Agent-specific state
      └── backups/      # Session backups

~/.claude-sessions/     # Legacy session storage (v2)

~/.agm/                 # AGM runtime data
  ├── logs/
  │   └── messages/     # Message logs
  └── cache/            # Temporary cache files

~/.claude/              # Claude CLI data
  └── history.jsonl     # Claude conversation history
```

### Permissions

```bash
# Set recommended permissions
chmod 700 ~/sessions
chmod 700 ~/.agm/logs
chmod 600 ~/.config/agm/config.yaml
chmod 600 ~/.bashrc  # Contains API keys
```

## System-Wide Installation

### Binary Installation

```bash
# 1. Build AGM
git clone https://github.com/vbonnet/dear-agent.git
cd ai-tools/agm
go build -o agm ./cmd/agm

# 2. Install system-wide
sudo install -m 755 agm /usr/local/bin/agm

# 3. Verify
agm --version

# 4. Create default configuration template
sudo mkdir -p /etc/agm
sudo cat > /etc/agm/config.yaml.example << EOF
defaults:
  interactive: true
  auto_associate_uuid: true

ui:
  theme: "agm"

advanced:
  tmux_timeout: "5s"
EOF

# 5. Install bash completion system-wide
sudo cp scripts/agm-completion.bash /etc/bash_completion.d/agm
```

### Package Manager (Future)

```bash
# Ubuntu/Debian (planned)
sudo apt-get install agm

# Fedora/RHEL (planned)
sudo dnf install agm

# macOS (planned)
brew install agm
```

### Multi-User Configuration

Each user maintains their own:
- Session storage (`~/sessions/`)
- Configuration (`~/.config/agm/`)
- API keys (environment variables)
- Message logs (`~/.agm/logs/`)

**Administrator setup:**
```bash
# Create default configuration for new users
cat > /etc/skel/.config/csm/config.yaml << EOF
defaults:
  interactive: true
  auto_associate_uuid: true

ui:
  theme: "agm"
EOF

# Set default permissions
chmod 700 /etc/skel/.config/csm
chmod 600 /etc/skel/.config/csm/config.yaml

# New users will inherit this configuration
```

## Container Deployment

### Docker

**Dockerfile:**
```dockerfile
FROM golang:1.24-alpine AS builder

# Install dependencies
RUN apk add --no-cache git tmux

# Build AGM
WORKDIR /build
COPY . .
RUN go mod download
RUN go build -o agm ./cmd/agm

FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache tmux bash

# Copy AGM binary
COPY --from=builder /build/agm /usr/local/bin/agm

# Create directories
RUN mkdir -p /root/sessions /root/.config/csm /root/.agm/logs

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/agm"]
CMD ["--help"]
```

**Build and run:**
```bash
# Build image
docker build -t agm:latest .

# Run AGM in container
docker run -it --rm \
  -v $HOME/sessions:/root/sessions \
  -v $HOME/.config/csm:/root/.config/csm \
  -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
  agm:latest list

# Interactive session
docker run -it --rm \
  -v $HOME/sessions:/root/sessions \
  -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY \
  agm:latest
```

**Docker Compose:**
```yaml
version: '3.8'

services:
  agm:
    build: .
    image: agm:latest
    volumes:
      - ./sessions:/root/sessions
      - ./config:/root/.config/csm
    environment:
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
      - GEMINI_API_KEY=${GEMINI_API_KEY}
      - OPENAI_API_KEY=${OPENAI_API_KEY}
    stdin_open: true
    tty: true
```

### Kubernetes (Advanced)

**StatefulSet example:**
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: agm
spec:
  serviceName: agm
  replicas: 1
  selector:
    matchLabels:
      app: agm
  template:
    metadata:
      labels:
        app: agm
    spec:
      containers:
      - name: agm
        image: agm:latest
        volumeMounts:
        - name: sessions
          mountPath: /root/sessions
        - name: config
          mountPath: /root/.config/csm
        env:
        - name: ANTHROPIC_API_KEY
          valueFrom:
            secretKeyRef:
              name: agm-secrets
              key: anthropic-key
  volumeClaimTemplates:
  - metadata:
      name: sessions
    spec:
      accessModes: [ "ReadWriteOnce" ]
      resources:
        requests:
          storage: 10Gi
```

**Secret management:**
```bash
# Create secret for API keys
kubectl create secret generic agm-secrets \
  --from-literal=anthropic-key=$ANTHROPIC_API_KEY \
  --from-literal=gemini-key=$GEMINI_API_KEY \
  --from-literal=openai-key=$OPENAI_API_KEY
```

## Configuration Management

### Environment-Specific Configurations

**Development:**
```yaml
# ~/.config/agm/config.yaml
defaults:
  interactive: true
  auto_associate_uuid: true
  confirm_destructive: true

ui:
  theme: "agm"
  fuzzy_search: true

advanced:
  tmux_timeout: "5s"
  health_check_cache: "1s"  # Short cache for development
```

**Production:**
```yaml
# /etc/agm/config.yaml
defaults:
  interactive: false  # Non-interactive for automation
  auto_associate_uuid: true
  confirm_destructive: false  # Skip confirmations

ui:
  theme: "agm"
  fuzzy_search: false  # Exact matching only

advanced:
  tmux_timeout: "10s"  # Longer timeout for stability
  health_check_cache: "5m"  # Longer cache for performance
  lock_timeout: "60s"
```

### Configuration Precedence

AGM loads configuration in this order (later overrides earlier):

1. Default values (hardcoded)
2. System configuration (`/etc/agm/config.yaml`)
3. User configuration (`~/.config/agm/config.yaml`)
4. Environment variables (`CSM_*`)
5. Command-line flags

**Example:**
```bash
# System default: theme = "csm"
# User config: theme = "dracula"
# Environment: CSM_THEME=catppuccin
# CLI flag: --theme=charm
# Result: theme = "charm" (CLI flag wins)
```

### Environment Variables

```bash
# API Keys
export ANTHROPIC_API_KEY="sk-ant-..."
export GEMINI_API_KEY="AIza..."
export OPENAI_API_KEY="sk-..."

# Google Cloud (for semantic search)
export GOOGLE_CLOUD_PROJECT="my-project"

# Configuration overrides
export CSM_DEBUG=true
export CSM_THEME=dracula
export CSM_CONFIG_PATH=/custom/path/config.yaml

# Session storage
export CSM_SESSION_DIR=~/my-sessions
```

## Production Considerations

### Systemd Service (For Automated Sessions)

**Service file (`/etc/systemd/user/agm-session.service`):**
```ini
[Unit]
Description=AGM Session - %i
After=network.target

[Service]
Type=forking
Environment="ANTHROPIC_API_KEY=sk-ant-..."
ExecStart=/usr/local/bin/agm new %i --detached
ExecStop=/usr/local/bin/agm kill %i
Restart=on-failure
RestartSec=10s

[Install]
WantedBy=default.target
```

**Enable and start:**
```bash
# Enable user lingering (sessions persist after logout)
loginctl enable-linger $USER

# Start session service
systemctl --user enable agm-session@my-session
systemctl --user start agm-session@my-session

# Check status
systemctl --user status agm-session@my-session
```

### Monitoring

**Health checks:**
```bash
# Cron job for health monitoring
*/5 * * * * /usr/local/bin/agm doctor --validate --json > /var/log/agm/health.json

# Alert on failures
*/10 * * * * /usr/local/bin/check-agm-health.sh
```

**Metrics collection:**
```bash
# Session statistics
agm list --format=json | jq '.sessions | length'

# Log analytics
agm logs stats --json
```

### Backup Strategy

**Automated backups:**
```bash
# Daily backup script
#!/bin/bash
BACKUP_DIR=/backups/agm/$(date +%Y-%m-%d)
mkdir -p $BACKUP_DIR

# Backup sessions
tar czf $BACKUP_DIR/sessions.tar.gz ~/sessions

# Backup configuration
cp ~/.config/agm/config.yaml $BACKUP_DIR/

# Backup logs
tar czf $BACKUP_DIR/logs.tar.gz ~/.agm/logs

# Cleanup old backups (keep 30 days)
find /backups/agm -type d -mtime +30 -exec rm -rf {} +
```

**Cron schedule:**
```bash
# Daily backups at 2 AM
0 2 * * * /usr/local/bin/backup-agm.sh
```

### Logging

**Log rotation:**
```bash
# /etc/logrotate.d/agm
/home/*/.agm/logs/messages/*.jsonl {
    daily
    rotate 30
    compress
    missingok
    notifempty
    sharedscripts
    postrotate
        systemctl --user reload agm-session@* || true
    endscript
}
```

### Security Hardening

```bash
# Restrict file permissions
find ~/sessions -type d -exec chmod 700 {} \;
find ~/sessions -type f -exec chmod 600 {} \;

# Secure configuration
chmod 700 ~/.config/agm
chmod 600 ~/.config/agm/config.yaml

# Secure logs
chmod 700 ~/.agm/logs
chmod 600 ~/.agm/logs/messages/*.jsonl

# Audit permissions
agm doctor --validate
```

## Troubleshooting

### Installation Issues

**Problem:** `go install` fails
```bash
# Solution: Update Go version
go version  # Check current version
# Install Go 1.24+ from https://go.dev/dl/
```

**Problem:** `tmux` not found
```bash
# Solution: Install tmux
sudo apt-get install tmux  # Ubuntu/Debian
sudo dnf install tmux      # Fedora/RHEL
brew install tmux          # macOS
```

### Deployment Issues

**Problem:** Sessions not persisting after logout
```bash
# Solution: Enable user lingering
loginctl enable-linger $USER
loginctl show-user $USER | grep Linger
# Should show: Linger=yes
```

**Problem:** Permission denied errors
```bash
# Solution: Fix permissions
chmod 700 ~/sessions ~/.agm
agm doctor --validate --fix
```

**Problem:** Container can't access sessions
```bash
# Solution: Mount volumes correctly
docker run -it --rm \
  -v $HOME/sessions:/root/sessions:rw \
  -v $HOME/.config/csm:/root/.config/csm:ro \
  agm:latest list
```

### Performance Optimization

**Problem:** Slow session listing
```bash
# Solution: Enable caching
# In ~/.config/agm/config.yaml:
advanced:
  health_check_cache: "5m"
  discovery_cache: "1m"
```

**Problem:** High memory usage
```bash
# Solution: Configure log rotation
agm logs clean --older-than 30
# Add to cron: daily cleanup
```

## Getting Help

- **Documentation:** [docs/INDEX.md](docs/INDEX.md)
- **Troubleshooting:** [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md)
- **Issues:** https://github.com/vbonnet/dear-agent/issues
- **Health check:** `agm doctor --validate`

---

**Last Updated:** 2026-02-04
**AGM Version:** 3.0
**Maintainer:** Foundation Engineering Team
