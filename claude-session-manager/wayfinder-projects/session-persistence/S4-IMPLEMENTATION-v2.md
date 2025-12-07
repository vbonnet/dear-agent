# S4: Implementation - Phase 3.5 Session Persistence (v2)

**Date**: December 7, 2025
**Status**: 🔄 IN REVIEW - Round 2
**Version**: 2.0
**Phase Goal**: Implement all 11 deliverables from S1, S2, and S3 sprint plans
**Prerequisites**:
- ✅ All planning phases approved (D2-D4, S1-S3, avg 9.47/10)
- ✅ Round 1 review complete (8.17/10)
- ✅ Addressing all feedback for v2

**Changes from v1**:
- Added Prerequisites section (Go 1.21+, tmux, dependencies)
- Added Development Workflow section
- Added CI/CD Configuration section
- Added Configuration Management section
- Added Test Fixtures & Examples section
- Fixed migration concurrency (lock before migrate)
- Fixed migration idempotency (.v1.bak check)
- Added TmuxInterface for mockability
- Added test execution commands
- Added deployment verification
- Clarified doctor fix confirmation strategy

---

## Prerequisites

### System Requirements

**Go Environment**:
- Go 1.21 or later
- Go modules enabled (`GO111MODULE=on`)
- Standard Go toolchain (`go`, `gofmt`, `go vet`)

**External Dependencies**:
- `tmux` 3.0+ installed and in PATH
- `git` for version control
- POSIX-compliant filesystem (macOS, Linux)

**Verification**:
```bash
# Check Go version
go version  # Should show go1.21 or later

# Check tmux
tmux -V  # Should show tmux 3.0 or later

# Check filesystem (sessions directory)
ls -la ~/sessions  # Should exist from current CSM
```

### Project Setup

**Initialize Go Module** (if not already done):
```bash
cd ~/src/repos/ai-tools/base/claude-session-manager
go mod init github.com/yourusername/claude-session-manager  # If needed
go mod tidy
```

**External Dependencies**:
```bash
# Add required dependencies
go get gopkg.in/yaml.v3
go mod tidy
```

**Expected `go.mod`**:
```go
module github.com/yourusername/claude-session-manager

go 1.21

require (
    gopkg.in/yaml.v3 v3.0.1
)
```

### Directory Structure Preparation

**Verify existing structure**:
```bash
# Current structure
~/src/repos/ai-tools/base/claude-session-manager/
├── cmd/
│   └── csm/
│       └── main.go  # Existing
├── internal/  # Will create packages here
└── go.mod
```

**Create new directories** (will be done during implementation):
```bash
mkdir -p internal/manifest
mkdir -p internal/fileutil
mkdir -p internal/logging
mkdir -p internal/session
mkdir -p cmd/csm/testdata/{manifests,history,worktrees}
```

---

## Development Workflow

### Local Development Setup

**1. Clone and setup** (if starting fresh):
```bash
cd ~/src/repos/ai-tools/base
git clone <repo-url> claude-session-manager
cd claude-session-manager
go mod download
```

**2. Run tests**:
```bash
# Run all tests
go test ./...

# Run only fast tests (for quick feedback)
go test ./... -short -timeout=2m

# Run specific package tests
go test ./internal/manifest -v

# Run with coverage
go test ./... -cover
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out  # View in browser
```

**3. Build binary**:
```bash
# Build for current platform
go build -o csm ./cmd/csm

# Build with version info
go build -ldflags="-X main.Version=3.5.0" -o csm ./cmd/csm

# Test binary
./csm list
./csm --help
```

**4. Run locally without installing**:
```bash
# Run directly
go run ./cmd/csm list
go run ./cmd/csm resume claude-1

# With arguments
go run ./cmd/csm doctor --fix
```

### Debugging

**Using delve**:
```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug specific command
dlv debug ./cmd/csm -- resume claude-1

# Debug tests
dlv test ./internal/manifest
```

**Logging**:
```go
// Add debug logging during development
import "log"

func Load(path string) (*Manifest, error) {
    log.Printf("DEBUG: Loading manifest from %s", path)
    // ...
}
```

**Testing individual functions**:
```bash
# Run specific test
go test ./internal/manifest -run TestMigrateV1ToV2 -v

# Run with print debugging
go test ./internal/manifest -v 2>&1 | grep "DEBUG"
```

### Code Quality Checks

**Before committing**:
```bash
# Format code
gofmt -w .

# Vet code
go vet ./...

# Run tests
go test ./... -short

# Check coverage
go test ./... -cover | grep -E "ok|coverage"
```

---

## CI/CD Configuration

### GitHub Actions Workflow

**File**: `.github/workflows/test.yml`

```yaml
name: Test

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest

    steps:
    - name: Checkout code
      uses: actions/checkout@v3

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'

    - name: Install tmux
      run: sudo apt-get update && sudo apt-get install -y tmux

    - name: Download dependencies
      run: go mod download

    - name: Run fast tests
      run: go test ./... -short -timeout=2m -v

    - name: Run vet
      run: go vet ./...

    - name: Check formatting
      run: |
        if [ -n "$(gofmt -l .)" ]; then
          echo "Go code is not formatted:"
          gofmt -d .
          exit 1
        fi

    - name: Build
      run: go build -v ./cmd/csm

  test-slow:
    runs-on: ubuntu-latest
    # Only run on main branch (nightly equivalent)
    if: github.ref == 'refs/heads/main'

    steps:
    - name: Checkout code
      uses: actions/checkout@v3

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'

    - name: Install tmux
      run: sudo apt-get update && sudo apt-get install -y tmux

    - name: Download dependencies
      run: go mod download

    - name: Run all tests (including slow)
      run: go test ./... -timeout=30m -v

    - name: Run benchmarks
      run: go test ./cmd/csm -bench=. -benchtime=10s
```

### Makefile

**File**: `Makefile`

```makefile
.PHONY: all build test test-short test-coverage clean install lint

# Build variables
BINARY_NAME=csm
VERSION?=3.5.0
LDFLAGS=-ldflags="-X main.Version=$(VERSION)"

all: test build

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/csm

install: build
	cp $(BINARY_NAME) ~/bin/csm
	@echo "Installed to ~/bin/csm"

test:
	go test ./... -v

test-short:
	go test ./... -short -timeout=2m -v

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint:
	gofmt -w .
	go vet ./...

clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	find . -name "*.test" -delete

.PHONY: smoke-test
smoke-test: build
	@echo "Running smoke tests..."
	./$(BINARY_NAME) list
	@echo "Smoke tests passed!"
```

**Usage**:
```bash
# Build
make build

# Run fast tests
make test-short

# Install locally
make install

# Generate coverage report
make test-coverage

# Lint and format
make lint

# Clean build artifacts
make clean
```

### Deployment Verification

**Post-deployment smoke test**:

**File**: `scripts/verify-deployment.sh`

```bash
#!/bin/bash
# Verify CSM deployment

set -e

echo "=== CSM Deployment Verification ==="

# Check binary exists
if ! command -v csm &> /dev/null; then
    echo "ERROR: csm not found in PATH"
    exit 1
fi

# Check version
VERSION=$(csm --version 2>&1 || echo "unknown")
echo "✓ CSM version: $VERSION"

# Check list command
echo "✓ Testing 'csm list'..."
csm list > /dev/null

# Check doctor command
echo "✓ Testing 'csm doctor'..."
csm doctor --quiet > /dev/null

# Check sessions directory
if [ ! -d "$HOME/sessions" ]; then
    echo "ERROR: ~/sessions directory missing"
    exit 1
fi
echo "✓ Sessions directory exists"

# Check logs directory
if [ ! -d "$HOME/.csm/logs" ]; then
    echo "WARN: Logs directory will be created on first migration"
fi

echo "=== Deployment verification PASSED ==="
```

**Run after deployment**:
```bash
chmod +x scripts/verify-deployment.sh
./scripts/verify-deployment.sh
```

---

## Configuration Management

### Configuration Strategy

**Configuration File**: `~/.csmrc` (optional, uses defaults if missing)

**Format**: YAML

**Example `~/.csmrc`**:
```yaml
# Claude Session Manager Configuration

# Claude installation paths
claude:
  history_path: "~/.claude/history.jsonl"
  session_env_dir: "~/.claude/session-env"
  file_history_dir: "~/.claude/file-history"

# CSM paths
csm:
  sessions_dir: "~/sessions"
  logs_dir: "~/.csm/logs"

# Timeouts and limits
timeouts:
  lock_timeout: 60  # seconds
  max_backups_per_session: 10

# Log rotation
logging:
  rotation_size_mb: 10
  rotation_keep_count: 5

# Performance
performance:
  batch_status_check: true
```

### Configuration Loading

**File**: `internal/config/config.go`

```go
package config

import (
    "os"
    "path/filepath"
    "gopkg.in/yaml.v3"
)

type Config struct {
    Claude struct {
        HistoryPath    string `yaml:"history_path"`
        SessionEnvDir  string `yaml:"session_env_dir"`
        FileHistoryDir string `yaml:"file_history_dir"`
    } `yaml:"claude"`

    CSM struct {
        SessionsDir string `yaml:"sessions_dir"`
        LogsDir     string `yaml:"logs_dir"`
    } `yaml:"csm"`

    Timeouts struct {
        LockTimeout          int `yaml:"lock_timeout"`
        MaxBackupsPerSession int `yaml:"max_backups_per_session"`
    } `yaml:"timeouts"`

    Logging struct {
        RotationSizeMB    int `yaml:"rotation_size_mb"`
        RotationKeepCount int `yaml:"rotation_keep_count"`
    } `yaml:"logging"`

    Performance struct {
        BatchStatusCheck bool `yaml:"batch_status_check"`
    } `yaml:"performance"`
}

func Load() (*Config, error) {
    c := &Config{}

    // Set defaults
    c.setDefaults()

    // Try to load config file
    configPath := filepath.Join(os.Getenv("HOME"), ".csmrc")
    if data, err := os.ReadFile(configPath); err == nil {
        if err := yaml.Unmarshal(data, c); err != nil {
            return nil, err
        }
    }

    // Expand tildes in paths
    c.expandPaths()

    return c, nil
}

func (c *Config) setDefaults() {
    home := os.Getenv("HOME")

    c.Claude.HistoryPath = filepath.Join(home, ".claude", "history.jsonl")
    c.Claude.SessionEnvDir = filepath.Join(home, ".claude", "session-env")
    c.Claude.FileHistoryDir = filepath.Join(home, ".claude", "file-history")

    c.CSM.SessionsDir = filepath.Join(home, "sessions")
    c.CSM.LogsDir = filepath.Join(home, ".csm", "logs")

    c.Timeouts.LockTimeout = 60
    c.Timeouts.MaxBackupsPerSession = 10

    c.Logging.RotationSizeMB = 10
    c.Logging.RotationKeepCount = 5

    c.Performance.BatchStatusCheck = true
}

func (c *Config) expandPaths() {
    home := os.Getenv("HOME")

    c.Claude.HistoryPath = expandTilde(c.Claude.HistoryPath, home)
    c.Claude.SessionEnvDir = expandTilde(c.Claude.SessionEnvDir, home)
    c.Claude.FileHistoryDir = expandTilde(c.Claude.FileHistoryDir, home)

    c.CSM.SessionsDir = expandTilde(c.CSM.SessionsDir, home)
    c.CSM.LogsDir = expandTilde(c.CSM.LogsDir, home)
}

func expandTilde(path string, home string) string {
    if len(path) > 0 && path[0] == '~' {
        return filepath.Join(home, path[1:])
    }
    return path
}

// Global config instance
var Global *Config

func init() {
    var err error
    Global, err = Load()
    if err != nil {
        // Use defaults if config load fails
        Global = &Config{}
        Global.setDefaults()
        Global.expandPaths()
    }
}
```

**Usage in code**:
```go
import "github.com/yourusername/claude-session-manager/internal/config"

func extractConversation(sessionUUID string) ([]HistoryEntry, error) {
    historyPath := config.Global.Claude.HistoryPath
    // ...
}
```

---

## Test Fixtures & Examples

### Test Data Structure

**Directory**: `cmd/csm/testdata/`

```
cmd/csm/testdata/
├── manifests/
│   ├── v1-simple.yaml          # Basic v1 manifest
│   ├── v1-complete.yaml        # All v1 fields populated
│   ├── v2-simple.yaml          # Basic v2 manifest
│   ├── v2-complete.yaml        # All v2 fields populated
│   ├── v2-archived.yaml        # Archived session
│   ├── invalid-yaml.yaml       # Malformed YAML (missing colon)
│   └── empty.yaml              # Empty file
├── history/
│   ├── simple.jsonl            # 10 messages
│   ├── medium.jsonl            # 200 messages
│   ├── large.jsonl             # 1000+ messages
│   └── malformed.jsonl         # Mix of valid/invalid JSON
└── worktrees/
    └── sample-project/
        └── README.md
```

### Sample Manifest Files

**v1-simple.yaml**:
```yaml
session_id: c4eb298c-8c89-4f75-8dae-c725a1291add
name: claude-test
created_at: 2025-12-01T10:00:00-08:00
context:
  project: /home/user/projects/test-app
tmux:
  session_name: claude-test
```

**v2-complete.yaml**:
```yaml
schema_version: "2.0"
session_id: e6121188-1234-5678-9abc-def012345678
name: claude-myapp
created_at: 2025-12-01T10:00:00-08:00
updated_at: 2025-12-07T14:30:00-08:00
lifecycle: ""
context:
  project: /home/user/projects/myapp
  purpose: "Implementing user authentication with JWT"
  tags:
    - auth
    - backend
    - security
  notes: "Using bcrypt for password hashing. Need to add refresh token rotation."
tmux:
  session_name: claude-myapp
```

**v2-archived.yaml**:
```yaml
schema_version: "2.0"
session_id: a1b2c3d4-5678-90ab-cdef-1234567890ab
name: claude-old
created_at: 2025-11-01T10:00:00-08:00
updated_at: 2025-11-15T16:00:00-08:00
lifecycle: "archived"
context:
  project: /home/user/old-projects/deprecated
  purpose: "Old project that's no longer active"
  tags:
    - deprecated
tmux:
  session_name: claude-old
```

### Sample History Files

**simple.jsonl** (10 messages):
```jsonl
{"session_id":"c4eb298c-8c89-4f75-8dae-c725a1291add","display":"help me implement JWT auth","timestamp":1701450000000,"project":"/home/user/projects/test-app"}
{"session_id":"c4eb298c-8c89-4f75-8dae-c725a1291add","display":"I'll help you implement JWT authentication...","timestamp":1701450010000,"project":"/home/user/projects/test-app"}
{"session_id":"c4eb298c-8c89-4f75-8dae-c725a1291add","display":"show me the code","timestamp":1701450020000,"project":"/home/user/projects/test-app"}
...
```

**malformed.jsonl** (mix of valid/invalid):
```jsonl
{"session_id":"test-uuid","display":"valid message 1","timestamp":1701450000000}
this is not valid JSON
{"session_id":"test-uuid","display":"valid message 2","timestamp":1701450010000}
{"session_id":"test-uuid","display":"missing quote,"timestamp":1701450020000}
{"session_id":"test-uuid","display":"valid message 3","timestamp":1701450030000}
```

### Mock Tmux Implementation

**File**: `cmd/csm/tmux_interface.go`

```go
package main

import (
    "fmt"
    "os/exec"
)

// TmuxInterface allows mocking tmux for tests
type TmuxInterface interface {
    HasSession(name string) (bool, error)
    ListSessions() ([]string, error)
    CreateSession(name, dir string) error
    SendKeys(name, keys string) error
    AttachSession(name string) error
}

// RealTmux uses actual tmux commands
type RealTmux struct{}

func (t *RealTmux) HasSession(name string) (bool, error) {
    cmd := exec.Command("tmux", "has-session", "-t", name)
    err := cmd.Run()
    if err != nil {
        // Exit code 1 = session doesn't exist
        return false, nil
    }
    return true, nil
}

func (t *RealTmux) ListSessions() ([]string, error) {
    cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
    output, err := cmd.Output()
    if err != nil {
        return nil, nil  // No sessions
    }

    sessions := strings.Split(strings.TrimSpace(string(output)), "\n")
    return sessions, nil
}

func (t *RealTmux) CreateSession(name, dir string) error {
    cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", dir)
    return cmd.Run()
}

func (t *RealTmux) SendKeys(name, keys string) error {
    cmd := exec.Command("tmux", "send-keys", "-t", name, keys, "Enter")
    return cmd.Run()
}

func (t *RealTmux) AttachSession(name string) error {
    cmd := exec.Command("tmux", "attach-session", "-t", name)
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}

// MockTmux for testing (no real tmux needed)
type MockTmux struct {
    Sessions map[string]bool
}

func NewMockTmux() *MockTmux {
    return &MockTmux{
        Sessions: make(map[string]bool),
    }
}

func (m *MockTmux) HasSession(name string) (bool, error) {
    return m.Sessions[name], nil
}

func (m *MockTmux) ListSessions() ([]string, error) {
    var sessions []string
    for name := range m.Sessions {
        sessions = append(sessions, name)
    }
    return sessions, nil
}

func (m *MockTmux) CreateSession(name, dir string) error {
    m.Sessions[name] = true
    return nil
}

func (m *MockTmux) SendKeys(name, keys string) error {
    if !m.Sessions[name] {
        return fmt.Errorf("session %s doesn't exist", name)
    }
    return nil
}

func (m *MockTmux) AttachSession(name string) error {
    if !m.Sessions[name] {
        return fmt.Errorf("session %s doesn't exist", name)
    }
    // In tests, just verify session exists
    return nil
}

// Global tmux interface (can be swapped for testing)
var tmux TmuxInterface = &RealTmux{}
```

**Usage in tests**:
```go
func TestResumeWithMock(t *testing.T) {
    // Save original
    originalTmux := tmux
    defer func() { tmux = originalTmux }()

    // Use mock
    mockTmux := NewMockTmux()
    tmux = mockTmux

    // Test
    mockTmux.Sessions["claude-test"] = true

    // ... run test ...
}
```

### Test Execution Commands

**Unit tests**:
```bash
# Run all unit tests
go test ./internal/manifest -v

# Run specific test
go test ./internal/manifest -run TestLoad -v

# Run with coverage
go test ./internal/manifest -cover
```

**Integration tests**:
```bash
# Run integration tests (includes slow tests)
go test ./cmd/csm -v

# Skip slow tests (for CI)
go test ./cmd/csm -short -v

# Run specific integration test
go test ./cmd/csm -run TestFullLifecycle -v
```

**Benchmarks**:
```bash
# Run all benchmarks
go test ./cmd/csm -bench=. -benchtime=10s

# Run specific benchmark
go test ./cmd/csm -bench=BenchmarkResumeAutoRecreation -benchtime=10s

# With memory profiling
go test ./cmd/csm -bench=. -benchmem
```

**Coverage report**:
```bash
# Generate coverage
go test ./... -coverprofile=coverage.out

# View as HTML
go tool cover -html=coverage.out

# Check coverage percentage
go test ./... -cover | grep coverage
```

---

## Sprint 1: Foundation & Core Infrastructure

(Content from v1, with fixes for migration concurrency and idempotency)

### D1.4: Migration v1 → v2 (8 hours) - UPDATED

**Files to Create**:
```
internal/manifest/
├── migrate.go       # Migration logic
└── migrate_test.go  # Tests
```

**Implementation Tasks**:

1. **Version detection** (same as v1)

2. **Migration with concurrency handling** (UPDATED):
   ```go
   func MigrateV1ToV2(path string) error {
       // CRITICAL: Acquire lock BEFORE migration
       // Prevents race condition if two processes load same v1 manifest
       if err := AcquireLock(path); err != nil {
           return fmt.Errorf("cannot acquire lock for migration: %w", err)
       }
       defer ReleaseLock(path)

       // Read v1 manifest
       data, err := os.ReadFile(path)
       if err != nil {
           return err
       }

       var v1 ManifestV1
       if err := yaml.Unmarshal(data, &v1); err != nil {
           return err
       }

       // Check if backup already exists (idempotency)
       backupPath := path + ".v1.bak"
       if _, err := os.Stat(backupPath); err == nil {
           // Backup exists, migration already done
           // This can happen if migration succeeded but save failed
           logMigration("SKIPPED", path, errors.New("backup already exists"))
           return nil
       }

       // Backup original (atomic)
       if err := fileutil.AtomicWrite(backupPath, data, 0600); err != nil {
           return fmt.Errorf("failed to create backup: %w", err)
       }

       // Convert to v2
       v2 := &Manifest{
           SchemaVersion: "2.0",
           SessionID:     v1.SessionID,
           Name:          v1.Name,
           CreatedAt:     v1.CreatedAt,
           UpdatedAt:     time.Now(),
           Lifecycle:     "",  // Empty = active/stopped
           Context: Context{
               Project: v1.Context.Project,
           },
           Tmux: Tmux{
               SessionName: v1.Tmux.SessionName,
           },
       }

       // Save v2 (atomic, lock already held)
       if err := Save(v2, path); err != nil {
           // Migration failed, remove backup to allow retry
           os.Remove(backupPath)
           logMigration("FAILED", path, err)
           return fmt.Errorf("failed to save v2: %w", err)
       }

       // Log success
       logMigration("SUCCESS", path, nil)

       return nil
   }
   ```

3. **Integration with Load()** (UPDATED):
   ```go
   func Load(path string) (*Manifest, error) {
       // Detect version
       version, err := detectVersion(path)
       if err != nil {
           return nil, err
       }

       // Migrate if needed (migration acquires its own lock)
       if version == "1.0" {
           if err := MigrateV1ToV2(path); err != nil {
               return nil, fmt.Errorf("migration failed: %w", err)
           }
       }

       // Load v2 (no lock needed for read-only load)
       data, err := os.ReadFile(path)
       if err != nil {
           return nil, err
       }

       var m Manifest
       if err := yaml.Unmarshal(data, &m); err != nil {
           return nil, err
       }

       return &m, nil
   }
   ```

4. **Migration logging** (same as v1)

5. **Tests** (UPDATED with new test cases):
   - Detect v1 manifest
   - Detect v2 manifest
   - Migrate v1 → v2
   - Backup created (.v1.bak)
   - **NEW**: Idempotency - second migration skips (backup exists)
   - **NEW**: Concurrent migration - two processes, one succeeds, one waits
   - Migration logged (SUCCESS)
   - Failed migration logged (FAILED)
   - **NEW**: Failed migration removes backup (allows retry)
   - Load v1 triggers migration
   - Load v2 skips migration

**Acceptance Criteria** (updated):
- All original 18 from S1-SPRINT-PLAN-v2.md:308-325
- **NEW**: [ ] Migration acquires lock before proceeding
- **NEW**: [ ] Second migration on same file is skipped (idempotent)
- **NEW**: [ ] Concurrent migrations handled safely
- **NEW**: [ ] Failed migration removes backup for retry

---

## Sprint 2: Enhanced Resume & Backup

(Content from v1, with TmuxInterface abstraction added)

### D2.1: Status Computation (6 hours) - UPDATED

**Files to Create**:
```
internal/session/
├── status.go       # Status computation logic (MOVED from cmd/csm/)
└── status_test.go  # Tests
```

**Why moved**: Status computation might be reused in future (API, web UI). Keep in `internal/` for reusability.

**Implementation Tasks**:

1. **Status computation with interface** (UPDATED):
   ```go
   package session

   import (
       "github.com/yourusername/claude-session-manager/internal/manifest"
   )

   // TmuxInterface allows mocking tmux for tests
   type TmuxInterface interface {
       HasSession(name string) (bool, error)
       ListSessions() ([]string, error)
   }

   func ComputeStatus(m *manifest.Manifest, tmux TmuxInterface) string {
       // Check lifecycle first
       if m.Lifecycle == manifest.LifecycleArchived {
           return "archived"
       }

       // Check tmux state
       exists, err := tmux.HasSession(m.Tmux.SessionName)
       if err != nil {
           // Tmux error, assume stopped
           return "stopped"
       }

       if exists {
           return "active"
       }

       return "stopped"
   }

   func ComputeStatusBatch(manifests []*manifest.Manifest, tmux TmuxInterface) map[string]string {
       statuses := make(map[string]string)

       // Get all tmux sessions in one call
       existingSessions, err := tmux.ListSessions()
       if err != nil {
           existingSessions = []string{}  // Assume no sessions on error
       }

       sessionSet := make(map[string]bool)
       for _, name := range existingSessions {
           sessionSet[name] = true
       }

       // Compute status for each manifest
       for _, m := range manifests {
           if m.Lifecycle == manifest.LifecycleArchived {
               statuses[m.Name] = "archived"
           } else if sessionSet[m.Tmux.SessionName] {
               statuses[m.Name] = "active"
           } else {
               statuses[m.Name] = "stopped"
           }
       }

       return statuses
   }
   ```

2. **Update cmd/csm/list.go** to use new package:
   ```go
   import (
       "github.com/yourusername/claude-session-manager/internal/session"
   )

   func runList() error {
       manifests, err := loadAllManifests()
       if err != nil {
           return err
       }

       statuses := session.ComputeStatusBatch(manifests, tmux)

       // ... display logic ...
   }
   ```

3. **Tests** (with mock):
   ```go
   func TestComputeStatus(t *testing.T) {
       mockTmux := &MockTmux{Sessions: map[string]bool{"claude-test": true}}

       m := &manifest.Manifest{
           Lifecycle: "",
           Tmux:      manifest.Tmux{SessionName: "claude-test"},
       }

       status := ComputeStatus(m, mockTmux)
       assert.Equal(t, "active", status)
   }
   ```

**Acceptance Criteria** (original 15 + new):
- All original from S2-SPRINT-PLAN-v2.md:197-211
- **NEW**: [ ] Status computation uses TmuxInterface
- **NEW**: [ ] Tests use mock tmux (no real tmux needed)

### D2.2: Enhanced Resume (Auto-Recreation) (8 hours) - UPDATED

**Uses TmuxInterface throughout**:

```go
func runResume(identifier string) error {
    // ... load manifest ...

    // Check if tmux session exists
    exists, err := tmux.HasSession(sessionName)
    if err != nil {
        return fmt.Errorf("tmux error: %w", err)
    }

    if !exists {
        // Session doesn't exist, auto-recreate
        fmt.Printf("Session stopped. Recreating tmux session '%s'...\n", sessionName)

        if err := recreateTmuxSession(m, tmux); err != nil {
            return fmt.Errorf("failed to recreate session: %w", err)
        }

        fmt.Println("Session recreated successfully.")
    }

    // Attach to session
    return tmux.AttachSession(sessionName)
}

func recreateTmuxSession(m *manifest.Manifest, tmux TmuxInterface) error {
    // Sanitize session name
    sessionName, err := sanitizeSessionName(m.Tmux.SessionName)
    if err != nil {
        return err
    }

    // Create tmux session
    if err := tmux.CreateSession(sessionName, m.Context.Project); err != nil {
        return fmt.Errorf("tmux create failed: %w", err)
    }

    // Send resume command
    resumeCmd := fmt.Sprintf("claude --resume %s", m.SessionID)
    if err := tmux.SendKeys(sessionName, resumeCmd); err != nil {
        return fmt.Errorf("failed to send claude resume command: %w", err)
    }

    return nil
}
```

**Tests with mock**:
```go
func TestResumeAutoRecreation(t *testing.T) {
    mockTmux := NewMockTmux()
    // Session doesn't exist initially
    mockTmux.Sessions["claude-test"] = false

    // ... run resume ...

    // Verify session was created
    assert.True(t, mockTmux.Sessions["claude-test"])
}
```

---

## Sprint 3: Health, Operations & Testing

(Content from v1, with doctor fix confirmation clarified)

### D3.1: Doctor Command (10 hours) - UPDATED

**Doctor Fix Confirmation Strategy**:

**Approach**: No interactive confirmation, use --dry-run for preview

**Rationale**:
1. Locks < 60s are protected (very safe threshold)
2. --dry-run flag allows preview without risk
3. Interactive prompts break automation
4. Doctor is a diagnostic tool (low-risk fixes only)

**User workflow**:
```bash
# Preview what would be fixed
csm doctor --fix --dry-run

# Review output, then apply if safe
csm doctor --fix
```

**Documentation addition** (in help text):
```
--fix: Automatically fix issues (removes stale locks > 60s old)
       Safe: Active locks (< 60s) are never removed
       Use --dry-run to preview fixes before applying

Examples:
  # Preview fixes
  csm doctor --fix --dry-run

  # Apply fixes (no confirmation needed - protected by 60s threshold)
  csm doctor --fix
```

**All other D3.1 implementation same as v1**

---

(Rest of Sprint 3 content same as v1)

---

## Success Criteria

(Same as v1, all original criteria apply)

---

## Changes from v1 to v2

### Major Additions

1. **Prerequisites Section** (NEW):
   - Go 1.21+ requirement
   - Tmux 3.0+ requirement
   - go.mod initialization
   - External dependencies (yaml.v3)
   - Environment verification commands

2. **Development Workflow Section** (NEW):
   - Local development setup
   - Test execution commands (unit, integration, benchmarks)
   - Build and run commands
   - Debugging with delve
   - Code quality checks

3. **CI/CD Configuration Section** (NEW):
   - GitHub Actions workflow (test.yml)
   - Makefile with common targets
   - Deployment verification script
   - Smoke test suite

4. **Configuration Management Section** (NEW):
   - ~/.csmrc configuration file
   - Configuration struct and loading
   - Default values
   - Path expansion (~/ handling)

5. **Test Fixtures & Examples Section** (NEW):
   - Sample v1/v2 manifests
   - Sample history.jsonl files
   - TmuxInterface and MockTmux
   - Test execution commands

### Code Improvements

6. **Migration Concurrency Fix**:
   - Acquire lock before migration
   - Prevents race condition on v1 manifests

7. **Migration Idempotency**:
   - Check .v1.bak exists before creating
   - Skip migration if backup exists
   - Remove backup on failed save (allow retry)

8. **TmuxInterface Abstraction**:
   - Interface for tmux operations
   - RealTmux for production
   - MockTmux for testing
   - Status computation uses interface
   - Resume uses interface

9. **Status Computation Location**:
   - Moved from cmd/csm/ to internal/session/
   - Allows reuse in future features

### Documentation Improvements

10. **Doctor Fix Confirmation Clarified**:
    - No interactive confirmation (automation-friendly)
    - --dry-run for preview
    - 60s threshold protects active locks
    - Rationale documented

11. **Backup Directory Permissions**:
    - Verified: backups/ directory created with 0700
    - Backup files created with 0600
    - Documented in security notes

---

## Review Checklist

- [x] Prerequisites section added (Go, tmux, dependencies)
- [x] Development workflow section added (test, build, debug)
- [x] CI/CD configuration section added (GitHub Actions, Makefile)
- [x] Configuration management section added (~/.csmrc)
- [x] Test fixtures section added (samples, mocks)
- [x] Migration concurrency fixed (lock before migrate)
- [x] Migration idempotency fixed (.v1.bak check)
- [x] TmuxInterface abstraction added
- [x] Status computation moved to internal/session/
- [x] Doctor fix confirmation clarified
- [x] All Round 1 feedback addressed

---

**Status**: Ready for Multi-Persona Review Round 2
**Version**: 2.0
**Last Updated**: December 7, 2025
