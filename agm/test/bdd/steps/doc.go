// Package steps holds the godog step definitions that make AGM's SPEC
// invariants executable. Each feature file under test/bdd/features/ has a
// matching <name>_steps.go that registers its steps via a Register*Steps
// function wired into RegisterScenarioDefinitions in main_test.go.
package steps
