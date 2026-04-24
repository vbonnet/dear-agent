package monitoring

import (
	"context"
	"regexp"
	"time"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// OutputParser parses sub-agent output to detect command execution
type OutputParser struct {
	agentID  string
	eventBus *eventbus.LocalBus
	patterns []CommandPattern
}

// CommandPattern defines a pattern to match in output
type CommandPattern struct {
	Name        string
	Regex       *regexp.Regexp
	EventType   string
	ExtractData func(matches []string) map[string]interface{}
}

// NewOutputParser creates a new output parser with default patterns
func NewOutputParser(agentID string, bus *eventbus.LocalBus) *OutputParser {
	op := &OutputParser{
		agentID:  agentID,
		eventBus: bus,
	}

	op.patterns = DefaultCommandPatterns()
	return op
}

// ParseLine parses a single line of output
func (op *OutputParser) ParseLine(line string) {
	for _, pattern := range op.patterns {
		if matches := pattern.Regex.FindStringSubmatch(line); matches != nil {
			// Extract data using pattern's function
			data := pattern.ExtractData(matches)
			if data == nil {
				data = make(map[string]interface{})
			}
			data["agent_id"] = op.agentID
			data["timestamp"] = time.Now().Format(time.RFC3339)
			data["raw_output"] = line

			// Publish event
			op.eventBus.Publish(context.Background(), &eventbus.Event{
				Type:      pattern.EventType,
				Source:    "output-parser",
				Data:      data,
			})

			// Only match first pattern
			break
		}
	}
}

// AddPattern adds a custom pattern
func (op *OutputParser) AddPattern(pattern CommandPattern) {
	op.patterns = append(op.patterns, pattern)
}

// DefaultCommandPatterns returns default patterns for common test frameworks
func DefaultCommandPatterns() []CommandPattern {
	return []CommandPattern{
		// Go test start
		{
			Name:      "go_test_start",
			Regex:     regexp.MustCompile(`go test`),
			EventType: EventTestStarted,
			ExtractData: func(matches []string) map[string]interface{} {
				return map[string]interface{}{
					"command": matches[0],
				}
			},
		},

		// Go test package result
		{
			Name:      "go_test_result",
			Regex:     regexp.MustCompile(`^(ok|FAIL)\s+(\S+)\s+([\d.]+s)`),
			EventType: EventTestPassed, // Will be EventTestFailed if FAIL
			ExtractData: func(matches []string) map[string]interface{} {
				return map[string]interface{}{
					"status":   matches[1],
					"package":  matches[2],
					"duration": matches[3],
				}
			},
		},

		// npm test start
		{
			Name:      "npm_test_start",
			Regex:     regexp.MustCompile(`npm test`),
			EventType: EventTestStarted,
			ExtractData: func(matches []string) map[string]interface{} {
				return map[string]interface{}{
					"command": matches[0],
				}
			},
		},

		// npm/Jest test summary
		{
			Name:      "npm_test_summary",
			Regex:     regexp.MustCompile(`Tests:\s+(\d+) passed(?:,\s+(\d+) failed)?,\s+(\d+) total`),
			EventType: EventTestPassed,
			ExtractData: func(matches []string) map[string]interface{} {
				data := map[string]interface{}{
					"passed": matches[1],
					"total":  matches[3],
				}
				if matches[2] != "" {
					data["failed"] = matches[2]
				}
				return data
			},
		},

		// pytest start
		{
			Name:      "pytest_start",
			Regex:     regexp.MustCompile(`pytest`),
			EventType: EventTestStarted,
			ExtractData: func(matches []string) map[string]interface{} {
				return map[string]interface{}{
					"command": matches[0],
				}
			},
		},

		// pytest summary
		{
			Name:      "pytest_summary",
			Regex:     regexp.MustCompile(`=+\s+(\d+)\s+passed.*in\s+([\d.]+)s`),
			EventType: EventTestPassed,
			ExtractData: func(matches []string) map[string]interface{} {
				return map[string]interface{}{
					"passed":   matches[1],
					"duration": matches[2] + "s",
				}
			},
		},

		// Git commit command
		{
			Name:      "git_commit_cli",
			Regex:     regexp.MustCompile(`git commit`),
			EventType: "sub_agent.command.git_commit",
			ExtractData: func(matches []string) map[string]interface{} {
				return map[string]interface{}{
					"command": matches[0],
				}
			},
		},

		// Git push command
		{
			Name:      "git_push",
			Regex:     regexp.MustCompile(`git push`),
			EventType: "sub_agent.command.git_push",
			ExtractData: func(matches []string) map[string]interface{} {
				return map[string]interface{}{
					"command": matches[0],
				}
			},
		},

		// Make build
		{
			Name:      "make_build",
			Regex:     regexp.MustCompile(`^make\s+build`),
			EventType: "sub_agent.command.build",
			ExtractData: func(matches []string) map[string]interface{} {
				return map[string]interface{}{
					"command": matches[0],
				}
			},
		},
	}
}
