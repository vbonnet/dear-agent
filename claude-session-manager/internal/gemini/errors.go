package gemini

import "fmt"

// UserError represents user input errors (exit code 1)
type UserError struct {
	Message string
	Usage   string
}

func (e *UserError) Error() string {
	if e.Usage != "" {
		return fmt.Sprintf("%s\n\n%s", e.Message, e.Usage)
	}
	return e.Message
}

// APIError represents Gemini API errors (exit code 2)
type APIError struct {
	Message string
	Cause   error
}

func (e *APIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}
