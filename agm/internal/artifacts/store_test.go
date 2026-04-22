package artifacts

import (
	"testing"
	"time"
)

func TestArtifactFields(t *testing.T) {
	now := time.Now()
	a := &Artifact{
		ID:        "art-001",
		SessionID: "sess-abc",
		Type:      "research-report",
		Path:      "/tmp/report.md",
		Size:      1024,
		Metadata:  map[string]interface{}{"key": "value"},
		CreatedAt: now,
	}

	if a.ID != "art-001" {
		t.Errorf("expected ID art-001, got %s", a.ID)
	}
	if a.SessionID != "sess-abc" {
		t.Errorf("expected SessionID sess-abc, got %s", a.SessionID)
	}
	if a.Type != "research-report" {
		t.Errorf("expected Type research-report, got %s", a.Type)
	}
	if a.Path != "/tmp/report.md" {
		t.Errorf("expected Path /tmp/report.md, got %s", a.Path)
	}
	if a.Size != 1024 {
		t.Errorf("expected Size 1024, got %d", a.Size)
	}
	if a.Metadata["key"] != "value" {
		t.Errorf("expected metadata key=value, got %v", a.Metadata["key"])
	}
	if !a.CreatedAt.Equal(now) {
		t.Errorf("expected CreatedAt %v, got %v", now, a.CreatedAt)
	}
}

func TestArtifactNilMetadata(t *testing.T) {
	a := &Artifact{
		ID:   "art-002",
		Type: "code-review",
	}

	if a.Metadata != nil {
		t.Errorf("expected nil metadata, got %v", a.Metadata)
	}
	if a.Size != 0 {
		t.Errorf("expected zero size, got %d", a.Size)
	}
	if a.Path != "" {
		t.Errorf("expected empty path, got %s", a.Path)
	}
}

func TestArtifactZeroTime(t *testing.T) {
	a := &Artifact{ID: "art-003"}
	if !a.CreatedAt.IsZero() {
		t.Errorf("expected zero CreatedAt, got %v", a.CreatedAt)
	}
}
