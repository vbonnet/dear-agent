package temporal

import (
	"strings"
	"testing"
)

// TestNewTemporalClient verifies that NewTemporalClient creates a valid client
func TestNewTemporalClient(t *testing.T) {
	client := NewTemporalClient()
	if client == nil {
		t.Fatal("NewTemporalClient returned nil")
	}
	if client.sessions == nil {
		t.Fatal("TemporalClient.sessions is nil")
	}
}

// TestHasSession verifies session existence checking
func TestHasSession(t *testing.T) {
	client := NewTemporalClient()

	// Test non-existent session
	exists, err := client.HasSession("test-session")
	if err != nil {
		t.Fatalf("HasSession returned error: %v", err)
	}
	if exists {
		t.Error("HasSession returned true for non-existent session")
	}

	// Create a session
	err = client.CreateSession("test-session", "/tmp")
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	// Test existing session
	exists, err = client.HasSession("test-session")
	if err != nil {
		t.Fatalf("HasSession returned error: %v", err)
	}
	if !exists {
		t.Error("HasSession returned false for existing session")
	}
}

// TestListSessions verifies listing all sessions
func TestListSessions(t *testing.T) {
	client := NewTemporalClient()

	// Test empty list
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(sessions))
	}

	// Create multiple sessions
	err = client.CreateSession("session1", "/tmp/1")
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	err = client.CreateSession("session2", "/tmp/2")
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	// Test non-empty list
	sessions, err = client.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessions))
	}
}

// TestListSessionsWithInfo verifies listing sessions with details
func TestListSessionsWithInfo(t *testing.T) {
	client := NewTemporalClient()

	// Test empty list
	sessions, err := client.ListSessionsWithInfo()
	if err != nil {
		t.Fatalf("ListSessionsWithInfo returned error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(sessions))
	}

	// Create a session
	err = client.CreateSession("test-session", "/tmp")
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	// Test session info
	sessions, err = client.ListSessionsWithInfo()
	if err != nil {
		t.Fatalf("ListSessionsWithInfo returned error: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Name != "test-session" {
		t.Errorf("Expected session name 'test-session', got '%s'", sessions[0].Name)
	}
	if sessions[0].AttachedClients != 0 {
		t.Errorf("Expected 0 attached clients, got %d", sessions[0].AttachedClients)
	}
}

// TestListClients verifies listing clients for a session
func TestListClients(t *testing.T) {
	client := NewTemporalClient()

	// Test non-existent session
	clients, err := client.ListClients("non-existent")
	if err != nil {
		t.Fatalf("ListClients returned error: %v", err)
	}
	if len(clients) != 0 {
		t.Errorf("Expected 0 clients for non-existent session, got %d", len(clients))
	}

	// Create a session
	err = client.CreateSession("test-session", "/tmp")
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	// Test existing session with no clients
	clients, err = client.ListClients("test-session")
	if err != nil {
		t.Fatalf("ListClients returned error: %v", err)
	}
	if len(clients) != 0 {
		t.Errorf("Expected 0 clients, got %d", len(clients))
	}
}

// TestCreateSession verifies session creation
func TestCreateSession(t *testing.T) {
	client := NewTemporalClient()

	err := client.CreateSession("test-session", "/tmp/workdir")
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	// Verify session exists
	exists, err := client.HasSession("test-session")
	if err != nil {
		t.Fatalf("HasSession returned error: %v", err)
	}
	if !exists {
		t.Error("Session was not created")
	}

	// Verify session state
	state := client.sessions["test-session"]
	if state == nil {
		t.Fatal("Session state is nil")
	}
	if state.name != "test-session" {
		t.Errorf("Expected session name 'test-session', got '%s'", state.name)
	}
	if state.workdir != "/tmp/workdir" {
		t.Errorf("Expected workdir '/tmp/workdir', got '%s'", state.workdir)
	}
	if state.attached {
		t.Error("Session should not be attached initially")
	}
}

// TestAttachSession verifies session attachment
func TestAttachSession(t *testing.T) {
	client := NewTemporalClient()

	// Test attaching to non-existent session
	err := client.AttachSession("non-existent")
	if err != nil {
		t.Fatalf("AttachSession returned error: %v", err)
	}

	// Create and attach to session
	err = client.CreateSession("test-session", "/tmp")
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	err = client.AttachSession("test-session")
	if err != nil {
		t.Fatalf("AttachSession returned error: %v", err)
	}

	// Verify attached state
	state := client.sessions["test-session"]
	if !state.attached {
		t.Error("Session should be attached")
	}
}

// TestSendKeys verifies sending keys to a session
func TestSendKeys(t *testing.T) {
	client := NewTemporalClient()

	err := client.SendKeys("test-session", "echo hello")
	if err != nil {
		t.Fatalf("SendKeys returned error: %v", err)
	}
}

// TestValidateSessionName verifies session name validation
func TestValidateSessionName(t *testing.T) {
	tests := []struct {
		name      string
		wantError bool
	}{
		{"valid-session", false},
		{"valid_session", false},
		{"valid123", false},
		{"", true},
		{"invalid session", true},
		{"invalid\tsession", true},
		{"invalid\nsession", true},
		{"invalid\rsession", true},
	}

	for _, tt := range tests {
		err := ValidateSessionName(tt.name)
		if tt.wantError && err == nil {
			t.Errorf("ValidateSessionName(%q) should return error", tt.name)
		}
		if !tt.wantError && err != nil {
			t.Errorf("ValidateSessionName(%q) returned unexpected error: %v", tt.name, err)
		}
	}
}

// TestValidateWorkdir verifies working directory validation
func TestValidateWorkdir(t *testing.T) {
	tests := []struct {
		workdir   string
		wantError bool
	}{
		{"/tmp", false},
		{"/home/user", false},
		{"", true},
	}

	for _, tt := range tests {
		err := ValidateWorkdir(tt.workdir)
		if tt.wantError && err == nil {
			t.Errorf("ValidateWorkdir(%q) should return error", tt.workdir)
		}
		if !tt.wantError && err != nil {
			t.Errorf("ValidateWorkdir(%q) returned unexpected error: %v", tt.workdir, err)
		}
	}
}

// TestFormatSessionInfo verifies session info formatting
func TestFormatSessionInfo(t *testing.T) {
	info := SessionInfo{
		Name:            "test-session",
		AttachedClients: 2,
		AttachedList:    "client1,client2",
	}

	result := FormatSessionInfo(info)
	if !strings.Contains(result, "test-session") {
		t.Error("Formatted string should contain session name")
	}
	if !strings.Contains(result, "2") {
		t.Error("Formatted string should contain client count")
	}
	if !strings.Contains(result, "client1,client2") {
		t.Error("Formatted string should contain attached list")
	}
}

// TestFormatClientInfo verifies client info formatting
func TestFormatClientInfo(t *testing.T) {
	info := ClientInfo{
		SessionName: "test-session",
		TTY:         "/dev/pts/1",
		PID:         12345,
	}

	result := FormatClientInfo(info)
	if !strings.Contains(result, "test-session") {
		t.Error("Formatted string should contain session name")
	}
	if !strings.Contains(result, "/dev/pts/1") {
		t.Error("Formatted string should contain TTY")
	}
	if !strings.Contains(result, "12345") {
		t.Error("Formatted string should contain PID")
	}
}

// TestSessionExists verifies the convenience function
func TestSessionExists(t *testing.T) {
	client := NewTemporalClient()

	// Test non-existent session
	if SessionExists(client, "non-existent") {
		t.Error("SessionExists returned true for non-existent session")
	}

	// Create session
	err := client.CreateSession("test-session", "/tmp")
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	// Test existing session
	if !SessionExists(client, "test-session") {
		t.Error("SessionExists returned false for existing session")
	}
}

// TestGetSessionCount verifies session counting
func TestGetSessionCount(t *testing.T) {
	client := NewTemporalClient()

	// Test empty
	count, err := GetSessionCount(client)
	if err != nil {
		t.Fatalf("GetSessionCount returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 sessions, got %d", count)
	}

	// Create sessions
	client.CreateSession("session1", "/tmp/1")
	client.CreateSession("session2", "/tmp/2")

	count, err = GetSessionCount(client)
	if err != nil {
		t.Fatalf("GetSessionCount returned error: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 sessions, got %d", count)
	}
}

// TestGetClientCount verifies client counting
func TestGetClientCount(t *testing.T) {
	client := NewTemporalClient()

	// Test non-existent session
	count, err := GetClientCount(client, "non-existent")
	if err != nil {
		t.Fatalf("GetClientCount returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 clients, got %d", count)
	}

	// Create session
	client.CreateSession("test-session", "/tmp")

	count, err = GetClientCount(client, "test-session")
	if err != nil {
		t.Fatalf("GetClientCount returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 clients, got %d", count)
	}
}

// TestFindSessionByName verifies session lookup
func TestFindSessionByName(t *testing.T) {
	client := NewTemporalClient()

	// Test non-existent session
	session, err := FindSessionByName(client, "non-existent")
	if err == nil {
		t.Error("FindSessionByName should return error for non-existent session")
	}
	if session != nil {
		t.Error("FindSessionByName should return nil for non-existent session")
	}

	// Create session
	client.CreateSession("test-session", "/tmp")

	// Test existing session
	session, err = FindSessionByName(client, "test-session")
	if err != nil {
		t.Fatalf("FindSessionByName returned error: %v", err)
	}
	if session == nil {
		t.Fatal("FindSessionByName returned nil for existing session")
	}
	if session.Name != "test-session" {
		t.Errorf("Expected session name 'test-session', got '%s'", session.Name)
	}
}

// TestGetAllSessionInfo verifies retrieving all session info
func TestGetAllSessionInfo(t *testing.T) {
	client := NewTemporalClient()

	// Test empty
	sessions, err := GetAllSessionInfo(client)
	if err != nil {
		t.Fatalf("GetAllSessionInfo returned error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(sessions))
	}

	// Create sessions
	client.CreateSession("session1", "/tmp/1")
	client.CreateSession("session2", "/tmp/2")

	sessions, err = GetAllSessionInfo(client)
	if err != nil {
		t.Fatalf("GetAllSessionInfo returned error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessions))
	}
}

// TestSessionInfoStruct verifies SessionInfo struct
func TestSessionInfoStruct(t *testing.T) {
	info := SessionInfo{
		Name:            "test",
		AttachedClients: 5,
		AttachedList:    "a,b,c",
	}

	if info.Name != "test" {
		t.Errorf("Expected Name 'test', got '%s'", info.Name)
	}
	if info.AttachedClients != 5 {
		t.Errorf("Expected AttachedClients 5, got %d", info.AttachedClients)
	}
	if info.AttachedList != "a,b,c" {
		t.Errorf("Expected AttachedList 'a,b,c', got '%s'", info.AttachedList)
	}
}

// TestClientInfoStruct verifies ClientInfo struct
func TestClientInfoStruct(t *testing.T) {
	info := ClientInfo{
		SessionName: "test-session",
		TTY:         "/dev/pts/1",
		PID:         12345,
	}

	if info.SessionName != "test-session" {
		t.Errorf("Expected SessionName 'test-session', got '%s'", info.SessionName)
	}
	if info.TTY != "/dev/pts/1" {
		t.Errorf("Expected TTY '/dev/pts/1', got '%s'", info.TTY)
	}
	if info.PID != 12345 {
		t.Errorf("Expected PID 12345, got %d", info.PID)
	}
}
