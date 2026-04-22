package agents

import (
	"reflect"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// TestSelectorProperty_AlwaysReturnsValidAgent verifies selector never returns nil/empty
func TestSelectorProperty_AlwaysReturnsValidAgent(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("SelectHarness always returns non-empty agent", prop.ForAll(
		func(sessionName string, config *HarnessConfig) bool {
			agent := SelectHarness(sessionName, config)

			// Agent must never be empty
			return agent != ""
		},
		gen.AlphaString(),
		genHarnessConfig(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestSelectorProperty_RespectsDefaultHarness verifies default agent is used when no match
func TestSelectorProperty_RespectsDefaultHarness(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("SelectHarness returns default when no keyword matches", prop.ForAll(
		func(sessionName, defaultAgent string) bool {
			// Config with no preferences (no keywords can match)
			config := &HarnessConfig{
				DefaultHarness: defaultAgent,
				Preferences:    []Preference{},
			}

			agent := SelectHarness(sessionName, config)

			// Should always return default agent when no preferences
			return agent == defaultAgent
		},
		gen.AlphaString(),
		gen.OneConstOf("claude", "gemini", "gpt4"),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestSelectorProperty_KeywordMatching verifies keyword matching behavior
func TestSelectorProperty_KeywordMatching(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("SelectHarness matches keywords case-insensitively", prop.ForAll(
		func(keyword, agentName string) bool {
			// Config with one preference
			config := &HarnessConfig{
				DefaultHarness: "default-agent",
				Preferences: []Preference{
					{
						Keywords: []string{keyword},
						Harness:  agentName,
					},
				},
			}

			// Test uppercase version of keyword
			upperSessionName := "TEST-" + keyword + "-SESSION"
			upperResult := SelectHarness(upperSessionName, config)

			// Test lowercase version of keyword
			lowerSessionName := "test-" + keyword + "-session"
			lowerResult := SelectHarness(lowerSessionName, config)

			// Both should return the same agent (case-insensitive matching)
			return upperResult == lowerResult && upperResult == agentName
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.OneConstOf("claude", "gemini", "gpt4"),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestSelectorProperty_FirstMatchWins verifies first matching preference is selected
func TestSelectorProperty_FirstMatchWins(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("SelectHarness returns first matching preference", prop.ForAll(
		func(keyword string) bool {
			// Config with two preferences matching same keyword
			config := &HarnessConfig{
				DefaultHarness: "default-agent",
				Preferences: []Preference{
					{
						Keywords: []string{keyword},
						Harness:  "first-agent",
					},
					{
						Keywords: []string{keyword},
						Harness:  "second-agent",
					},
				},
			}

			sessionName := "test-" + keyword + "-session"
			agent := SelectHarness(sessionName, config)

			// First matching preference should win
			return agent == "first-agent"
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestSelectorProperty_EmptySessionName verifies empty session name handling
func TestSelectorProperty_EmptySessionName(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("SelectHarness returns default for empty session name", prop.ForAll(
		func(config *HarnessConfig) bool {
			agent := SelectHarness("", config)

			// Empty session name should return default agent
			return agent == config.DefaultHarness
		},
		genHarnessConfig(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestSelectorProperty_Deterministic verifies deterministic behavior
func TestSelectorProperty_Deterministic(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("SelectHarness is deterministic (same input → same output)", prop.ForAll(
		func(sessionName string, config *HarnessConfig) bool {
			// Call selector twice with same inputs
			agent1 := SelectHarness(sessionName, config)
			agent2 := SelectHarness(sessionName, config)

			// Results must be identical
			return agent1 == agent2
		},
		gen.AlphaString(),
		genHarnessConfig(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestSelectorProperty_SubstringMatching verifies substring matching works
func TestSelectorProperty_SubstringMatching(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("SelectHarness matches keyword as substring", prop.ForAll(
		func(keyword, prefix, suffix string) bool {
			config := &HarnessConfig{
				DefaultHarness: "default-agent",
				Preferences: []Preference{
					{
						Keywords: []string{keyword},
						Harness:  "matched-agent",
					},
				},
			}

			// Session name contains keyword as substring
			sessionName := prefix + keyword + suffix
			agent := SelectHarness(sessionName, config)

			// Should match if keyword is non-empty
			if keyword == "" {
				return agent == config.DefaultHarness
			}
			return agent == "matched-agent"
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.AlphaString(),
		gen.AlphaString(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// TestSelectorProperty_MultipleKeywords verifies multiple keywords in one preference
func TestSelectorProperty_MultipleKeywords(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("SelectHarness matches any keyword in preference", prop.ForAll(
		func(keyword1, keyword2 string) bool {
			config := &HarnessConfig{
				DefaultHarness: "default-agent",
				Preferences: []Preference{
					{
						Keywords: []string{keyword1, keyword2},
						Harness:  "multi-keyword-agent",
					},
				},
			}

			// Test session names with each keyword
			sessionName1 := "test-" + keyword1
			sessionName2 := "test-" + keyword2

			agent1 := SelectHarness(sessionName1, config)
			agent2 := SelectHarness(sessionName2, config)

			// Both should match if keywords are non-empty
			if keyword1 == "" && keyword2 == "" {
				return agent1 == config.DefaultHarness && agent2 == config.DefaultHarness
			}
			if keyword1 == "" {
				return agent2 == "multi-keyword-agent"
			}
			if keyword2 == "" {
				return agent1 == "multi-keyword-agent"
			}

			return agent1 == "multi-keyword-agent" && agent2 == "multi-keyword-agent"
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Generator functions

// genHarnessConfig generates random HarnessConfig instances
func genHarnessConfig() gopter.Gen {
	return gen.Struct(reflect.TypeOf(HarnessConfig{}), map[string]gopter.Gen{
		"DefaultHarness": gen.OneConstOf("claude", "gemini", "gpt4", "default"),
		"Preferences":    genPreferences(),
	}).Map(func(config HarnessConfig) *HarnessConfig {
		return &config
	})
}

// genPreferences generates random Preference slices
func genPreferences() gopter.Gen {
	return gen.SliceOf(genPreference())
}

// genPreference generates random Preference instances
func genPreference() gopter.Gen {
	return gen.Struct(reflect.TypeOf(Preference{}), map[string]gopter.Gen{
		"Keywords": gen.SliceOf(gen.AlphaString().SuchThat(func(s string) bool { return s != "" })),
		"Harness":  gen.OneConstOf("claude", "gemini", "gpt4", "custom-agent"),
	})
}
