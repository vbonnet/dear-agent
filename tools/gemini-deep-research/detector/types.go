package detector

// ContentType represents the type of content to extract
type ContentType int

const (
	// ContentTypeVideo represents video content (YouTube, Vimeo)
	ContentTypeVideo ContentType = iota
	// ContentTypeArxiv represents arxiv academic papers
	ContentTypeArxiv
	// ContentTypeHuggingFace represents HuggingFace papers
	ContentTypeHuggingFace
	// ContentTypeArticle represents generic web articles (fallback)
	ContentTypeArticle
)

// String returns the string representation of ContentType
func (c ContentType) String() string {
	switch c {
	case ContentTypeVideo:
		return "Video"
	case ContentTypeArxiv:
		return "ArxivPaper"
	case ContentTypeHuggingFace:
		return "HuggingFacePaper"
	case ContentTypeArticle:
		return "GenericArticle"
	default:
		return "Unknown"
	}
}

// ParseContentType converts a string to ContentType
// Valid values: "video", "article", "arxiv", "huggingface"
func ParseContentType(s string) (ContentType, bool) {
	switch s {
	case "video":
		return ContentTypeVideo, true
	case "article":
		return ContentTypeArticle, true
	case "arxiv":
		return ContentTypeArxiv, true
	case "huggingface":
		return ContentTypeHuggingFace, true
	default:
		return ContentTypeArticle, false
	}
}
