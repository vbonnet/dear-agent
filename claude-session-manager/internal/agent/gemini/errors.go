package gemini

import (
	"errors"
	"fmt"

	"github.com/user/ai-tools/agm/internal/agent"
)

// ErrUnsupportedCommand indicates that a command type is not supported by the Gemini adapter.
var ErrUnsupportedCommand = errors.New("command not supported by Gemini adapter")

// ParameterError indicates that a command parameter is missing or invalid.
type ParameterError struct {
	CommandType   agent.CommandType
	ParameterName string
	Issue         string
}

// Error implements the error interface.
func (e *ParameterError) Error() string {
	return fmt.Sprintf("command %s: parameter '%s': %s",
		e.CommandType, e.ParameterName, e.Issue)
}
