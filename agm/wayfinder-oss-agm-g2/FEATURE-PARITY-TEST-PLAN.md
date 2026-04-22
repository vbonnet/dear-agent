---
date: "2026-02-03"
bead: oss-agm-g2
status: READY FOR IMPLEMENTATION (blocked on GeminiAdapter completion)
---

# Gemini Feature Parity Test Plan

## Overview

This document outlines comprehensive integration tests to verify GeminiAdapter has complete feature parity with ClaudeAdapter. Tests are designed using Ginkgo's DescribeTable pattern for parameterization.

**Current Status**: ⚠️ Tests ready to implement, but BLOCKED by incomplete GeminiAdapter (see TEST-ANALYSIS-REPORT.md)

## Test Strategy

### Approach
Use Ginkgo DescribeTable pattern to run identical test scenarios for both agents:
- Same test logic for both "claude" and "gemini"
- Parameterized execution with agent name
- Consistent assertions and validation
- Reuse existing test helpers

### Test Organization

```
test/integration/
├── agent_parity/
│   ├── session_management_test.go     (CreateSession, Resume, Terminate, Status)
│   ├── messaging_test.go              (SendMessage, GetHistory)
│   ├── data_exchange_test.go          (Export, Import)
│   ├── capabilities_test.go           (Capabilities verification)
│   ├── command_execution_test.go      (ExecuteCommand variants)
│   └── lifecycle_integration_test.go  (End-to-end workflows)
└── helpers/
    └── agent_helpers.go                (Agent-agnostic test utilities)
```

## Test Suites

### 1. Session Management Tests

**File**: `test/integration/agent_parity/session_management_test.go`

#### Test Cases

**1.1 CreateSession - Basic Session Creation**
```go
DescribeTable("creates new session with default parameters",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName),
            WorkingDirectory: "/tmp",
        }

        sessionID, err := getAdapter(agentName).CreateSession(ctx)

        Expect(err).ToNot(HaveOccurred())
        Expect(sessionID).ToNot(BeEmpty())

        // Verify session is active
        status, err := getAdapter(agentName).GetSessionStatus(sessionID)
        Expect(err).ToNot(HaveOccurred())
        Expect(status).To(Equal(agent.StatusActive))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**1.2 CreateSession - With Project Context**
```go
DescribeTable("creates session with project metadata",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-project"),
            WorkingDirectory: "~/src/ai-tools",
            Project:          "ai-tools",
        }

        sessionID, err := getAdapter(agentName).CreateSession(ctx)
        Expect(err).ToNot(HaveOccurred())

        // Verify project context persisted
        // (Implementation-specific verification)
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**1.3 CreateSession - With Authorized Directories**
```go
DescribeTable("creates session with pre-authorized directories",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-authdir"),
            WorkingDirectory: "/tmp",
            AuthorizedDirs:   []string{"~/src", "~/data"},
        }

        sessionID, err := getAdapter(agentName).CreateSession(ctx)
        Expect(err).ToNot(HaveOccurred())

        // Verify directories authorized (agent-specific validation)
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**1.4 ResumeSession - Resume Active Session**
```go
DescribeTable("resumes existing active session",
    func(agentName string) {
        // Create session
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-resume"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)

        // Resume session
        err := getAdapter(agentName).ResumeSession(sessionID)
        Expect(err).ToNot(HaveOccurred())

        // Verify still active
        status, _ := getAdapter(agentName).GetSessionStatus(sessionID)
        Expect(status).To(Equal(agent.StatusActive))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**1.5 ResumeSession - Error on Non-Existent Session**
```go
DescribeTable("returns error when resuming non-existent session",
    func(agentName string) {
        fakeSessionID := agent.SessionID("non-existent-session-id")

        err := getAdapter(agentName).ResumeSession(fakeSessionID)

        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("not found"))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**1.6 TerminateSession - Graceful Shutdown**
```go
DescribeTable("terminates active session gracefully",
    func(agentName string) {
        // Create session
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-terminate"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)

        // Terminate
        err := getAdapter(agentName).TerminateSession(sessionID)
        Expect(err).ToNot(HaveOccurred())

        // Verify terminated
        status, _ := getAdapter(agentName).GetSessionStatus(sessionID)
        Expect(status).To(Equal(agent.StatusTerminated))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**1.7 GetSessionStatus - Active Session**
```go
DescribeTable("returns active status for running session",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-status"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)

        status, err := getAdapter(agentName).GetSessionStatus(sessionID)

        Expect(err).ToNot(HaveOccurred())
        Expect(status).To(Equal(agent.StatusActive))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**1.8 GetSessionStatus - Terminated Session**
```go
DescribeTable("returns terminated status after session ends",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-terminated"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        getAdapter(agentName).TerminateSession(sessionID)

        status, err := getAdapter(agentName).GetSessionStatus(sessionID)

        Expect(err).ToNot(HaveOccurred())
        Expect(status).To(Equal(agent.StatusTerminated))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

### 2. Messaging Tests

**File**: `test/integration/agent_parity/messaging_test.go`

#### Test Cases

**2.1 SendMessage - Single User Message**
```go
DescribeTable("sends user message to agent",
    func(agentName string) {
        // Create session
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-msg"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        defer getAdapter(agentName).TerminateSession(sessionID)

        // Send message
        msg := agent.Message{
            Role:    agent.RoleUser,
            Content: "Hello, this is a test message",
        }

        err := getAdapter(agentName).SendMessage(sessionID, msg)

        Expect(err).ToNot(HaveOccurred())
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**2.2 SendMessage - Error on Terminated Session**
```go
DescribeTable("returns error when sending to terminated session",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-msgterm"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        getAdapter(agentName).TerminateSession(sessionID)

        msg := agent.Message{
            Role:    agent.RoleUser,
            Content: "This should fail",
        }

        err := getAdapter(agentName).SendMessage(sessionID, msg)

        Expect(err).To(HaveOccurred())
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**2.3 GetHistory - Empty for New Session**
```go
DescribeTable("returns empty history for new session",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-hist"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        defer getAdapter(agentName).TerminateSession(sessionID)

        history, err := getAdapter(agentName).GetHistory(sessionID)

        Expect(err).ToNot(HaveOccurred())
        Expect(history).To(BeEmpty())
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**2.4 GetHistory - Contains Sent Messages**
```go
DescribeTable("returns conversation history with sent messages",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-histmsg"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        defer getAdapter(agentName).TerminateSession(sessionID)

        // Send multiple messages
        msg1 := agent.Message{Role: agent.RoleUser, Content: "First message"}
        msg2 := agent.Message{Role: agent.RoleUser, Content: "Second message"}

        getAdapter(agentName).SendMessage(sessionID, msg1)
        time.Sleep(100 * time.Millisecond) // Allow processing
        getAdapter(agentName).SendMessage(sessionID, msg2)
        time.Sleep(100 * time.Millisecond)

        history, err := getAdapter(agentName).GetHistory(sessionID)

        Expect(err).ToNot(HaveOccurred())
        Expect(len(history)).To(BeNumerically(">=", 2))

        // Verify message content appears in history
        historyText := ""
        for _, msg := range history {
            historyText += msg.Content
        }
        Expect(historyText).To(ContainSubstring("First message"))
        Expect(historyText).To(ContainSubstring("Second message"))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**2.5 GetHistory - Message Ordering**
```go
DescribeTable("preserves message order in history",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-order"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        defer getAdapter(agentName).TerminateSession(sessionID)

        // Send messages in specific order
        messages := []string{"First", "Second", "Third"}
        for _, content := range messages {
            msg := agent.Message{Role: agent.RoleUser, Content: content}
            getAdapter(agentName).SendMessage(sessionID, msg)
            time.Sleep(50 * time.Millisecond)
        }

        history, err := getAdapter(agentName).GetHistory(sessionID)

        Expect(err).ToNot(HaveOccurred())

        // Verify chronological order
        userMessages := []string{}
        for _, msg := range history {
            if msg.Role == agent.RoleUser {
                userMessages = append(userMessages, msg.Content)
            }
        }

        Expect(userMessages).To(ContainElement("First"))
        Expect(userMessages).To(ContainElement("Second"))
        Expect(userMessages).To(ContainElement("Third"))

        // Verify First comes before Second, Second before Third
        firstIdx := indexOf(userMessages, "First")
        secondIdx := indexOf(userMessages, "Second")
        thirdIdx := indexOf(userMessages, "Third")

        Expect(firstIdx).To(BeNumerically("<", secondIdx))
        Expect(secondIdx).To(BeNumerically("<", thirdIdx))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

### 3. Data Exchange Tests

**File**: `test/integration/agent_parity/data_exchange_test.go`

#### Test Cases

**3.1 ExportConversation - JSONL Format**
```go
DescribeTable("exports conversation in JSONL format",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-export-jsonl"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        defer getAdapter(agentName).TerminateSession(sessionID)

        // Send message to create history
        msg := agent.Message{Role: agent.RoleUser, Content: "Export test"}
        getAdapter(agentName).SendMessage(sessionID, msg)
        time.Sleep(100 * time.Millisecond)

        // Export
        data, err := getAdapter(agentName).ExportConversation(sessionID, agent.FormatJSONL)

        Expect(err).ToNot(HaveOccurred())
        Expect(data).ToNot(BeEmpty())

        // Verify JSONL format (each line is valid JSON)
        lines := strings.Split(string(data), "\n")
        validLines := 0
        for _, line := range lines {
            if line != "" {
                var msg map[string]interface{}
                err := json.Unmarshal([]byte(line), &msg)
                if err == nil {
                    validLines++
                }
            }
        }
        Expect(validLines).To(BeNumerically(">=", 1))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**3.2 ExportConversation - Markdown Format**
```go
DescribeTable("exports conversation in Markdown format",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-export-md"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        defer getAdapter(agentName).TerminateSession(sessionID)

        msg := agent.Message{Role: agent.RoleUser, Content: "Markdown export test"}
        getAdapter(agentName).SendMessage(sessionID, msg)
        time.Sleep(100 * time.Millisecond)

        data, err := getAdapter(agentName).ExportConversation(sessionID, agent.FormatMarkdown)

        Expect(err).ToNot(HaveOccurred())
        Expect(data).ToNot(BeEmpty())
        Expect(string(data)).To(ContainSubstring("Markdown export test"))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**3.3 ExportConversation - HTML Format**
```go
DescribeTable("exports conversation in HTML format",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-export-html"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        defer getAdapter(agentName).TerminateSession(sessionID)

        msg := agent.Message{Role: agent.RoleUser, Content: "HTML export test"}
        getAdapter(agentName).SendMessage(sessionID, msg)
        time.Sleep(100 * time.Millisecond)

        data, err := getAdapter(agentName).ExportConversation(sessionID, agent.FormatHTML)

        // Note: ClaudeAdapter returns "not yet implemented" for HTML
        // Both agents should have same behavior
        if agentName == "claude" {
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("not yet implemented"))
        } else {
            // Gemini should also return same error OR implement HTML export
            // Either way, behavior must be consistent
            Expect(err).To(HaveOccurred())
        }
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**3.4 ImportConversation - JSONL Format**
```go
DescribeTable("imports conversation from JSONL data",
    func(agentName string) {
        // First, create and export a conversation
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-import-src"),
            WorkingDirectory: "/tmp",
        }
        origSessionID, _ := getAdapter(agentName).CreateSession(ctx)

        msg := agent.Message{Role: agent.RoleUser, Content: "Import test message"}
        getAdapter(agentName).SendMessage(origSessionID, msg)
        time.Sleep(100 * time.Millisecond)

        exportData, _ := getAdapter(agentName).ExportConversation(origSessionID, agent.FormatJSONL)
        getAdapter(agentName).TerminateSession(origSessionID)

        // Now import into new session
        newSessionID, err := getAdapter(agentName).ImportConversation(exportData, agent.FormatJSONL)

        // Note: ClaudeAdapter returns "not yet implemented"
        // Both agents should have same behavior
        if agentName == "claude" {
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("not yet implemented"))
        } else {
            // Gemini should match Claude behavior (error or implementation)
            Expect(err).To(HaveOccurred())
        }

        if newSessionID != "" {
            defer getAdapter(agentName).TerminateSession(newSessionID)
        }
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

### 4. Capabilities Tests

**File**: `test/integration/agent_parity/capabilities_test.go`

#### Test Cases

**4.1 Capabilities - Returns Valid Struct**
```go
DescribeTable("returns valid capabilities struct",
    func(agentName string) {
        caps := getAdapter(agentName).Capabilities()

        Expect(caps.ModelName).ToNot(BeEmpty())
        Expect(caps.MaxContextWindow).To(BeNumerically(">", 0))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**4.2 Capabilities - Agent-Specific Features**
```go
It("claude supports slash commands", func() {
    caps := getAdapter("claude").Capabilities()
    Expect(caps.SupportsSlashCommands).To(BeTrue())
})

It("gemini does not support slash commands (API agent)", func() {
    caps := getAdapter("gemini").Capabilities()
    Expect(caps.SupportsSlashCommands).To(BeFalse())
})
```

**4.3 Capabilities - Common Features**
```go
DescribeTable("both agents support tools",
    func(agentName string) {
        caps := getAdapter(agentName).Capabilities()
        Expect(caps.SupportsTools).To(BeTrue())
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)

DescribeTable("both agents support vision",
    func(agentName string) {
        caps := getAdapter(agentName).Capabilities()
        Expect(caps.SupportsVision).To(BeTrue())
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

### 5. Command Execution Tests

**File**: `test/integration/agent_parity/command_execution_test.go`

#### Test Cases

**5.1 ExecuteCommand - Rename Session**
```go
DescribeTable("renames session via command",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-rename"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        defer getAdapter(agentName).TerminateSession(sessionID)

        cmd := agent.Command{
            Type: agent.CommandRename,
            Params: map[string]interface{}{
                "session_id": string(sessionID),
                "name":       "new-session-name",
            },
        }

        err := getAdapter(agentName).ExecuteCommand(cmd)

        Expect(err).ToNot(HaveOccurred())
        // Verify rename (implementation-specific)
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**5.2 ExecuteCommand - Set Directory**
```go
DescribeTable("changes working directory via command",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-setdir"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        defer getAdapter(agentName).TerminateSession(sessionID)

        cmd := agent.Command{
            Type: agent.CommandSetDir,
            Params: map[string]interface{}{
                "session_id": string(sessionID),
                "path":       "~/src",
            },
        }

        err := getAdapter(agentName).ExecuteCommand(cmd)

        Expect(err).ToNot(HaveOccurred())
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**5.3 ExecuteCommand - Authorize Directory**
```go
DescribeTable("authorizes directory via command",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-auth"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        defer getAdapter(agentName).TerminateSession(sessionID)

        cmd := agent.Command{
            Type: agent.CommandAuthorize,
            Params: map[string]interface{}{
                "session_id": string(sessionID),
                "path":       "~/data",
            },
        }

        err := getAdapter(agentName).ExecuteCommand(cmd)

        // Note: ClaudeAdapter returns "not yet implemented"
        // Both should have same behavior
        if agentName == "claude" {
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("not yet implemented"))
        } else {
            Expect(err).To(HaveOccurred())
        }
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**5.4 ExecuteCommand - Run Hook**
```go
DescribeTable("executes hook via command",
    func(agentName string) {
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-hook"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)
        defer getAdapter(agentName).TerminateSession(sessionID)

        cmd := agent.Command{
            Type: agent.CommandRunHook,
            Params: map[string]interface{}{
                "session_id": string(sessionID),
                "hook_name":  "pre_message",
                "script":     "echo 'hook executed'",
            },
        }

        err := getAdapter(agentName).ExecuteCommand(cmd)

        // Both should have same behavior (implemented or not)
        if agentName == "claude" {
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("not yet implemented"))
        } else {
            Expect(err).To(HaveOccurred())
        }
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

### 6. Lifecycle Integration Tests

**File**: `test/integration/agent_parity/lifecycle_integration_test.go`

#### Test Cases

**6.1 Full Session Lifecycle**
```go
DescribeTable("completes full session lifecycle",
    func(agentName string) {
        // 1. Create
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-lifecycle"),
            WorkingDirectory: "/tmp",
            Project:          "test-project",
        }

        sessionID, err := getAdapter(agentName).CreateSession(ctx)
        Expect(err).ToNot(HaveOccurred())
        Expect(sessionID).ToNot(BeEmpty())

        // 2. Verify active
        status, _ := getAdapter(agentName).GetSessionStatus(sessionID)
        Expect(status).To(Equal(agent.StatusActive))

        // 3. Send messages
        msg1 := agent.Message{Role: agent.RoleUser, Content: "Test message 1"}
        msg2 := agent.Message{Role: agent.RoleUser, Content: "Test message 2"}

        err = getAdapter(agentName).SendMessage(sessionID, msg1)
        Expect(err).ToNot(HaveOccurred())

        time.Sleep(100 * time.Millisecond)

        err = getAdapter(agentName).SendMessage(sessionID, msg2)
        Expect(err).ToNot(HaveOccurred())

        time.Sleep(100 * time.Millisecond)

        // 4. Get history
        history, err := getAdapter(agentName).GetHistory(sessionID)
        Expect(err).ToNot(HaveOccurred())
        Expect(len(history)).To(BeNumerically(">=", 2))

        // 5. Export
        exportData, err := getAdapter(agentName).ExportConversation(sessionID, agent.FormatJSONL)
        Expect(err).ToNot(HaveOccurred())
        Expect(exportData).ToNot(BeEmpty())

        // 6. Terminate
        err = getAdapter(agentName).TerminateSession(sessionID)
        Expect(err).ToNot(HaveOccurred())

        // 7. Verify terminated
        status, _ = getAdapter(agentName).GetSessionStatus(sessionID)
        Expect(status).To(Equal(agent.StatusTerminated))
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**6.2 Resume Workflow**
```go
DescribeTable("suspends and resumes session with history preservation",
    func(agentName string) {
        // Create session with messages
        ctx := agent.SessionContext{
            Name:             testEnv.UniqueSessionName(agentName + "-resume-wf"),
            WorkingDirectory: "/tmp",
        }
        sessionID, _ := getAdapter(agentName).CreateSession(ctx)

        msg := agent.Message{Role: agent.RoleUser, Content: "Pre-suspend message"}
        getAdapter(agentName).SendMessage(sessionID, msg)
        time.Sleep(100 * time.Millisecond)

        // Get original history
        origHistory, _ := getAdapter(agentName).GetHistory(sessionID)
        origLength := len(origHistory)

        // Suspend (detach) - implementation-specific
        // For Claude: tmux detach
        // For Gemini: pause API session

        // Resume
        err := getAdapter(agentName).ResumeSession(sessionID)
        Expect(err).ToNot(HaveOccurred())

        // Verify history preserved
        resumedHistory, _ := getAdapter(agentName).GetHistory(sessionID)
        Expect(len(resumedHistory)).To(Equal(origLength))

        // Send another message
        msg2 := agent.Message{Role: agent.RoleUser, Content: "Post-resume message"}
        err = getAdapter(agentName).SendMessage(sessionID, msg2)
        Expect(err).ToNot(HaveOccurred())

        time.Sleep(100 * time.Millisecond)

        // Verify history now longer
        finalHistory, _ := getAdapter(agentName).GetHistory(sessionID)
        Expect(len(finalHistory)).To(BeNumerically(">", origLength))

        // Cleanup
        getAdapter(agentName).TerminateSession(sessionID)
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

**6.3 Concurrent Sessions**
```go
DescribeTable("manages multiple concurrent sessions independently",
    func(agentName string) {
        // Create 3 sessions
        sessions := []agent.SessionID{}
        for i := 1; i <= 3; i++ {
            ctx := agent.SessionContext{
                Name:             testEnv.UniqueSessionName(fmt.Sprintf("%s-concurrent-%d", agentName, i)),
                WorkingDirectory: "/tmp",
            }
            sessionID, _ := getAdapter(agentName).CreateSession(ctx)
            sessions = append(sessions, sessionID)
        }

        // Send different message to each session
        for i, sessionID := range sessions {
            msg := agent.Message{
                Role:    agent.RoleUser,
                Content: fmt.Sprintf("Message to session %d", i+1),
            }
            err := getAdapter(agentName).SendMessage(sessionID, msg)
            Expect(err).ToNot(HaveOccurred())
        }

        time.Sleep(200 * time.Millisecond)

        // Verify each session has only its own message
        for i, sessionID := range sessions {
            history, _ := getAdapter(agentName).GetHistory(sessionID)

            // Find user messages
            userMsgs := []string{}
            for _, msg := range history {
                if msg.Role == agent.RoleUser {
                    userMsgs = append(userMsgs, msg.Content)
                }
            }

            expectedContent := fmt.Sprintf("Message to session %d", i+1)
            Expect(userMsgs).To(ContainElement(expectedContent))
        }

        // Cleanup
        for _, sessionID := range sessions {
            getAdapter(agentName).TerminateSession(sessionID)
        }
    },
    Entry("claude agent", "claude"),
    Entry("gemini agent", "gemini"),
)
```

## Test Helpers

**File**: `test/integration/helpers/agent_helpers.go`

```go
package helpers

import (
    "github.com/vbonnet/ai-tools/agm/internal/agent"
)

// Agent adapter cache to avoid recreating adapters
var adapterCache = make(map[string]agent.Agent)

// getAdapter returns agent adapter by name, caching instances
func GetAdapter(name string) agent.Agent {
    if adapter, ok := adapterCache[name]; ok {
        return adapter
    }

    adapter, err := agent.GetAgent(name)
    if err != nil {
        panic(fmt.Sprintf("failed to get agent %s: %v", name, err))
    }

    adapterCache[name] = adapter
    return adapter
}

// Helper function to find index of string in slice
func indexOf(slice []string, item string) int {
    for i, s := range slice {
        if s == item {
            return i
        }
    }
    return -1
}
```

## Test Execution

### Running Tests

```bash
# Run all agent parity tests
cd main/agm
go test -v ./test/integration/agent_parity/...

# Run specific test suite
go test -v ./test/integration/agent_parity/ -run TestSessionManagement

# Run tests for specific agent only
go test -v ./test/integration/agent_parity/ -ginkgo.focus="claude"
go test -v ./test/integration/agent_parity/ -ginkgo.focus="gemini"

# Run with verbose output
go test -v ./test/integration/agent_parity/... -ginkgo.v
```

### Expected Results

When GeminiAdapter is fully implemented, all tests should:
- ✅ Pass for both "claude" and "gemini" Entry points
- ✅ Complete within reasonable timeouts (< 30s total)
- ✅ Clean up resources properly (no leaked sessions)
- ✅ Show consistent behavior between agents

## Coverage Metrics

### Success Criteria

1. **Test Count**: At least 25 parameterized test cases
2. **Pass Rate**: 100% for both agents
3. **Coverage**: All 11 Agent interface methods tested
4. **Execution Time**: < 30 seconds for full suite
5. **Parity**: Both agents behave identically for common features

### Current Status

- ❌ **0 tests passing** (GeminiAdapter not implemented)
- ⚠️ **Tests ready to run** once implementation exists
- ✅ **Test infrastructure complete** (Ginkgo, helpers, patterns)

## Implementation Blockers

**Cannot execute these tests until**:
1. GeminiAdapter methods implemented (9 of 11 need implementation)
2. Gemini SDK integrated (google-generativeai)
3. Session storage mechanism implemented
4. API key configuration added
5. Unit tests for GeminiAdapter passing

**Estimated implementation effort**: 8-12 hours

See TEST-ANALYSIS-REPORT.md for details.

---

**Status**: Test plan complete, awaiting GeminiAdapter implementation
**Next Step**: Implement GeminiAdapter (new bead: oss-csm-g1-implementation)
**Author**: Claude Sonnet 4.5
**Date**: 2026-02-03
