# Global MCP Quick Start Guide

Quick reference for using global MCPs with AGM.

## Setup (5 minutes)

### 1. Configure Global MCPs

Create `~/.config/agm/mcp.yaml`:

```bash
mkdir -p ~/.config/agm
cat > ~/.config/agm/mcp.yaml << 'EOF'
mcp_servers:
  - name: googledocs
    url: http://localhost:8001
    type: mcp
EOF
```

### 2. Start Global MCP Server

```bash
cd packages/mcp-http-server

# Build if needed
npm install
npm run build

# Start server
npm run dev -- \
  --port 8001 \
  --mcp-command "npx -y @modelcontextprotocol/server-googledocs"
```

### 3. Verify MCP is Running

```bash
# Check health
curl http://localhost:8001/health

# Expected output:
# {"status":"healthy","uptime":123,"sessions":0,...}

# Or use AGM command
agm mcp-status
```

## Usage

### Check MCP Status

```bash
agm mcp-status

# Output:
# Global MCP Server Status:
#
# NAME           STATUS        URL
# ----           ------        ---
# googledocs     AVAILABLE     http://localhost:8001
#
# Summary: 1/1 global MCPs available
```

### Create Session

```bash
agm session new my-project
# AGM automatically detects and uses the global MCP
```

### Session-Specific MCPs

Override global MCPs for a specific project:

```bash
cd /path/to/my-project
mkdir -p .agm

cat > .agm/mcp.yaml << 'EOF'
mcp_servers:
  - name: local-tools
    command: node
    args: [server.js]
    env:
      DEBUG: "true"
EOF
```

## Common Commands

```bash
# Check all global MCPs
agm mcp-status

# Check with JSON output
agm mcp-status --json

# Manual health check
curl http://localhost:8001/health

# View MCP logs
tail -f ~/.agm/mcp-services/googledocs/mcp-server.log

# Test MCP connection
curl -X POST http://localhost:8001/mcp/sessions \
  -H "Content-Type: application/json" \
  -d '{"clientId":"test"}'
```

## Troubleshooting

### MCP Not Detected

```bash
# 1. Check if server is running
curl http://localhost:8001/health

# 2. Check config
cat ~/.config/agm/mcp.yaml

# 3. Check port binding
netstat -tuln | grep 8001

# 4. Enable debug mode
export AGM_DEBUG=true
agm mcp-status
```

### Connection Timeout

```bash
# Increase timeout
export AGM_MCP_TIMEOUT=10s

# Check network
ping localhost

# Check firewall
sudo iptables -L | grep 8001
```

### Session Hangs

```bash
# Check Claude logs
tail -f ~/.agm/sessions/*/claude.log

# Check MCP logs
tail -f ~/.agm/mcp-services/*/mcp-server.log

# Kill and restart MCP
pkill -f mcp-http-server
npm run dev -- --port 8001 --mcp-command "npx -y @modelcontextprotocol/server-googledocs"
```

## Configuration Reference

### Global Config: `~/.config/agm/mcp.yaml`

```yaml
mcp_servers:
  - name: googledocs        # Unique name
    url: http://localhost:8001  # HTTP endpoint
    type: mcp               # Type (always "mcp")
```

### Session Config: `<project>/.agm/mcp.yaml`

```yaml
mcp_servers:
  - name: local-mcp         # Unique name
    command: node           # Command to run
    args: [server.js]       # Command arguments
    env:                    # Environment variables
      DEBUG: "true"
```

### Environment Variables

```bash
# Alternative to YAML config
export AGM_MCP_SERVERS="googledocs=http://localhost:8001,github=http://localhost:8002"

# Debug mode
export AGM_DEBUG=true

# Timeout
export AGM_MCP_TIMEOUT=10s
```

## Advanced

### Multiple MCPs

```yaml
mcp_servers:
  - name: googledocs
    url: http://localhost:8001
    type: mcp

  - name: github
    url: http://localhost:8002
    type: mcp

  - name: filesystem
    url: http://localhost:8003
    type: mcp
```

### Systemd Service (Linux)

Create `/etc/systemd/system/mcp-googledocs.service`:

```ini
[Unit]
Description=Global MCP - Google Docs
After=network.target

[Service]
Type=simple
User=youruser
WorkingDirectory=/home/youruser/src/ws/oss/swarm/projects/mcp-global-sharing/packages/mcp-http-server
ExecStart=/usr/bin/node dist/server.js --port 8001 --mcp-command "npx -y @modelcontextprotocol/server-googledocs"
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable mcp-googledocs
sudo systemctl start mcp-googledocs
sudo systemctl status mcp-googledocs
```

## Resources

- Full Documentation: `main/agm/internal/mcp/README.md`
- Implementation Details: `main/agm/docs/global-mcp-integration.md`
- Example Config: `main/agm/internal/mcp/example-config.yaml`
- MCP HTTP Server: `packages/mcp-http-server/README.md`

## Quick Reference

| Task | Command |
|------|---------|
| Check MCP status | `agm mcp-status` |
| Create session | `agm session new <name>` |
| Health check | `curl http://localhost:8001/health` |
| View MCP logs | `tail -f ~/.agm/mcp-services/*/mcp-server.log` |
| Debug mode | `export AGM_DEBUG=true` |

## Next Steps

1. ✅ Configure global MCPs
2. ✅ Start MCP servers
3. ✅ Verify with `agm mcp-status`
4. ✅ Create test session
5. ⏭️ Proceed to Task #2: Engram Integration
