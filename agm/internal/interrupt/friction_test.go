package interrupt

import (
	"testing"
)

func TestCheckFriction_FirstInterruptIsFree(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	level := CheckFriction("new-session")
	if level != FrictionFree {
		t.Errorf("Expected FrictionFree for first interrupt, got %d", level)
	}
}

func TestCheckFriction_SecondRequiresReason(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Log one interrupt
	err := LogInterrupt(&AuditEntry{
		Sender:    "test",
		Recipient: "target-session",
		FlagUsed:  "emergency-interrupt",
	})
	if err != nil {
		t.Fatalf("LogInterrupt failed: %v", err)
	}

	level := CheckFriction("target-session")
	if level != FrictionReason {
		t.Errorf("Expected FrictionReason after 1 interrupt, got %d", level)
	}
}

func TestCheckFriction_ThirdIsWarn(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Log two interrupts
	for i := 0; i < 2; i++ {
		err := LogInterrupt(&AuditEntry{
			Sender:    "test",
			Recipient: "target-session",
			FlagUsed:  "emergency-interrupt",
		})
		if err != nil {
			t.Fatalf("LogInterrupt %d failed: %v", i, err)
		}
	}

	level := CheckFriction("target-session")
	if level != FrictionWarn {
		t.Errorf("Expected FrictionWarn after 2 interrupts, got %d", level)
	}
}

func TestEnforceFriction_FreePassesWithoutReason(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := EnforceFriction("new-session", "")
	if err != nil {
		t.Errorf("Expected no error for first interrupt, got: %v", err)
	}
}

func TestEnforceFriction_SecondFailsWithoutReason(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	_ = LogInterrupt(&AuditEntry{Sender: "test", Recipient: "s1", FlagUsed: "emergency-interrupt"})

	err := EnforceFriction("s1", "")
	if err == nil {
		t.Fatal("Expected error for second interrupt without reason")
	}
}

func TestEnforceFriction_SecondPassesWithReason(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	_ = LogInterrupt(&AuditEntry{Sender: "test", Recipient: "s1", FlagUsed: "emergency-interrupt"})

	err := EnforceFriction("s1", "context window full")
	if err != nil {
		t.Errorf("Expected no error with reason provided, got: %v", err)
	}
}

func TestEnforceFriction_ThirdPassesWithReason(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	for i := 0; i < 2; i++ {
		_ = LogInterrupt(&AuditEntry{Sender: "test", Recipient: "s1", FlagUsed: "emergency-interrupt"})
	}

	err := EnforceFriction("s1", "still an emergency")
	if err != nil {
		t.Errorf("Expected no error for third interrupt with reason, got: %v", err)
	}
}

func TestEnforceFriction_ThirdFailsWithoutReason(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	for i := 0; i < 2; i++ {
		_ = LogInterrupt(&AuditEntry{Sender: "test", Recipient: "s1", FlagUsed: "emergency-interrupt"})
	}

	err := EnforceFriction("s1", "")
	if err == nil {
		t.Fatal("Expected error for third interrupt without reason")
	}
}

func TestEnforceFriction_FifthPassesWithReason(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	for i := 0; i < 4; i++ {
		_ = LogInterrupt(&AuditEntry{Sender: "test", Recipient: "s1", FlagUsed: "emergency-interrupt"})
	}

	err := EnforceFriction("s1", "critical issue persists")
	if err != nil {
		t.Errorf("Expected no error for 5th interrupt with reason, got: %v", err)
	}
}

func TestFriction_IsolatedPerRecipient(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer t.Setenv("HOME", tmpDir)

	// Interrupt session-a twice
	for i := 0; i < 2; i++ {
		_ = LogInterrupt(&AuditEntry{Sender: "test", Recipient: "session-a", FlagUsed: "emergency-interrupt"})
	}

	// session-b should still be free
	level := CheckFriction("session-b")
	if level != FrictionFree {
		t.Errorf("Expected FrictionFree for session-b, got %d", level)
	}
}
