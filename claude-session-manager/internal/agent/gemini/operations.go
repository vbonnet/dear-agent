package gemini

// GeminiOperation represents a Gemini-specific operation that can be executed.
//
// Operations are created by CommandTranslator and executed by GeminiAdapter.
type GeminiOperation interface {
	// Type returns the operation type identifier.
	Type() string

	// Execute performs the operation using the provided GeminiAdapter.
	// This method is a placeholder for future implementation.
	Execute(adapter *GeminiAdapter) error
}

// RenameSessionOperation represents a session rename operation.
type RenameSessionOperation struct {
	// NewName is the new name for the session.
	NewName string
}

// Type returns the operation type identifier.
func (op *RenameSessionOperation) Type() string {
	return "rename_session"
}

// Execute performs the rename operation.
// This is a placeholder implementation for future work.
func (op *RenameSessionOperation) Execute(adapter *GeminiAdapter) error {
	// Implementation will be added when GeminiAdapter is created
	return nil
}

// SetDirectoryOperation represents a working directory change operation.
type SetDirectoryOperation struct {
	// Path is the new working directory path.
	Path string
}

// Type returns the operation type identifier.
func (op *SetDirectoryOperation) Type() string {
	return "set_directory"
}

// Execute performs the directory change operation.
// This is a placeholder implementation for future work.
func (op *SetDirectoryOperation) Execute(adapter *GeminiAdapter) error {
	// Implementation will be added when GeminiAdapter is created
	return nil
}

// GeminiAdapter is a placeholder type for future implementation.
// It will implement the agent.Agent interface.
type GeminiAdapter struct {
	// Fields will be added when adapter is implemented
}
