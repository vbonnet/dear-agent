package tofuimport

import (
	"strings"
	"testing"
)

func TestParseInventoryAcceptsAWellFormedFleet(t *testing.T) {
	inventory, err := ParseInventory([]byte(`{"active":["engram-research","dear-agent"],"archived":["old-thing"]}`))
	if err != nil {
		t.Fatalf("ParseInventory: %v", err)
	}
	// Sorted, so the import order and therefore any partial-failure point is
	// reproducible between runs.
	if got := strings.Join(inventory.Active, ","); got != "dear-agent,engram-research" {
		t.Fatalf("active repositories not sorted: %s", got)
	}
	if got := strings.Join(inventory.Archived, ","); got != "old-thing" {
		t.Fatalf("unexpected archived repositories: %s", got)
	}
}

func TestParseInventoryRejectsUnsafeFleets(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name:    "an inventory without dear-agent has no canonical source of truth",
			raw:     `{"active":["engram-research"],"archived":[]}`,
			wantErr: "omits dear-agent",
		},
		{
			name:    "case-insensitive duplicates are one repository declared twice",
			raw:     `{"active":["dear-agent","Dear-Agent"],"archived":[]}`,
			wantErr: "duplicates or overlaps",
		},
		{
			name:    "a repository in both lists would be managed and ignored at once",
			raw:     `{"active":["dear-agent"],"archived":["DEAR-AGENT"]}`,
			wantErr: "duplicates or overlaps",
		},
		{
			// A newline splits one record into two repositories downstream.
			name:    "an embedded newline is not a valid identity segment",
			raw:     "{\"active\":[\"dear-agent\",\"a\\nb\"],\"archived\":[]}",
			wantErr: "not a valid GitHub identity segment",
		},
		{
			// A slash produces a bogus owner/name slug.
			name:    "an embedded slash is not a valid identity segment",
			raw:     `{"active":["dear-agent","owner/repo"],"archived":[]}`,
			wantErr: "not a valid GitHub identity segment",
		},
		{
			// A quote produces an invalid state address or import ID.
			name:    "an embedded quote is not a valid identity segment",
			raw:     `{"active":["dear-agent","a\"b"],"archived":[]}`,
			wantErr: "not a valid GitHub identity segment",
		},
		{
			name:    "a leading dash is not a valid identity segment",
			raw:     `{"active":["dear-agent","-lead"],"archived":[]}`,
			wantErr: "not a valid GitHub identity segment",
		},
		{
			name:    "an empty name is not a valid identity segment",
			raw:     `{"active":["dear-agent",""],"archived":[]}`,
			wantErr: "not a valid GitHub identity segment",
		},
		{
			name:    "an unexpected field means the evaluation shape changed",
			raw:     `{"active":["dear-agent"],"archived":[],"pending":[]}`,
			wantErr: "not the expected",
		},
		{
			name:    "a non-object body is not an empty fleet",
			raw:     `[]`,
			wantErr: "not the expected",
		},
		{
			name:    "an unparseable body is not an empty fleet",
			raw:     `not json`,
			wantErr: "not the expected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseInventory([]byte(tt.raw))
			if err == nil {
				t.Fatal("ParseInventory unexpectedly succeeded")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}
