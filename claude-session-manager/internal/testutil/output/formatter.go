package output

// Formatter formats output data for display
type Formatter interface {
	Format(data interface{}) (string, error)
	FormatError(err error) (string, error)
}

// Format returns a formatter based on the format type
func Format(format string) Formatter {
	switch format {
	case "json":
		return &JSONFormatter{}
	case "text":
		fallthrough
	default:
		return &TextFormatter{}
	}
}
