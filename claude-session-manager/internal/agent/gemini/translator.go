// Package gemini provides Gemini adapter implementation for the Agent interface.
//
// This package contains the CommandTranslator which maps generic agent.Command
// types to Gemini-specific operations. The Gemini adapter differs from Claude
// in that it is API-based rather than CLI-based, so commands translate to API
// operations or local state updates rather than slash commands.
package gemini

import (
	"github.com/user/ai-tools/agm/internal/agent"
)

// CommandTranslator translates generic agent.Command to Gemini-specific operations.
//
// The translator is stateless and can be reused across multiple translations.
type CommandTranslator struct {
	// Stateless - no fields needed
}

// NewCommandTranslator creates a new CommandTranslator.
func NewCommandTranslator() *CommandTranslator {
	return &CommandTranslator{}
}

// Translate translates an agent.Command to a GeminiOperation.
//
// Supported command types:
//   - agent.CommandRename: Translates to RenameSessionOperation
//   - agent.CommandSetDir: Translates to SetDirectoryOperation
//
// Returns ErrUnsupportedCommand if the command type is not supported.
// Returns ParameterError if command parameters are missing or invalid.
//
// Example:
//
//	translator := NewCommandTranslator()
//	cmd := agent.Command{
//	    Type:   agent.CommandRename,
//	    Params: map[string]interface{}{"name": "new-session"},
//	}
//	op, err := translator.Translate(cmd)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// op is *RenameSessionOperation{NewName: "new-session"}
func (t *CommandTranslator) Translate(cmd agent.Command) (GeminiOperation, error) {
	switch cmd.Type {
	case agent.CommandRename:
		return t.translateRename(cmd)
	case agent.CommandSetDir:
		return t.translateSetDir(cmd)
	default:
		return nil, ErrUnsupportedCommand
	}
}

// translateRename translates a rename command to RenameSessionOperation.
func (t *CommandTranslator) translateRename(cmd agent.Command) (*RenameSessionOperation, error) {
	name, ok := cmd.Params["name"]
	if !ok {
		return nil, &ParameterError{
			CommandType:   cmd.Type,
			ParameterName: "name",
			Issue:         "missing",
		}
	}

	nameStr, ok := name.(string)
	if !ok {
		return nil, &ParameterError{
			CommandType:   cmd.Type,
			ParameterName: "name",
			Issue:         "must be a string",
		}
	}

	if nameStr == "" {
		return nil, &ParameterError{
			CommandType:   cmd.Type,
			ParameterName: "name",
			Issue:         "cannot be empty",
		}
	}

	return &RenameSessionOperation{NewName: nameStr}, nil
}

// translateSetDir translates a set directory command to SetDirectoryOperation.
func (t *CommandTranslator) translateSetDir(cmd agent.Command) (*SetDirectoryOperation, error) {
	path, ok := cmd.Params["path"]
	if !ok {
		return nil, &ParameterError{
			CommandType:   cmd.Type,
			ParameterName: "path",
			Issue:         "missing",
		}
	}

	pathStr, ok := path.(string)
	if !ok {
		return nil, &ParameterError{
			CommandType:   cmd.Type,
			ParameterName: "path",
			Issue:         "must be a string",
		}
	}

	if pathStr == "" {
		return nil, &ParameterError{
			CommandType:   cmd.Type,
			ParameterName: "path",
			Issue:         "cannot be empty",
		}
	}

	return &SetDirectoryOperation{Path: pathStr}, nil
}
