# S4: Implementation - Phase 3.5 Session Persistence

**Date**: December 7, 2025
**Status**: 🔄 IN PROGRESS - Implementation Execution
**Phase Goal**: Implement all 11 deliverables from S1, S2, and S3 sprint plans
**Prerequisites**:
- ✅ D1 Discovery Complete
- ✅ D2 Architecture Approved (8.8/10)
- ✅ D3 Implementation Design Approved (9.0/10)
- ✅ D4 Requirements Approved (9.3/10)
- ✅ S1 Sprint Plan Approved (9.4/10)
- ✅ S2 Sprint Plan Approved (9.5/10)
- ✅ S3 Sprint Plan Approved (9.5/10)

---

## Executive Summary

This phase implements the complete Phase 3.5 Session Persistence feature set across three sprints:

**Sprint 1 (S1)**: Foundation & Core Infrastructure
- 5 deliverables: Schema v2, Validation, Locking, Migration, Fileutil
- 2-3 days estimated

**Sprint 2 (S2)**: Enhanced Resume & Backup
- 3 deliverables: Status Computation, Enhanced Resume, Backup Command
- 2-3 days estimated

**Sprint 3 (S3)**: Health, Operations & Testing
- 3 deliverables: Doctor Command, Log Rotation, Integration Testing
- 2-3 days estimated

**Total**: 11 deliverables, 6-9 days estimated, all specifications complete

---

## Implementation Approach

### Execution Model

Following Wayfinder methodology for implementation:

1. **Implement by Sprint** - Complete S1 → S2 → S3 in order
2. **Test as You Go** - Write tests alongside implementation
3. **Commit Frequently** - Commit after each deliverable
4. **Review at Sprint Boundaries** - Multi-persona review after each sprint
5. **Integration Testing** - Comprehensive testing in S3

### Quality Gates

Each deliverable must meet:
- ✅ All acceptance criteria from sprint plan
- ✅ Unit tests passing
- ✅ Code coverage targets (>80% critical, >60% overall)
- ✅ No critical bugs
- ✅ Documentation complete (godoc + user docs)

Each sprint must meet:
- ✅ All deliverables complete
- ✅ Integration tests passing (cross-deliverable)
- ✅ Multi-persona review ≥8.5/10
- ✅ Committed to git

---

## Sprint 1: Foundation & Core Infrastructure

**Goal**: Implement foundational components that all other features depend on

**Duration**: 2-3 days
**Deliverables**: 5

### D1.1: Manifest Schema v2 (6 hours)

**Files to Create**:
```
internal/manifest/
├── manifest.go         # Schema v2 struct + Load/Save
├── constants.go        # All constants centralized
└── manifest_test.go    # Tests
```

**Implementation Tasks**:
1. Define `Manifest` struct with v2 fields:
   ```go
   type Manifest struct {
       SchemaVersion string    `yaml:"schema_version"`
       SessionID     string    `yaml:"session_id"`
       Name          string    `yaml:"name"`
       CreatedAt     time.Time `yaml:"created_at"`
       UpdatedAt     time.Time `yaml:"updated_at"`
       Lifecycle     string    `yaml:"lifecycle"`  // "" or "archived"
       Context       Context   `yaml:"context"`
       Tmux          Tmux      `yaml:"tmux"`
   }

   type Context struct {
       Project      string   `yaml:"project"`
       Purpose      string   `yaml:"purpose,omitempty"`
       Tags         []string `yaml:"tags,omitempty"`
       Notes        string   `yaml:"notes,omitempty"`
   }
   ```

2. Implement `Load(path string) (*Manifest, error)`:
   - Read YAML file
   - Unmarshal into struct
   - Trigger migration if v1 detected
   - Return v2 manifest

3. Implement `Save(m *Manifest, path string) error`:
   - Use fileutil.AtomicWrite
   - Marshal to YAML
   - Set UpdatedAt timestamp

4. Write tests:
   - Load v2 manifest
   - Save v2 manifest
   - Roundtrip (save → load → verify)

**Acceptance Criteria**: 18 from S1-SPRINT-PLAN-v2.md:170-187

### D1.2: Context Validation (3 hours)

**Files to Modify**:
```
internal/manifest/
└── manifest.go  # Add Validate() method
```

**Implementation Tasks**:
1. Implement `Validate() error`:
   ```go
   func (m *Manifest) Validate() error {
       // Check required fields
       if m.SessionID == "" {
           return errors.New("session_id is required")
       }

       // UTF-8 character counting
       if utf8.RuneCountInString(m.Context.Purpose) > MaxPurposeLen {
           return fmt.Errorf("purpose exceeds %d characters", MaxPurposeLen)
       }

       // Tags validation
       if len(m.Context.Tags) > MaxTagsCount {
           return fmt.Errorf("too many tags (max %d)", MaxTagsCount)
       }
       for _, tag := range m.Context.Tags {
           if utf8.RuneCountInString(tag) > MaxTagLen {
               return fmt.Errorf("tag exceeds %d characters", MaxTagLen)
           }
       }

       // Notes validation
       if utf8.RuneCountInString(m.Context.Notes) > MaxNotesLen {
           return fmt.Errorf("notes exceed %d characters", MaxNotesLen)
       }

       return nil
   }
   ```

2. Call `Validate()` in `Save()` before writing

3. Write tests:
   - Valid manifest passes
   - Purpose too long fails
   - Too many tags fails
   - Tag too long fails
   - Notes too long fails
   - UTF-8 character counting (emoji test)

**Acceptance Criteria**: 15 from S1-SPRINT-PLAN-v2.md:207-221

### D1.3: File Locking (6 hours)

**Files to Create**:
```
internal/manifest/
├── lock.go       # Locking implementation
└── lock_test.go  # Tests
```

**Implementation Tasks**:
1. Implement lock file format:
   ```go
   type Lock struct {
       PID       int
       Timestamp time.Time
   }

   func (l *Lock) Write(path string) error {
       content := fmt.Sprintf("%d\n%s\n", l.PID, l.Timestamp.Format(time.RFC3339))
       return os.WriteFile(path, []byte(content), 0600)
   }

   func ReadLock(path string) (*Lock, error) {
       data, err := os.ReadFile(path)
       if err != nil {
           return nil, err
       }
       lines := strings.Split(strings.TrimSpace(string(data)), "\n")
       if len(lines) < 2 {
           return nil, errors.New("invalid lock format")
       }

       pid, err := strconv.Atoi(lines[0])
       if err != nil {
           return nil, fmt.Errorf("invalid PID: %w", err)
       }

       timestamp, err := time.Parse(time.RFC3339, lines[1])
       if err != nil {
           return nil, fmt.Errorf("invalid timestamp: %w", err)
       }

       return &Lock{PID: pid, Timestamp: timestamp}, nil
   }
   ```

2. Implement `AcquireLock(manifestPath string) error`:
   ```go
   func AcquireLock(manifestPath string) error {
       lockPath := manifestPath + ".lock"

       // Check if lock exists
       if _, err := os.Stat(lockPath); err == nil {
           lock, err := ReadLock(lockPath)
           if err != nil {
               // Corrupted lock, remove it
               os.Remove(lockPath)
           } else {
               age := time.Since(lock.Timestamp)
               if age < LockTimeout {
                   // Active lock
                   return fmt.Errorf("session is locked by process %d (started %s)\n\nTry one of the following:\n  • Wait a minute and retry (process may finish)\n  • Check if process is still running: ps -p %d\n  • If process is stuck, kill it: kill %d\n  • Check for stale locks: csm doctor --fix",
                       lock.PID, lock.Timestamp.Format(time.RFC3339), lock.PID, lock.PID)
               }
               // Stale lock, remove it
               os.Remove(lockPath)
           }
       }

       // Create lock
       lock := &Lock{
           PID:       os.Getpid(),
           Timestamp: time.Now(),
       }
       return lock.Write(lockPath)
   }
   ```

3. Implement `ReleaseLock(manifestPath string) error`:
   ```go
   func ReleaseLock(manifestPath string) error {
       lockPath := manifestPath + ".lock"
       return os.Remove(lockPath)
   }
   ```

4. Write tests:
   - Acquire lock succeeds
   - Second acquire fails (locked)
   - Release lock succeeds
   - Acquire after release succeeds
   - Stale lock (>60s) is removed
   - Active lock (<60s) blocks
   - Error message format correct

**Acceptance Criteria**: 18 from S1-SPRINT-PLAN-v2.md:254-271

### D1.4: Migration v1 → v2 (8 hours)

**Files to Create**:
```
internal/manifest/
├── migrate.go       # Migration logic
└── migrate_test.go  # Tests
```

**Implementation Tasks**:
1. Implement version detection:
   ```go
   func detectVersion(path string) (string, error) {
       data, err := os.ReadFile(path)
       if err != nil {
           return "", err
       }

       var versionCheck struct {
           SchemaVersion string `yaml:"schema_version"`
       }

       if err := yaml.Unmarshal(data, &versionCheck); err != nil {
           return "", err
       }

       if versionCheck.SchemaVersion == "" {
           return "1.0", nil  // Assume v1 if missing
       }

       return versionCheck.SchemaVersion, nil
   }
   ```

2. Implement `MigrateV1ToV2(path string) error`:
   ```go
   func MigrateV1ToV2(path string) error {
       // Read v1 manifest
       data, err := os.ReadFile(path)
       if err != nil {
           return err
       }

       var v1 ManifestV1
       if err := yaml.Unmarshal(data, &v1); err != nil {
           return err
       }

       // Backup original
       backupPath := path + ".v1.bak"
       if err := os.WriteFile(backupPath, data, 0600); err != nil {
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
               // purpose, tags, notes remain empty (not in v1)
           },
           Tmux: Tmux{
               SessionName: v1.Tmux.SessionName,
           },
       }

       // Save v2
       if err := v2.Save(path); err != nil {
           return fmt.Errorf("failed to save v2: %w", err)
       }

       // Log migration
       logMigration("SUCCESS", path, nil)

       return nil
   }
   ```

3. Implement migration logging:
   ```go
   func logMigration(status string, path string, err error) {
       logPath := filepath.Join(os.Getenv("HOME"), ".csm", "logs", "migration.log")

       // Create logs directory if needed
       os.MkdirAll(filepath.Dir(logPath), 0700)

       f, openErr := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
       if openErr != nil {
           fmt.Fprintf(os.Stderr, "Warning: cannot write migration log: %v\n", openErr)
           return
       }
       defer f.Close()

       timestamp := time.Now().Format(time.RFC3339)
       if err != nil {
           fmt.Fprintf(f, "[%s] %s: %s - %v\n", timestamp, status, path, err)
       } else {
           fmt.Fprintf(f, "[%s] %s: %s\n", timestamp, status, path)
       }
   }
   ```

4. Integrate with `Load()`:
   ```go
   func Load(path string) (*Manifest, error) {
       // Detect version
       version, err := detectVersion(path)
       if err != nil {
           return nil, err
       }

       // Migrate if needed
       if version == "1.0" {
           if err := MigrateV1ToV2(path); err != nil {
               logMigration("FAILED", path, err)
               return nil, fmt.Errorf("migration failed: %w", err)
           }
       }

       // Load v2
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

5. Write tests:
   - Detect v1 manifest
   - Detect v2 manifest
   - Migrate v1 → v2
   - Backup created (.v1.bak)
   - Migration logged (SUCCESS)
   - Failed migration logged (FAILED)
   - Load v1 triggers migration
   - Load v2 skips migration

**Acceptance Criteria**: 18 from S1-SPRINT-PLAN-v2.md:308-325

### D1.5: Fileutil Package (3 hours)

**Files to Create**:
```
internal/fileutil/
├── atomic.go       # Atomic write operations
└── atomic_test.go  # Tests
```

**Implementation Tasks**:
1. Implement `AtomicWrite(path string, data []byte, perm os.FileMode) error`:
   ```go
   func AtomicWrite(path string, data []byte, perm os.FileMode) error {
       // Create temp file in same directory (for atomic rename)
       dir := filepath.Dir(path)
       tmpFile, err := os.CreateTemp(dir, ".tmp-*")
       if err != nil {
           return fmt.Errorf("cannot create temp file: %w", err)
       }
       tmpPath := tmpFile.Name()

       // Write data
       if _, err := tmpFile.Write(data); err != nil {
           tmpFile.Close()
           os.Remove(tmpPath)
           return fmt.Errorf("cannot write temp file: %w", err)
       }

       // Set permissions
       if err := tmpFile.Chmod(perm); err != nil {
           tmpFile.Close()
           os.Remove(tmpPath)
           return fmt.Errorf("cannot set permissions: %w", err)
       }

       // Close before rename (Windows compatibility)
       if err := tmpFile.Close(); err != nil {
           os.Remove(tmpPath)
           return fmt.Errorf("cannot close temp file: %w", err)
       }

       // Atomic rename
       if err := os.Rename(tmpPath, path); err != nil {
           os.Remove(tmpPath)
           return fmt.Errorf("cannot rename temp file: %w", err)
       }

       return nil
   }
   ```

2. Write tests:
   - Write new file succeeds
   - Overwrite existing file succeeds
   - Permissions set correctly (0600)
   - Temp file cleaned up on error
   - Atomic (crash during write doesn't corrupt)

**Acceptance Criteria**: 10 from S1-SPRINT-PLAN-v2.md:356-365

### Sprint 1 Completion Criteria

- [ ] All 5 deliverables implemented
- [ ] All unit tests passing
- [ ] Test coverage >80% for manifest package
- [ ] No critical bugs
- [ ] Code committed to git
- [ ] Multi-persona review ≥8.5/10

---

## Sprint 2: Enhanced Resume & Backup

**Goal**: Implement user-facing features that depend on S1 foundation

**Duration**: 2-3 days
**Deliverables**: 3

### D2.1: Status Computation (6 hours)

**Files to Create**:
```
cmd/csm/
├── status.go       # Status computation logic
└── status_test.go  # Tests
```

**Implementation Tasks**:
1. Implement `ComputeStatus(m *manifest.Manifest) string`:
   ```go
   func ComputeStatus(m *manifest.Manifest) string {
       // Check lifecycle first
       if m.Lifecycle == manifest.LifecycleArchived {
           return "archived"
       }

       // Check tmux state
       sessionName := m.Tmux.SessionName
       cmd := exec.Command("tmux", "has-session", "-t", sessionName)
       if err := cmd.Run(); err == nil {
           return "active"
       }

       return "stopped"
   }
   ```

2. Implement batch status checking:
   ```go
   func ComputeStatusBatch(manifests []*manifest.Manifest) map[string]string {
       statuses := make(map[string]string)

       // Separate archived from others
       var activeNames []string
       for _, m := range manifests {
           if m.Lifecycle == manifest.LifecycleArchived {
               statuses[m.Name] = "archived"
           } else {
               activeNames = append(activeNames, m.Tmux.SessionName)
           }
       }

       // Single tmux query for all non-archived
       existingSessions := getTmuxSessions()  // Parse `tmux list-sessions`

       for _, m := range manifests {
           if statuses[m.Name] != "" {
               continue  // Already marked as archived
           }

           if contains(existingSessions, m.Tmux.SessionName) {
               statuses[m.Name] = "active"
           } else {
               statuses[m.Name] = "stopped"
           }
       }

       return statuses
   }
   ```

3. Update `csm list` command:
   ```go
   func runList() error {
       manifests, err := loadAllManifests()
       if err != nil {
           return err
       }

       statuses := ComputeStatusBatch(manifests)

       fmt.Printf("%-12s %-10s %-40s %s\n", "NAME", "STATUS", "UUID", "PROJECT")
       fmt.Println(strings.Repeat("-", 80))

       for _, m := range manifests {
           status := statuses[m.Name]
           fmt.Printf("%-12s %-10s %-40s %s\n",
               m.Name, status, m.SessionID, m.Context.Project)
       }

       return nil
   }
   ```

4. Write tests:
   - Lifecycle "archived" → status "archived"
   - Tmux exists → status "active"
   - Tmux missing → status "stopped"
   - Batch checking (single tmux query)
   - List command shows correct statuses

**Acceptance Criteria**: 15 from S2-SPRINT-PLAN-v2.md:197-211

### D2.2: Enhanced Resume (Auto-Recreation) (8 hours)

**Files to Modify**:
```
cmd/csm/
├── resume.go       # Add auto-recreation logic
└── resume_test.go  # Add tests
```

**Implementation Tasks**:
1. Detect missing tmux session:
   ```go
   func runResume(identifier string) error {
       // Load manifest
       m, err := loadManifest(identifier)
       if err != nil {
           return err
       }

       // Acquire lock
       if err := manifest.AcquireLock(manifestPath); err != nil {
           return err
       }
       defer manifest.ReleaseLock(manifestPath)

       // Check status
       status := ComputeStatus(m)

       if status == "archived" {
           return errors.New("cannot resume archived session")
       }

       sessionName := m.Tmux.SessionName

       // Check if tmux session exists
       cmd := exec.Command("tmux", "has-session", "-t", sessionName)
       if err := cmd.Run(); err != nil {
           // Session doesn't exist, auto-recreate
           fmt.Printf("Session stopped. Recreating tmux session '%s'...\n", sessionName)

           if err := recreateTmuxSession(m); err != nil {
               return fmt.Errorf("failed to recreate session: %w", err)
           }

           fmt.Println("Session recreated successfully.")
       }

       // Attach to session
       return attachToSession(sessionName)
   }
   ```

2. Implement `recreateTmuxSession(m *manifest.Manifest) error`:
   ```go
   func recreateTmuxSession(m *manifest.Manifest) error {
       // Sanitize session name
       sessionName, err := sanitizeSessionName(m.Tmux.SessionName)
       if err != nil {
           return err
       }

       // Create tmux session
       cmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName,
           "-c", m.Context.Project)
       if err := cmd.Run(); err != nil {
           return fmt.Errorf("tmux new-session failed: %w", err)
       }

       // Send resume command to tmux
       resumeCmd := fmt.Sprintf("claude --resume %s", m.SessionID)
       cmd = exec.Command("tmux", "send-keys", "-t", sessionName, resumeCmd, "Enter")
       if err := cmd.Run(); err != nil {
           return fmt.Errorf("failed to send claude resume command: %w", err)
       }

       return nil
   }
   ```

3. Implement input sanitization:
   ```go
   func sanitizeSessionName(name string) (string, error) {
       validPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
       if !validPattern.MatchString(name) {
           return "", fmt.Errorf("invalid session name: contains prohibited characters")
       }
       return name, nil
   }
   ```

4. Write tests:
   - Resume active session (attach only)
   - Resume stopped session (recreate + attach)
   - Resume archived session (error)
   - Sanitization blocks invalid names
   - Recreate uses correct project directory
   - Recreate sends claude resume command

**Acceptance Criteria**: 18 from S2-SPRINT-PLAN-v2.md:244-261

### D2.3: Backup Command (6 hours)

**Files to Create**:
```
cmd/csm/
├── backup.go       # Backup implementation
└── backup_test.go  # Tests
```

**Implementation Tasks**:
1. Implement `extractConversation(historyPath string, sessionUUID string)`:
   ```go
   func extractConversation(historyPath string, sessionUUID string) ([]HistoryEntry, error) {
       file, err := os.Open(historyPath)
       if err != nil {
           return nil, fmt.Errorf("cannot open history file: %w", err)
       }
       defer file.Close()

       var entries []HistoryEntry
       var skippedCount int

       scanner := bufio.NewScanner(file)
       scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)  // 10MB line limit

       for scanner.Scan() {
           line := scanner.Bytes()

           var entry HistoryEntry
           if err := json.Unmarshal(line, &entry); err != nil {
               skippedCount++
               continue  // Skip malformed lines
           }

           if entry.SessionID == sessionUUID {
               entries = append(entries, entry)
           }
       }

       if err := scanner.Err(); err != nil {
           return nil, fmt.Errorf("error reading history: %w", err)
       }

       if skippedCount > 0 {
           fmt.Fprintf(os.Stderr, "Warning: skipped %d malformed entries\n", skippedCount)
       }

       return entries, nil
   }
   ```

2. Implement `runBackup(identifier string) error`:
   ```go
   func runBackup(identifier string) error {
       // Load manifest
       m, err := loadManifest(identifier)
       if err != nil {
           return err
       }

       // Acquire lock
       if err := manifest.AcquireLock(manifestPath); err != nil {
           return err
       }
       defer manifest.ReleaseLock(manifestPath)

       // Extract conversation
       historyPath := filepath.Join(os.Getenv("HOME"), ".claude", "history.jsonl")
       entries, err := extractConversation(historyPath, m.SessionID)
       if err != nil {
           return err
       }

       if len(entries) == 0 {
           return errors.New("no conversation history found")
       }

       // Create backup
       backupDir := filepath.Join(sessionDir, "backups")
       os.MkdirAll(backupDir, 0700)

       timestamp := time.Now().Format("20060102-150405")
       backupPath := filepath.Join(backupDir, fmt.Sprintf("backup-%s.jsonl", timestamp))

       // Write backup (atomic)
       var buf bytes.Buffer
       for _, entry := range entries {
           data, _ := json.Marshal(entry)
           buf.Write(data)
           buf.WriteByte('\n')
       }

       if err := fileutil.AtomicWrite(backupPath, buf.Bytes(), 0600); err != nil {
           return fmt.Errorf("failed to write backup: %w", err)
       }

       fmt.Printf("Backed up %d messages to %s\n", len(entries), backupPath)

       // Cleanup old backups
       if err := cleanupOldBackups(backupDir, 10); err != nil {
           fmt.Fprintf(os.Stderr, "Warning: backup cleanup failed: %v\n", err)
       }

       return nil
   }
   ```

3. Implement `cleanupOldBackups(dir string, keep int) error`:
   ```go
   func cleanupOldBackups(dir string, keep int) error {
       files, err := filepath.Glob(filepath.Join(dir, "backup-*.jsonl"))
       if err != nil {
           return err
       }

       if len(files) <= keep {
           return nil  // Nothing to delete
       }

       // Sort by modification time (newest first)
       sort.Slice(files, func(i, j int) bool {
           iInfo, _ := os.Stat(files[i])
           jInfo, _ := os.Stat(files[j])
           return iInfo.ModTime().After(jInfo.ModTime())
       })

       // Delete oldest
       for i := keep; i < len(files); i++ {
           if err := os.Remove(files[i]); err != nil {
               return fmt.Errorf("failed to delete %s: %w", files[i], err)
           }
       }

       return nil
   }
   ```

4. Write tests:
   - Extract conversation from history.jsonl
   - Skip malformed JSON lines
   - Create backup file with timestamp
   - Backup contains all messages
   - Cleanup keeps last 10 backups
   - Atomic write (crash-safe)
   - File permissions (0600)

**Acceptance Criteria**: 18 from S2-SPRINT-PLAN-v2.md:307-324

### Sprint 2 Completion Criteria

- [ ] All 3 deliverables implemented
- [ ] All unit tests passing
- [ ] Integration tests (S1+S2 together)
- [ ] Test coverage >80% for cmd/csm
- [ ] No critical bugs
- [ ] Code committed to git
- [ ] Multi-persona review ≥8.5/10

---

## Sprint 3: Health, Operations & Testing

**Goal**: Make system production-ready through health checks, operations, and comprehensive testing

**Duration**: 2-3 days
**Deliverables**: 3

### D3.1: Doctor Command (10 hours)

**Files to Create**:
```
cmd/csm/
├── doctor.go                      # Doctor command
├── doctor_test.go                 # Unit tests
├── doctor_integration_test.go     # Integration tests
├── doctor_fix_test.go             # Fix mode tests
├── doctor_quiet_test.go           # Output mode tests
├── doctor_specific_test.go        # Specific session tests
├── doctor_optimization_test.go    # UUID optimization tests
└── doctor_dryrun_test.go          # Dry-run mode tests
```

**Implementation Tasks**: See S3-SPRINT-PLAN-v2.md:437-595 for complete specification

**Key Components**:
1. Health check functions
2. UUID optimization (single history parse)
3. Lock safety (protect < 60s locks)
4. Output modes (verbose, summary, quiet, dry-run)
5. Fix action logging

**Acceptance Criteria**: 39 from S3-SPRINT-PLAN-v2.md:552-595

### D3.2: Log Rotation (5 hours)

**Files to Create**:
```
internal/logging/
├── rotate.go                      # Log rotation
├── rotate_test.go                 # Unit tests
├── rotate_integration_test.go     # Integration tests
├── rotate_edge_test.go            # Edge case tests
└── rotate_fallback_test.go        # Fallback tests
```

**Implementation Tasks**: See S3-SPRINT-PLAN-v2.md:607-730 for complete specification

**Key Components**:
1. Rotation policy (10MB trigger, 5 files)
2. Atomic operations (temp + rename)
3. Fallback strategy (/tmp on disk full)
4. File permissions (0600)

**Acceptance Criteria**: 17 from S3-SPRINT-PLAN-v2.md:703-725

### D3.3: Integration & Performance Testing (10 hours)

**Files to Create**:
```
cmd/csm/
├── integration_test.go            # Integration tests (TS-INT-1 to TS-INT-15)
├── benchmark_test.go              # Performance benchmarks (BM-1 to BM-9)
├── load_test.go                   # Load testing
├── stress_test.go                 # Stress testing
├── test_helpers.go                # Test utilities
└── testdata/                      # Test fixtures
    ├── manifests/
    ├── history/
    └── worktrees/
```

**Implementation Tasks**: See S3-SPRINT-PLAN-v2.md:734-847 for complete specification

**Key Components**:
1. 15 integration test scenarios
2. 9 performance benchmarks
3. Test infrastructure (fixtures, mocks, helpers)
4. Fast vs slow test separation

**Acceptance Criteria**: 17 from S3-SPRINT-PLAN-v2.md:821-840

### Sprint 3 Completion Criteria

- [ ] All 3 deliverables implemented
- [ ] All integration tests passing (TS-INT-1 to TS-INT-15)
- [ ] All performance benchmarks meeting targets (BM-1 to BM-9)
- [ ] Test coverage >80% critical, >60% overall
- [ ] Fast tests < 2 minutes (CI)
- [ ] No critical bugs
- [ ] Code committed to git
- [ ] Multi-persona review ≥8.5/10

---

## Testing Strategy

### Unit Tests
- Written alongside implementation
- One test file per implementation file
- Test coverage >80% for critical paths, >60% overall

### Integration Tests
- Test cross-deliverable interactions
- Test S1+S2 integration after Sprint 2
- Test S1+S2+S3 integration after Sprint 3

### Performance Tests
- Benchmarks for all critical operations
- Validate against NFR targets from D4
- Profile if targets not met

### Test Organization
- Fast tests (< 2 min): Unit + quick integration
- Slow tests (nightly): Load + stress + long-running

---

## Success Criteria

Phase 3.5 Implementation (S4) is **DONE** when:

### Functional
- ✅ All 11 deliverables implemented
- ✅ All acceptance criteria met (127 total across S1+S2+S3)
- ✅ All features working as specified

### Quality
- ✅ All unit tests passing
- ✅ All integration tests passing (15 scenarios)
- ✅ All performance benchmarks meeting targets (9 benchmarks)
- ✅ Test coverage >80% critical, >60% overall
- ✅ Zero critical bugs
- ✅ Multi-persona review ≥8.5/10 for each sprint

### Documentation
- ✅ Godoc comments on all exported functions
- ✅ Inline comments for complex logic
- ✅ User documentation (help text, examples)
- ✅ Developer documentation (README, CHANGELOG)

### Deployment
- ✅ All code committed to git
- ✅ Post-deployment verification passed
- ✅ Rollback procedure tested

---

## Implementation Timeline

### Week 1
- **Day 1-2**: Sprint 1 (Foundation)
  - D1.1: Manifest Schema v2
  - D1.2: Context Validation
  - D1.3: File Locking
  - D1.4: Migration v1 → v2
  - D1.5: Fileutil Package
  - Sprint 1 review

- **Day 3**: Sprint 2 Start (User Features)
  - D2.1: Status Computation
  - D2.2: Enhanced Resume (partial)

### Week 2
- **Day 4**: Sprint 2 Complete
  - D2.2: Enhanced Resume (complete)
  - D2.3: Backup Command
  - Sprint 2 review

- **Day 5-6**: Sprint 3 Start (Operations)
  - D3.1: Doctor Command
  - D3.2: Log Rotation

- **Day 7-8**: Sprint 3 Complete
  - D3.3: Integration & Performance Testing
  - Sprint 3 review
  - Final review and documentation

### Contingency
- **Day 9**: Buffer for issues, additional testing, documentation polish

---

## Risk Management

### Risk 1: Implementation Complexity Higher Than Estimated
**Probability**: MEDIUM
**Impact**: MEDIUM
**Mitigation**:
- Detailed specifications already complete
- Code examples in sprint plans
- Buffer day in timeline
- Can extend if needed

### Risk 2: Performance Targets Not Met
**Probability**: LOW
**Impact**: MEDIUM
**Mitigation**:
- Profile early if targets missed
- Optimizations already specified (batch operations, caching)
- Can adjust targets if necessary (document reasoning)

### Risk 3: Integration Test Flakiness
**Probability**: MEDIUM
**Impact**: LOW
**Mitigation**:
- Test isolation strategy defined
- Cleanup between tests
- Mock strategy for CI
- Run multiple times to verify stability

### Risk 4: Unforeseen Edge Cases
**Probability**: LOW
**Impact**: LOW
**Mitigation**:
- Comprehensive planning caught most edge cases
- Multi-persona reviews identified issues
- Can add tests as discovered

---

## Commit Strategy

**After each deliverable**:
```bash
git add <files>
git commit -m "<deliverable>: <description>

<details>

🤖 Generated with Claude Code
Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

**After each sprint**:
```bash
git tag s<N>-complete
git push && git push --tags
```

---

## Review Process

After each sprint completion:

1. **Self-review**: Check all acceptance criteria
2. **Test validation**: All tests passing
3. **Multi-persona review**: 5-6 reviewers
4. **Iterate if needed**: Address feedback < 8.5/10
5. **Commit and tag**: Mark sprint complete
6. **Report to user**: Summary + paths + score

---

## Next Steps

**Current Status**: S4 Implementation Plan created, ready for execution

**To begin implementation**:
1. Start with Sprint 1, Deliverable 1.1 (Manifest Schema v2)
2. Create `internal/manifest/` directory structure
3. Implement schema, tests, documentation
4. Commit and proceed to D1.2

**Awaiting user approval to begin implementation**.

---

## Files Referenced

All sprint plans with complete specifications:
- `S1-SPRINT-PLAN-v2.md` - Foundation (5 deliverables)
- `S2-SPRINT-PLAN-v2.md` - User Features (3 deliverables)
- `S3-SPRINT-PLAN-v2.md` - Operations (3 deliverables)

All architecture and design:
- `D2-ARCHITECTURE-v2.md` - Architecture decisions
- `D3-IMPLEMENTATION-v2-CHANGES.md` - Implementation design
- `D4-REQUIREMENTS-v2.md` - Complete requirements

---

**End of S4 Implementation Plan**
