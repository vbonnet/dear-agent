package ui

import "testing"

func TestCleanupConfirmationDescription_ListsEachSelectedTarget(t *testing.T) {
	archive := []string{`"shared-name" [ID: "stopped-id"]`}
	remove := []string{
		`"shared-name" [ID: "archive-a"]`,
		`"shared-name" [ID: "archive-b"]`,
	}
	want := "Archive 1 session:\n" +
		"  - \"shared-name\" [ID: \"stopped-id\"]\n" +
		"Delete 2 sessions:\n" +
		"  - \"shared-name\" [ID: \"archive-a\"]\n" +
		"  - \"shared-name\" [ID: \"archive-b\"]\n" +
		"\nThis action cannot be undone for deleted sessions."
	if got := cleanupConfirmationDescription(archive, remove); got != want {
		t.Errorf("confirmation description = %q, want %q", got, want)
	}
}
