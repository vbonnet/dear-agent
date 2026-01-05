package output

import (
	"encoding/json"

	testerrors "github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/errors"
)

// JSONFormatter formats output as JSON
type JSONFormatter struct{}

// Format formats data as JSON
func (f *JSONFormatter) Format(data interface{}) (string, error) {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// FormatError formats an error as JSON
func (f *JSONFormatter) FormatError(err error) (string, error) {
	errorData := map[string]interface{}{
		"error": err.Error(),
	}

	switch e := err.(type) {
	case *testerrors.UserError:
		errorData["type"] = "user_error"
		errorData["title"] = e.Title
		errorData["solutions"] = e.Solutions
	case *testerrors.SystemError:
		errorData["type"] = "system_error"
		errorData["title"] = e.Title
		errorData["solutions"] = e.Solutions
	case *testerrors.TimeoutError:
		errorData["type"] = "timeout_error"
		errorData["title"] = e.Title
		errorData["solutions"] = e.Solutions
	default:
		errorData["type"] = "unknown_error"
	}

	bytes, err := json.MarshalIndent(errorData, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
