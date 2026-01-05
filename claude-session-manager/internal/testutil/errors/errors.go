package errors

// UserError represents an error caused by invalid user input
type UserError struct {
	Title     string
	Message   string
	Solutions []string
}

func (e *UserError) Error() string {
	return e.Message
}

// SystemError represents an error caused by system/infrastructure issues
type SystemError struct {
	Title     string
	Cause     error
	Solutions []string
}

func (e *SystemError) Error() string {
	if e.Cause != nil {
		return e.Title + ": " + e.Cause.Error()
	}
	return e.Title
}

// TimeoutError represents a timeout error
type TimeoutError struct {
	Title     string
	Cause     error
	Solutions []string
}

func (e *TimeoutError) Error() string {
	if e.Cause != nil {
		return e.Title + ": " + e.Cause.Error()
	}
	return e.Title
}

// ExitCode returns the appropriate exit code for an error
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	switch err.(type) {
	case *UserError:
		return 3
	case *TimeoutError:
		return 2
	case *SystemError:
		return 1
	default:
		return 1
	}
}

// NewUserError creates a new UserError
func NewUserError(title, message string, solutions []string) *UserError {
	return &UserError{
		Title:     title,
		Message:   message,
		Solutions: solutions,
	}
}

// NewSystemError creates a new SystemError
func NewSystemError(title string, cause error, solutions []string) *SystemError {
	return &SystemError{
		Title:     title,
		Cause:     cause,
		Solutions: solutions,
	}
}

// NewTimeoutError creates a new TimeoutError
func NewTimeoutError(title string, cause error, solutions []string) *TimeoutError {
	return &TimeoutError{
		Title:     title,
		Cause:     cause,
		Solutions: solutions,
	}
}
