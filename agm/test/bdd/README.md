# BDD Test Suite for AGM

This directory contains Behavior-Driven Development (BDD) tests for the AGM (Agent Generalization Manager) project. The tests verify consistent behavior across multiple agent adapters (Claude, Gemini, etc.).

## Overview

The BDD test suite uses:
- **Gherkin** for scenario definitions (human-readable test specifications)
- **godog** for test execution (Cucumber for Go)
- **Mock adapters** for fast, deterministic testing without API keys

## Directory Structure

```
test/bdd/
├── features/                    # Gherkin feature files
│   ├── session_lifecycle.feature
│   ├── conversation_persistence.feature
│   └── agent_selection.feature
├── steps/                       # Step definitions (Go code)
│   ├── setup_steps.go
│   ├── session_steps.go
│   └── conversation_steps.go
├── internal/                    # Test-only code
│   ├── adapters/mock/           # Mock adapters
│   └── testenv/                 # Test environment
├── main_test.go                 # Test suite entry point
└── README.md                    # This file
```

## Running Tests

### Run all BDD tests (local development)

```bash
cd main/agm
make test-bdd
```

### Run tests from test/bdd directory

```bash
cd test/bdd
go test -v
```

### Run specific scenarios by tag

```bash
cd test/bdd
go test -v -godog.tags=@smoke
```

### Generate JUnit XML for CI

```bash
cd test/bdd
go test -v -godog.format=junit -godog.output=junit.xml
```

## Writing New Scenarios

### Step 1: Add Gherkin scenario to a feature file

Example (`features/my_feature.feature`):

```gherkin
Feature: My Feature
  Scenario Outline: Test something
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    When I do something with "<agent>"
    Then the result should be correct

    Examples:
      | agent  |
      | claude |
      | gemini |
```

### Step 2: Implement step definitions

Create a new file in `steps/` or add to existing file:

```go
package steps

import (
    "context"
    "github.com/cucumber/godog"
    "agm/test/bdd/internal/testenv"
)

func RegisterMySteps(ctx *godog.ScenarioContext) {
    ctx.Step(`^I do something with "([^"]*)"$`, iDoSomething)
}

func iDoSomething(ctx context.Context, agent string) (context.Context, error) {
    env := testenv.EnvFromContext(ctx)
    // Implementation here
    return ctx, nil
}
```

### Step 3: Register steps in main_test.go

Add to `InitializeScenario`:

```go
steps.RegisterMySteps(ctx)
```

### Step 4: Run tests to verify

```bash
make test-bdd
```

## Adding New Agents

To add a new agent (e.g., GPT-4):

### Step 1: Create mock adapter

Create `internal/adapters/mock/gpt4.go`:

```go
package mock

type GPT4Adapter struct {
    sessions map[string]*Session
    mu       sync.RWMutex
}

func NewGPT4Adapter() *GPT4Adapter {
    return &GPT4Adapter{
        sessions: make(map[string]*Session),
    }
}

func (a *GPT4Adapter) Name() string {
    return "gpt4"
}

// Implement other Adapter interface methods...
```

### Step 2: Add to test environment

Edit `internal/testenv/environment.go`:

```go
type Environment struct {
    T            *testing.T
    ClaudeAdapter mock.Adapter
    GeminiAdapter mock.Adapter
    GPT4Adapter   mock.Adapter  // Add this
    // ...
}

func NewEnvironment(t *testing.T) *Environment {
    return &Environment{
        T:             t,
        ClaudeAdapter: mock.NewClaudeAdapter(),
        GeminiAdapter: mock.NewGeminiAdapter(),
        GPT4Adapter:   mock.NewGPT4Adapter(),  // Add this
    }
}

func (e *Environment) GetAdapter(name string) (mock.Adapter, error) {
    switch name {
    case "claude":
        return e.ClaudeAdapter, nil
    case "gemini":
        return e.GeminiAdapter, nil
    case "gpt4":                          // Add this
        return e.GPT4Adapter, nil         // Add this
    default:
        return nil, fmt.Errorf("unknown adapter: %s", name)
    }
}
```

### Step 3: Update feature files

Add "gpt4" to Examples tables:

```gherkin
Examples:
  | agent  |
  | claude |
  | gemini |
  | gpt4   |
```

That's it! The BDD tests will now run against all three agents.

## Troubleshooting

### "undefined step" errors

**Symptom:** godog reports undefined steps

**Solution:**
1. Check step regex in step definition matches Gherkin
2. Ensure step is registered in `InitializeScenario`
3. Run `go test -v` to see which steps are missing

### Tests fail with "session not found"

**Symptom:** Session-related tests fail

**Solution:**
1. Check that session is created in setup step
2. Verify `env.CurrentSession` is set correctly
3. Check that session ID is passed correctly between steps

### Import cycle errors

**Symptom:** Cannot compile due to import cycle

**Solution:**
- Keep test code in `test/bdd/` separate from production code
- Don't import production code into test mocks
- Use interfaces to break dependencies

### Tests pass locally but fail in CI

**Symptom:** CI reports failures

**Solution:**
1. Check for race conditions (run with `-race` flag)
2. Verify deterministic behavior (no random data, time.Now() issues)
3. Check CI logs for specific errors

## Performance

**Target:** <3 minutes for full test suite

**Current Performance:**
- ~8 scenarios × 2 agents = 16 scenario executions
- Mock adapters (no API calls, instant responses)
- Expected time: <30 seconds

## CI Integration

BDD tests run automatically on every pull request via GitHub Actions.

**Workflow:** `.github/workflows/test.yml`

Test results are uploaded as JUnit XML and displayed in PR checks.

## Contact

For questions or issues with BDD tests, see:
- Main README: main/agm/README.md
- Wayfinder project documentation: the git history/
