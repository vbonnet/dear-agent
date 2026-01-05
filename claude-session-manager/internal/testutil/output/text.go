package output

import (
	"fmt"
	"strings"

	testerrors "github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/errors"
)

// TextFormatter formats output as human-readable text
type TextFormatter struct{}

// Format formats data as text
func (f *TextFormatter) Format(data interface{}) (string, error) {
	// Use default string representation for simple data
	// Specific formatters can override this for complex types
	return fmt.Sprintf("%v", data), nil
}

// FormatError formats an error as human-readable text
func (f *TextFormatter) FormatError(err error) (string, error) {
	var sb strings.Builder

	switch e := err.(type) {
	case *testerrors.UserError:
		sb.WriteString("Error: ")
		sb.WriteString(e.Title)
		sb.WriteString("\n")
		sb.WriteString(e.Message)
		sb.WriteString("\n")
		if len(e.Solutions) > 0 {
			sb.WriteString("\nSolutions:\n")
			for _, solution := range e.Solutions {
				sb.WriteString("  - ")
				sb.WriteString(solution)
				sb.WriteString("\n")
			}
		}
	case *testerrors.SystemError:
		sb.WriteString("System Error: ")
		sb.WriteString(e.Title)
		sb.WriteString("\n")
		if e.Cause != nil {
			sb.WriteString("Cause: ")
			sb.WriteString(e.Cause.Error())
			sb.WriteString("\n")
		}
		if len(e.Solutions) > 0 {
			sb.WriteString("\nSolutions:\n")
			for _, solution := range e.Solutions {
				sb.WriteString("  - ")
				sb.WriteString(solution)
				sb.WriteString("\n")
			}
		}
	case *testerrors.TimeoutError:
		sb.WriteString("Timeout Error: ")
		sb.WriteString(e.Title)
		sb.WriteString("\n")
		if e.Cause != nil {
			sb.WriteString("Cause: ")
			sb.WriteString(e.Cause.Error())
			sb.WriteString("\n")
		}
		if len(e.Solutions) > 0 {
			sb.WriteString("\nSolutions:\n")
			for _, solution := range e.Solutions {
				sb.WriteString("  - ")
				sb.WriteString(solution)
				sb.WriteString("\n")
			}
		}
	default:
		sb.WriteString("Error: ")
		sb.WriteString(err.Error())
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
