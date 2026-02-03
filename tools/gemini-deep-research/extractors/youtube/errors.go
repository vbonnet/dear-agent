package youtube

import "errors"

var (
	// ErrYTDLPNotFound is returned when yt-dlp is not installed
	ErrYTDLPNotFound = errors.New("yt-dlp not found")

	// ErrNoSubtitles is returned when no subtitles are available
	ErrNoSubtitles = errors.New("no subtitles available")

	// ErrInvalidURL is returned when the URL is invalid
	ErrInvalidURL = errors.New("invalid YouTube URL")

	// ErrTranscriptTooShort is returned when transcript is below minimum length
	ErrTranscriptTooShort = errors.New("transcript too short")

	// ErrTimeout is returned when the operation times out
	ErrTimeout = errors.New("operation timeout")

	// ErrPrivateVideo is returned when video is private or restricted
	ErrPrivateVideo = errors.New("video is private or restricted")
)

// ExtractionError wraps extraction errors with context
type ExtractionError struct {
	URL             string
	Operation       string
	Err             error
	Troubleshooting []string
}

func (e *ExtractionError) Error() string {
	msg := "Failed to " + e.Operation + "\n"
	msg += "  URL: " + e.URL + "\n"
	msg += "  Reason: " + e.Err.Error() + "\n"
	if len(e.Troubleshooting) > 0 {
		msg += "  Troubleshooting:\n"
		for _, tip := range e.Troubleshooting {
			msg += "    - " + tip + "\n"
		}
	}
	return msg
}

func (e *ExtractionError) Unwrap() error {
	return e.Err
}

// NewExtractionError creates a new ExtractionError
func NewExtractionError(url, operation string, err error, tips ...string) *ExtractionError {
	return &ExtractionError{
		URL:             url,
		Operation:       operation,
		Err:             err,
		Troubleshooting: tips,
	}
}
