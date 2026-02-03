package youtube

// Content represents extracted YouTube content
type Content struct {
	// Transcript is the plain text transcript
	Transcript string

	// URL is the original YouTube URL
	URL string

	// VideoID is the extracted video ID
	VideoID string

	// Length is the character count of the transcript
	Length int
}

// NewContent creates a new Content instance
func NewContent(url, videoID, transcript string) *Content {
	return &Content{
		URL:        url,
		VideoID:    videoID,
		Transcript: transcript,
		Length:     len(transcript),
	}
}
