package session

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestManifestProperty_UUIDFormat verifies that Claude UUIDs always conform to RFC 4122 format
func TestManifestProperty_UUIDFormat(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Claude UUID always matches RFC 4122 format", prop.ForAll(
		func(uuid string) bool {
			// Create manifest with generated UUID
			m := &manifest.Manifest{
				SchemaVersion: "2.0",
				SessionID:     "test-session",
				Name:          "test",
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Context: manifest.Context{
					Project: "~/test",
				},
				Claude: manifest.Claude{
					UUID: uuid,
				},
				Tmux: manifest.Tmux{
					SessionName: "test",
				},
			}

			// UUID must match RFC 4122 pattern: 8-4-4-4-12 hex digits
			err := manifest.ValidateUUID(m.Claude.UUID)
			return err == nil
		},
		genValidUUID(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestManifestProperty_TimestampsParseable verifies all timestamps are always parseable
func TestManifestProperty_TimestampsParseable(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Timestamps are always parseable", prop.ForAll(
		func(createdAt, updatedAt time.Time) bool {
			m := &manifest.Manifest{
				SchemaVersion: "2.0",
				SessionID:     "test-session",
				Name:          "test",
				CreatedAt:     createdAt,
				UpdatedAt:     updatedAt,
				Context: manifest.Context{
					Project: "~/test",
				},
				Tmux: manifest.Tmux{
					SessionName: "test",
				},
			}

			// Timestamps must not be zero and must be valid
			if m.CreatedAt.IsZero() {
				return false
			}
			if m.UpdatedAt.IsZero() {
				return false
			}

			// Must be able to format and parse
			createdStr := m.CreatedAt.Format(time.RFC3339)
			updatedStr := m.UpdatedAt.Format(time.RFC3339)

			_, err1 := time.Parse(time.RFC3339, createdStr)
			_, err2 := time.Parse(time.RFC3339, updatedStr)

			return err1 == nil && err2 == nil
		},
		genValidTime(),
		genValidTime(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestManifestProperty_RequiredFieldsPresent verifies required fields are always present
func TestManifestProperty_RequiredFieldsPresent(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Required fields always present after validation", prop.ForAll(
		func(sessionID, name, project, tmuxName string) bool {
			m := &manifest.Manifest{
				SchemaVersion: "2.0",
				SessionID:     sessionID,
				Name:          name,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Context: manifest.Context{
					Project: project,
				},
				Tmux: manifest.Tmux{
					SessionName: tmuxName,
				},
			}

			// Validate should succeed only when all required fields present
			err := m.Validate()

			// If validation succeeds, all fields must be non-empty
			if err == nil {
				return m.SchemaVersion != "" &&
					m.SessionID != "" &&
					m.Name != "" &&
					m.Context.Project != "" &&
					m.Tmux.SessionName != ""
			}

			// If validation fails, at least one field must be empty
			return m.SchemaVersion == "" ||
				m.SessionID == "" ||
				m.Name == "" ||
				m.Context.Project == "" ||
				m.Tmux.SessionName == ""
		},
		genValidSessionID(),
		genValidName(),
		genValidPath(),
		genValidSessionID(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestManifestProperty_LifecycleValidation verifies lifecycle field validation
func TestManifestProperty_LifecycleValidation(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Lifecycle must be empty or 'archived'", prop.ForAll(
		func(lifecycle string) bool {
			m := &manifest.Manifest{
				SchemaVersion: "2.0",
				SessionID:     "test-session",
				Name:          "test",
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Lifecycle:     lifecycle,
				Context: manifest.Context{
					Project: "~/test",
				},
				Tmux: manifest.Tmux{
					SessionName: "test",
				},
			}

			err := m.Validate()

			// Valid only if lifecycle is empty or "archived"
			validLifecycle := lifecycle == "" || lifecycle == manifest.LifecycleArchived
			validationPassed := err == nil

			// Both should agree
			return validLifecycle == validationPassed
		},
		gen.OneGenOf(
			gen.Const(""),
			gen.Const(manifest.LifecycleArchived),
			gen.AlphaString(),
		),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestManifestProperty_TagsValidation verifies tags are validated correctly
func TestManifestProperty_TagsValidation(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Tags respect count and length limits", prop.ForAll(
		func(tags []string) bool {
			m := &manifest.Manifest{
				SchemaVersion: "2.0",
				SessionID:     "test-session",
				Name:          "test",
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Context: manifest.Context{
					Project: "~/test",
					Tags:    tags,
				},
				Tmux: manifest.Tmux{
					SessionName: "test",
				},
			}

			err := m.Validate()

			// Check if tags violate constraints
			tooManyTags := len(tags) > manifest.MaxTagsCount
			tagTooLong := false
			for _, tag := range tags {
				if len([]rune(tag)) > manifest.MaxTagLen {
					tagTooLong = true
					break
				}
			}

			hasConstraintViolation := tooManyTags || tagTooLong
			validationFailed := err != nil

			// Validation should fail if and only if constraints are violated
			return hasConstraintViolation == validationFailed
		},
		gen.SliceOf(gen.AlphaString()),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestManifestProperty_PurposeLength verifies purpose length validation
func TestManifestProperty_PurposeLength(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("Purpose respects max length", prop.ForAll(
		func(purpose string) bool {
			m := &manifest.Manifest{
				SchemaVersion: "2.0",
				SessionID:     "test-session",
				Name:          "test",
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Context: manifest.Context{
					Project: "~/test",
					Purpose: purpose,
				},
				Tmux: manifest.Tmux{
					SessionName: "test",
				},
			}

			err := m.Validate()

			purposeTooLong := len([]rune(purpose)) > manifest.MaxPurposeLen
			validationFailed := err != nil

			// Validation should fail if and only if purpose is too long
			return purposeTooLong == validationFailed
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Generator functions

// genValidUUID generates RFC 4122 compliant UUIDs
func genValidUUID() gopter.Gen {
	return gen.RegexMatch("^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$")
}

// genValidTime generates valid timestamps
func genValidTime() gopter.Gen {
	startTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2030, 12, 31, 23, 59, 59, 0, time.UTC)
	duration := endTime.Sub(startTime)

	return gen.TimeRange(startTime, duration)
}

// genValidSessionID generates valid session IDs
func genValidSessionID() gopter.Gen {
	return gen.RegexMatch("^[a-zA-Z0-9_.-]{1,100}$")
}

// genValidName generates valid session names
func genValidName() gopter.Gen {
	return gen.Identifier()
}

// genValidPath generates valid file paths
func genValidPath() gopter.Gen {
	return gen.OneConstOf("~/test", "/tmp/workspace", "~/projects/test")
}
