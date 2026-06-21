package beads

import (
	"context"
	"strings"
	"testing"
)

func TestCreateArgs(t *testing.T) {
	t.Run("with db", func(t *testing.T) {
		got := strings.Join(createArgs("/path/.beads", "my task"), "\x00")
		want := strings.Join([]string{
			"--db", "/path/.beads", "create", "my task",
			"--priority", "P1", "--type", "task", "--silent",
		}, "\x00")
		if got != want {
			t.Errorf("createArgs = %q, want %q", got, want)
		}
	})
	t.Run("without db omits --db", func(t *testing.T) {
		got := createArgs("", "my task")
		for _, a := range got {
			if a == "--db" {
				t.Fatalf("expected no --db when db empty, got %v", got)
			}
		}
		if got[0] != "create" {
			t.Errorf("first arg = %q, want create", got[0])
		}
	})
	t.Run("silent flag present so stdout is just the id", func(t *testing.T) {
		joined := strings.Join(createArgs("db", "t"), " ")
		if !strings.Contains(joined, "--silent") {
			t.Errorf("createArgs missing --silent: %q", joined)
		}
	})
}

func TestDefaultDB_EnvOverride(t *testing.T) {
	t.Setenv("WAYFINDER_BEADS_DB", "/tmp/override/.beads")
	if got := DefaultDB(); got != "/tmp/override/.beads" {
		t.Errorf("DefaultDB = %q, want override", got)
	}
	t.Setenv("WAYFINDER_BEADS_DB", "")
	got := DefaultDB()
	if got == "" || !strings.HasSuffix(got, "beads/context-engine/.beads") {
		t.Errorf("DefaultDB = %q, want path ending in beads/context-engine/.beads", got)
	}
}

func TestCreate_EmptyTitle(t *testing.T) {
	// Empty title is rejected before bd is ever invoked.
	if _, err := Create(context.Background(), "/unused", "  "); err == nil {
		t.Fatal("expected error for empty title")
	}
}
