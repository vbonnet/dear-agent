# AGM Deployment Guide

Comprehensive guide for deploying AGM (AI/Agent Session Manager) with Temporal workflow orchestration.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Docker Compose Deployment (Local/Development)](#docker-compose-deployment-localdevelopment)
- [Temporal Cloud Deployment (Production)](#temporal-cloud-deployment-production)
- [Post-Deployment Verification](#post-deployment-verification)
- [Troubleshooting](#troubleshooting)

---

## Overview

AGM supports two deployment modes for Temporal workflow orchestration:

1. **Docker Compose (Local/Development)**: Self-hosted Temporal server running in Docker containers alongside AGM. Ideal for development, testing, and small-scale deployments.

2. **Temporal Cloud (Production)**: Managed Temporal Cloud service with high availability, scalability, and enterprise-grade reliability. Recommended for production deployments.

### Deployment Options Comparison

| Feature | Docker Compose | Temporal Cloud |
|---------|---------------|----------------|
| Setup Complexity | Low | Medium |
| Operational Overhead | High (self-managed) | Low (fully managed) |
| Scalability | Limited | High |
| High Availability | No | Yes |
| Cost | Free (infrastructure only) | Paid service |
| Best For | Development, testing | Production, enterprise |

---

## Prerequisites

### Common Prerequisites

- **Operating System**: Linux (Ubuntu 20.04+, Debian 11+) or macOS 11+
- **Go**: 1.24+ (for building from source)
- **tmux**: 2.8+ (required for session management)
- **Git**: For cloning the repository

### Installation

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y tmux git

# macOS
brew install tmux git

# Install Go 1.24+ from https://go.dev/dl/
```

### AGM Installation

```bash
# Install from source
git clone https://github.com/vbonnet/dear-agent.git
cd ai-tools/agm
go build -o agm ./cmd/agm
sudo install -m 755 agm /usr/local/bin/agm

# Verify installation
agm --version
```

### API Keys

Configure API keys for the AI agents you plan to use:

```bash
# Add to ~/.bashrc or ~/.zshrc
export ANTHROPIC_API_KEY="sk-ant-..."      # For Claude
export GEMINI_API_KEY="AIza..."            # For Gemini
export OPENAI_API_KEY="sk-..."             # For GPT

# Reload shell configuration
source ~/.bashrc
```

---

## Docker Compose Deployment (Local/Development)

This deployment runs Temporal server, PostgreSQL database, and Temporal UI in Docker containers on your local machine.

### Prerequisites

- **Docker**: 20.10+ with Docker Compose
- **Docker Compose**: v2.0+ (included with Docker Desktop)

```bash
# Install Docker (Ubuntu/Debian)
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Install Docker (macOS)
# Download Docker Desktop from https://www.docker.com/products/docker-desktop

# Verify installation
docker --version
docker compose version
```

### Step 1: Create Docker Compose Configuration

Create a directory for Temporal deployment:

```bash
mkdir -p ~/.agm/temporal
cd ~/.agm/temporal
```

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  # PostgreSQL database for Temporal
  postgresql:
    image: postgres:16-alpine
    container_name: temporal-postgresql
    environment:
      POSTGRES_USER: temporal
      POSTGRES_PASSWORD: temporal
      POSTGRES_DB: temporal
    ports:
      - "5432:5432"
    volumes:
      - temporal-postgres-data:/var/lib/postgresql/data
    networks:
      - temporal-network
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U temporal"]
      interval: 10s
      timeout: 5s
      retries: 5

  # Temporal server
  temporal:
    image: temporalio/auto-setup:1.28.1
    container_name: temporal-server
    depends_on:
      postgresql:
        condition: service_healthy
    environment:
      - DB=postgresql
      - DB_PORT=5432
      - POSTGRES_USER=temporal
      - POSTGRES_PWD=temporal
      - POSTGRES_SEEDS=postgresql
      - DYNAMIC_CONFIG_FILE_PATH=config/dynamicconfig/development-sql.yaml
    ports:
      - "7233:7233"  # Temporal server gRPC endpoint
    volumes:
      - ./dynamicconfig:/etc/temporal/config/dynamicconfig
    networks:
      - temporal-network
    healthcheck:
      test: ["CMD", "tctl", "--address", "temporal:7233", "cluster", "health"]
      interval: 10s
      timeout: 5s
      retries: 5

  # Temporal Web UI
  temporal-ui:
    image: temporalio/ui:2.35.0
    container_name: temporal-ui
    depends_on:
      temporal:
        condition: service_healthy
    environment:
      - TEMPORAL_ADDRESS=temporal:7233
      - TEMPORAL_CORS_ORIGINS=http://localhost:8088
    ports:
      - "8088:8080"  # Web UI accessible at http://localhost:8088
    networks:
      - temporal-network

networks:
  temporal-network:
    driver: bridge

volumes:
  temporal-postgres-data:
```

### Step 2: Create Dynamic Configuration (Optional)

Create optional dynamic configuration for development:

```bash
mkdir -p ~/.agm/temporal/dynamicconfig
cat > ~/.agm/temporal/dynamicconfig/development-sql.yaml << EOF
# Development configuration for Temporal
# Increases timeouts and enables verbose logging
system.forceSearchAttributesCacheRefreshOnRead:
  - value: true
    constraints: {}

frontend.enableUpdateWorkflowExecution:
  - value: true
    constraints: {}

limit.maxIDLength:
  - value: 255
    constraints: {}
EOF
```

### Step 3: Start Temporal Services

```bash
cd ~/.agm/temporal

# Start all services in detached mode
docker compose up -d

# View logs (optional)
docker compose logs -f

# Wait for services to be healthy (may take 30-60 seconds)
docker compose ps
```

Expected output:
```
NAME                 IMAGE                        STATUS
temporal-postgresql  postgres:16-alpine           Up (healthy)
temporal-server      temporalio/auto-setup:1.28.1 Up (healthy)
temporal-ui          temporalio/ui:2.35.0         Up
```

### Step 4: Verify Temporal Server

```bash
# Check Temporal server is responding
docker exec temporal-server tctl cluster health

# Expected output:
# SERVING
```

### Step 5: Access Temporal UI

Open your browser and navigate to:

```
http://localhost:8088
```

You should see the Temporal Web UI showing the default namespace.

### Step 6: Configure AGM for Local Temporal

```bash
# Set backend to Temporal
export AGM_SESSION_BACKEND=temporal

# Configure Temporal connection (local)
export TEMPORAL_HOST=localhost:7233
export TEMPORAL_NAMESPACE=default

# Add to ~/.bashrc for persistence
cat >> ~/.bashrc << EOF

# AGM Temporal Configuration (Local)
export AGM_SESSION_BACKEND=temporal
export TEMPORAL_HOST=localhost:7233
export TEMPORAL_NAMESPACE=default
EOF

source ~/.bashrc
```

### Step 7: Test AGM with Temporal

```bash
# Create a test session
agm new test-session --harness claude-code

# Verify workflow was created in Temporal UI
# Navigate to http://localhost:8088 and check for "session-test-session" workflow

# List sessions
agm list

# Clean up test session
agm kill test-session
```

### Managing Docker Compose Deployment

```bash
# Stop services (data persists)
docker compose stop

# Start services again
docker compose start

# Stop and remove containers (data persists in volumes)
docker compose down

# Stop and remove everything including volumes (CAUTION: deletes all data)
docker compose down -v

# View service logs
docker compose logs temporal
docker compose logs postgresql

# Restart a single service
docker compose restart temporal
```

### Upgrading Temporal Version

```bash
cd ~/.agm/temporal

# Pull latest images
docker compose pull

# Recreate containers with new images
docker compose up -d --force-recreate
```

---

## Temporal Cloud Deployment (Production)

Temporal Cloud provides a fully managed, production-ready Temporal service with high availability and scalability.

### Prerequisites

- Temporal Cloud account (sign up at https://temporal.io/cloud)
- OpenSSL or similar tool for certificate generation

### Step 1: Create Temporal Cloud Account

1. Visit https://temporal.io/cloud
2. Click "Sign Up" or "Start Free Trial"
3. Complete registration and email verification
4. Log in to Temporal Cloud console

### Step 2: Create Namespace

A namespace is an isolated environment for your workflows.

1. In Temporal Cloud console, click "Create Namespace"
2. Enter namespace details:
   - **Namespace Name**: `agm-production` (or your preferred name)
   - **Region**: Select region closest to your deployment
   - **Retention Period**: 7 days (adjust based on needs)
3. Click "Create Namespace"
4. Note the namespace endpoint (e.g., `agm-production.a2dd6.tmprl.cloud:7233`)

### Step 3: Generate mTLS Certificates

Temporal Cloud requires mutual TLS (mTLS) for authentication.

#### Option A: Generate Self-Signed Certificates (Development/Testing)

```bash
# Create directory for certificates
mkdir -p ~/.agm/temporal/certs
cd ~/.agm/temporal/certs

# Generate CA private key
openssl genrsa -out ca.key 2048

# Generate CA certificate
openssl req -new -x509 -key ca.key -out ca.pem -days 365 \
  -subj "/C=US/ST=State/L=City/O=Organization/CN=AGM CA"

# Generate client private key
openssl genrsa -out client.key 2048

# Generate client certificate signing request (CSR)
openssl req -new -key client.key -out client.csr \
  -subj "/C=US/ST=State/L=City/O=Organization/CN=agm-client"

# Sign client certificate with CA
openssl x509 -req -in client.csr -CA ca.pem -CAkey ca.key \
  -CAcreateserial -out client.pem -days 365

# Set proper permissions
chmod 600 client.key ca.key
chmod 644 client.pem ca.pem

# Clean up CSR
rm client.csr
```

#### Option B: Use Temporal Cloud Certificate Authority (Recommended)

1. In Temporal Cloud console, go to your namespace
2. Click "Settings" → "Certificates"
3. Click "Generate Certificate"
4. Download the generated certificate bundle
5. Extract to `~/.agm/temporal/certs/`:
   - `client.pem` - Client certificate
   - `client.key` - Client private key
   - `ca.pem` - CA certificate (optional, Temporal Cloud provides this)

### Step 4: Upload CA Certificate to Temporal Cloud

1. In Temporal Cloud console, go to namespace settings
2. Click "Settings" → "Certificates" → "Add Certificate"
3. Upload your `ca.pem` file
4. Click "Save"

### Step 5: Create API Key (Alternative to mTLS)

Some deployments may use API keys instead of mTLS:

1. In Temporal Cloud console, go to "Settings" → "API Keys"
2. Click "Create API Key"
3. Enter description: "AGM Production"
4. Copy and securely store the API key
5. Set environment variable:

```bash
export TEMPORAL_API_KEY="your-api-key-here"
```

### Step 6: Configure AGM for Temporal Cloud

```bash
# Set backend to Temporal
export AGM_SESSION_BACKEND=temporal

# Configure Temporal Cloud connection
export TEMPORAL_HOST=agm-production.a2dd6.tmprl.cloud:7233
export TEMPORAL_NAMESPACE=agm-production

# Configure mTLS certificates
export TEMPORAL_TLS_CERT=~/.agm/temporal/certs/client.pem
export TEMPORAL_TLS_KEY=~/.agm/temporal/certs/client.key
export TEMPORAL_TLS_CA=~/.agm/temporal/certs/ca.pem  # Optional

# Alternative: Use API key instead of mTLS
# export TEMPORAL_API_KEY=your-api-key-here

# Add to environment configuration
cat >> ~/.bashrc << EOF

# AGM Temporal Cloud Configuration (Production)
export AGM_SESSION_BACKEND=temporal
export TEMPORAL_HOST=agm-production.a2dd6.tmprl.cloud:7233
export TEMPORAL_NAMESPACE=agm-production
export TEMPORAL_TLS_CERT=~/.agm/temporal/certs/client.pem
export TEMPORAL_TLS_KEY=~/.agm/temporal/certs/client.key
export TEMPORAL_TLS_CA=~/.agm/temporal/certs/ca.pem
EOF

source ~/.bashrc
```

### Step 7: Verify Connection

```bash
# Test connection (requires tctl CLI)
tctl --namespace agm-production cluster health

# Or test with AGM directly
agm list
```

### Security Best Practices

1. **Protect Certificate Files**:
   ```bash
   chmod 600 ~/.agm/temporal/certs/client.key
   chmod 644 ~/.agm/temporal/certs/client.pem
   ```

2. **Use Secret Management**:
   For production, consider using secret management tools:
   - AWS Secrets Manager
   - HashiCorp Vault
   - Azure Key Vault
   - Kubernetes Secrets

3. **Rotate Certificates Regularly**:
   Set calendar reminders to rotate certificates before expiration (typically every 90-365 days).

4. **Restrict Network Access**:
   If possible, restrict Temporal Cloud access to specific IP ranges or VPN.

### Connection String Format

The complete Temporal Cloud connection string format:

```bash
# gRPC endpoint format
<namespace>.<account-id>.tmprl.cloud:7233

# Example
agm-production.a2dd6.tmprl.cloud:7233
```

### Certificate Paths and Permissions

Required certificate files:

```
~/.agm/temporal/certs/
├── client.pem      # Client certificate (chmod 644)
├── client.key      # Client private key (chmod 600)
└── ca.pem          # CA certificate (chmod 644, optional)
```

Verify permissions:

```bash
ls -la ~/.agm/temporal/certs/

# Expected output:
# -rw-r--r-- client.pem
# -rw------- client.key
# -rw-r--r-- ca.pem
```

---

## Post-Deployment Verification

After deploying Temporal (either locally or on Temporal Cloud), verify your AGM setup.

### Step 1: Verify Temporal Connection

```bash
# Check AGM can connect to Temporal
agm doctor --validate

# Expected output should show Temporal backend status
```

### Step 2: Test Workflow Execution

Create a test session to verify workflows are executing:

```bash
# Create test session
agm new test-workflow-session --harness claude-code --project /tmp/test

# Verify session was created
agm list

# Check workflow in Temporal UI
# - Docker Compose: http://localhost:8088
# - Temporal Cloud: https://cloud.temporal.io
```

### Step 3: Check Temporal UI

#### For Docker Compose:

1. Open http://localhost:8088
2. Navigate to "Workflows"
3. Look for workflow ID: `session-test-workflow-session`
4. Verify workflow state is "Running"

#### For Temporal Cloud:

1. Log in to https://cloud.temporal.io
2. Select your namespace (e.g., `agm-production`)
3. Navigate to "Workflows"
4. Look for workflow ID: `session-test-workflow-session`
5. Verify workflow state is "Running"

### Step 4: Verify Database Connectivity

AGM uses SQLite for session persistence:

```bash
# Check database file exists
ls -la ~/sessions/*.db

# Expected: ~/sessions/agm.db

# Query database (requires sqlite3 CLI)
sqlite3 ~/sessions/agm.db "SELECT session_id, name, agent FROM sessions;"
```

### Step 5: Execute Sample Workflow

Test complete workflow lifecycle:

```bash
# 1. Create session
agm new sample-session --harness claude-code --project ~/projects/test

# 2. Verify session is active
agm list

# 3. Attach to session (in tmux)
agm attach sample-session

# 4. Detach from session (Ctrl+B, then D)

# 5. Check session is still running
agm list

# 6. Stop session
agm stop sample-session

# 7. Verify session is stopped
agm list

# 8. Archive session
agm archive sample-session

# 9. Verify session is archived
agm list --archived

# 10. Clean up
agm delete sample-session --force
```

### Step 6: Monitor Workflow Metrics

#### For Docker Compose:

```bash
# View Temporal server logs
docker logs temporal-server

# Check PostgreSQL connections
docker exec temporal-postgresql psql -U temporal -c "SELECT count(*) FROM pg_stat_activity;"
```

#### For Temporal Cloud:

1. In Temporal Cloud console, navigate to "Metrics"
2. Monitor:
   - Workflow execution rate
   - Activity execution rate
   - Task queue backlog
   - Workflow errors

### Step 7: Verify Health Check Cache

AGM caches health checks for performance:

```bash
# Run health check
agm doctor --validate

# Check logs for cache hits
cat ~/.agm/logs/agm.log | grep "health check cache"
```

### Verification Checklist

- [ ] AGM connects to Temporal successfully
- [ ] Workflows appear in Temporal UI
- [ ] Session creation works
- [ ] Session listing works
- [ ] Session attachment works
- [ ] Database queries work
- [ ] No error logs in AGM
- [ ] No error logs in Temporal (if Docker Compose)
- [ ] Certificates are properly secured (if Temporal Cloud)
- [ ] API keys are configured
- [ ] Health checks pass

---

## Troubleshooting

### Common Issues and Solutions

#### 1. Temporal Server Not Responding (Docker Compose)

**Symptom:**
```
Error: failed to dial temporal server: connection refused
```

**Solution:**

```bash
# Check if containers are running
docker compose ps

# If not running, start them
cd ~/.agm/temporal
docker compose up -d

# Check container logs
docker compose logs temporal

# Verify health
docker exec temporal-server tctl cluster health
```

#### 2. Connection Refused (Temporal Cloud)

**Symptom:**
```
Error: failed to dial temporal server: connection refused
rpc error: code = Unavailable
```

**Solution:**

```bash
# Check environment variables
echo $TEMPORAL_HOST
echo $TEMPORAL_NAMESPACE

# Verify certificate files exist
ls -la ~/.agm/temporal/certs/

# Test network connectivity
curl -v https://$TEMPORAL_HOST

# Check certificate permissions
chmod 600 ~/.agm/temporal/certs/client.key
chmod 644 ~/.agm/temporal/certs/client.pem
```

#### 3. Certificate Problems (Temporal Cloud)

**Symptom:**
```
Error: tls: failed to verify certificate
rpc error: code = Unavailable desc = connection error
```

**Solution:**

```bash
# Verify certificate format
openssl x509 -in ~/.agm/temporal/certs/client.pem -text -noout

# Verify key matches certificate
openssl x509 -in ~/.agm/temporal/certs/client.pem -noout -modulus | md5sum
openssl rsa -in ~/.agm/temporal/certs/client.key -noout -modulus | md5sum
# Output should match

# Regenerate certificates if needed (see Step 3 in Temporal Cloud section)

# Verify CA certificate is uploaded to Temporal Cloud
# Check in Temporal Cloud console → Settings → Certificates
```

#### 4. Database Connection Errors

**Symptom:**
```
Error: unable to open database file
Error: database is locked
```

**Solution:**

```bash
# Check database file permissions
ls -la ~/sessions/agm.db
chmod 644 ~/sessions/agm.db

# Check for locked database
lsof ~/sessions/agm.db

# If locked, kill holding process
# (Use caution)

# Verify database integrity
sqlite3 ~/sessions/agm.db "PRAGMA integrity_check;"
# Expected: ok
```

#### 5. Port Conflicts (Docker Compose)

**Symptom:**
```
Error: bind: address already in use
```

**Solution:**

```bash
# Find process using port 7233 (Temporal) or 8088 (UI)
lsof -i :7233
lsof -i :8088

# Stop conflicting process or change ports in docker-compose.yml

# Alternative: Use different ports in docker-compose.yml
# Change:
#   ports:
#     - "17233:7233"  # Temporal gRPC
#     - "18088:8080"  # Temporal UI
```

#### 6. PostgreSQL Connection Failed (Docker Compose)

**Symptom:**
```
Error: failed to connect to database
pq: password authentication failed
```

**Solution:**

```bash
# Check PostgreSQL container
docker logs temporal-postgresql

# Verify environment variables match
docker compose config | grep POSTGRES

# Reset database (CAUTION: deletes all data)
docker compose down -v
docker compose up -d
```

#### 7. Workflow Not Appearing in UI

**Symptom:**
Workflows created but not visible in Temporal UI

**Solution:**

```bash
# 1. Verify workflow was created
agm list

# 2. Check workflow ID format
# Should be: session-<session-name>

# 3. In Temporal UI, check correct namespace
# Docker Compose: "default"
# Temporal Cloud: Your namespace (e.g., "agm-production")

# 4. Check workflow filters in UI
# Remove any filters that might hide workflows

# 5. Query via tctl
tctl workflow list --namespace default
```

#### 8. Backend Not Switching to Temporal

**Symptom:**
```
Still using tmux backend despite setting AGM_SESSION_BACKEND=temporal
```

**Solution:**

```bash
# Verify environment variable is set
echo $AGM_SESSION_BACKEND

# If empty, set and export
export AGM_SESSION_BACKEND=temporal

# Add to shell profile
echo 'export AGM_SESSION_BACKEND=temporal' >> ~/.bashrc
source ~/.bashrc

# Verify with AGM
agm doctor | grep -i backend
```

#### 9. High Memory Usage (Docker Compose)

**Symptom:**
Temporal containers consuming excessive memory

**Solution:**

```bash
# Check current memory usage
docker stats

# Limit container memory in docker-compose.yml
services:
  temporal:
    deploy:
      resources:
        limits:
          memory: 1G
  postgresql:
    deploy:
      resources:
        limits:
          memory: 512M

# Restart with new limits
docker compose down
docker compose up -d
```

#### 10. Temporal Cloud Rate Limiting

**Symptom:**
```
Error: RESOURCE_EXHAUSTED: rate limit exceeded
```

**Solution:**

```bash
# Check Temporal Cloud usage limits
# Visit: https://cloud.temporal.io → Settings → Usage

# Reduce workflow creation rate
# Add delays between operations

# Upgrade Temporal Cloud plan if consistently hitting limits

# Contact Temporal support for rate limit increase
```

### Debug Mode

Enable debug mode for detailed logging:

```bash
# Set debug environment variable
export AGM_DEBUG=true

# Run AGM command
agm list

# Check debug logs
cat ~/.agm/logs/agm.log
```

### Getting Help

If issues persist:

1. **Check documentation**:
   - AGM: `main/agm/docs/`
   - Temporal: https://docs.temporal.io

2. **Run health check**:
   ```bash
   agm doctor --validate --verbose
   ```

3. **Check logs**:
   ```bash
   # AGM logs
   tail -f ~/.agm/logs/agm.log

   # Temporal logs (Docker Compose)
   docker compose logs -f temporal

   # PostgreSQL logs (Docker Compose)
   docker compose logs -f postgresql
   ```

4. **Report issues**:
   - GitHub: https://github.com/vbonnet/dear-agent/issues
   - Include output from `agm doctor --validate`
   - Include relevant log snippets

---

## Next Steps

After successful deployment:

1. **Configure Production Settings**:
   - Set up log rotation
   - Configure monitoring and alerting
   - Implement backup strategy
   - See: `main/agm/docs/AGM-DEPLOYMENT.md`

2. **Set Up Monitoring**:
   - Monitor Temporal workflows
   - Track AGM session metrics
   - Set up health check alerts

3. **Security Hardening**:
   - Review certificate security
   - Implement secret rotation
   - Configure network policies
   - See: `main/agm/docs/AGM-SECURITY.md`

4. **User Training**:
   - Train team on AGM commands
   - Document workflows and processes
   - Create runbooks for common operations

---

**Last Updated:** 2026-02-15
**AGM Version:** 3.0
**Temporal SDK Version:** 1.28.1
**Maintainer:** Foundation Engineering Team
