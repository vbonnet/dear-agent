package extractors

import (
	"context"
	"fmt"
	"net/http"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/extractors/arxiv"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/extractors/web"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/extractors/youtube"
)

// ExtractorFactory creates and manages extractors for different content types
type ExtractorFactory struct {
	youtube     *youtubeExtractorWrapper
	arxiv       *arxivExtractorWrapper
	huggingface *huggingfaceExtractorWrapper
	generic     *genericExtractorWrapper
	httpClient  *http.Client
}

// NewExtractorFactory creates a new ExtractorFactory
// Initializes all extractors with default configuration
func NewExtractorFactory() (*ExtractorFactory, error) {
	// Create YouTube extractor
	ytExtractor, err := youtube.NewExtractor()
	if err != nil {
		return nil, fmt.Errorf("initialize YouTube extractor: %w", err)
	}

	// Create HTTP client for web extractors
	httpClient := &http.Client{}

	// Create Arxiv extractor
	arxivExtractor := arxiv.NewExtractor()

	// Create HuggingFace extractor
	hfExtractor := web.NewHuggingFaceExtractor(httpClient)

	// Create Generic web extractor
	genericExtractor := web.NewGenericWebExtractor(httpClient)

	return &ExtractorFactory{
		youtube:     &youtubeExtractorWrapper{inner: ytExtractor},
		arxiv:       &arxivExtractorWrapper{inner: arxivExtractor},
		huggingface: &huggingfaceExtractorWrapper{inner: hfExtractor},
		generic:     &genericExtractorWrapper{inner: genericExtractor},
		httpClient:  httpClient,
	}, nil
}

// GetExtractor returns an appropriate extractor for the given content type
// Returns an error if the content type is not supported
func (f *ExtractorFactory) GetExtractor(contentType detector.ContentType) (Extractor, error) {
	switch contentType {
	case detector.ContentTypeVideo:
		return f.youtube, nil
	case detector.ContentTypeArxiv:
		return f.arxiv, nil
	case detector.ContentTypeHuggingFace:
		return f.huggingface, nil
	case detector.ContentTypeArticle:
		return f.generic, nil
	default:
		return nil, fmt.Errorf("unsupported content type: %s", contentType.String())
	}
}

// Wrapper types to adapt existing extractors to the unified Extractor interface

// youtubeExtractorWrapper wraps youtube.YouTubeExtractor
type youtubeExtractorWrapper struct {
	inner *youtube.YouTubeExtractor
}

func (w *youtubeExtractorWrapper) Extract(ctx context.Context, url string) (*Content, error) {
	content, err := w.inner.Extract(ctx, url)
	if err != nil {
		return nil, err
	}

	return &Content{
		Raw: content.Transcript,
		Metadata: map[string]interface{}{
			"url":      content.URL,
			"video_id": content.VideoID,
			"length":   content.Length,
		},
	}, nil
}

func (w *youtubeExtractorWrapper) Name() string {
	return w.inner.Name()
}

func (w *youtubeExtractorWrapper) ContentType() detector.ContentType {
	return w.inner.ContentType()
}

// arxivExtractorWrapper wraps arxiv.Extractor
type arxivExtractorWrapper struct {
	inner *arxiv.Extractor
}

func (w *arxivExtractorWrapper) Extract(ctx context.Context, url string) (*Content, error) {
	content, err := w.inner.Extract(ctx, url)
	if err != nil {
		return nil, err
	}

	// Format arxiv content as plain text
	formattedContent := content.Format()

	return &Content{
		Raw: formattedContent,
		Metadata: map[string]interface{}{
			"title":           content.Title,
			"authors":         content.Authors,
			"abstract":        content.Abstract,
			"published_date":  content.PublishedDate,
			"extraction_mode": content.ExtractionMode,
		},
	}, nil
}

func (w *arxivExtractorWrapper) Name() string {
	return w.inner.Name()
}

func (w *arxivExtractorWrapper) ContentType() detector.ContentType {
	return w.inner.ContentType()
}

// huggingfaceExtractorWrapper wraps web.HuggingFaceExtractor
type huggingfaceExtractorWrapper struct {
	inner *web.HuggingFaceExtractor
}

func (w *huggingfaceExtractorWrapper) Extract(ctx context.Context, url string) (*Content, error) {
	content, err := w.inner.Extract(ctx, url)
	if err != nil {
		return nil, err
	}

	return &Content{
		Raw: content.Content,
		Metadata: map[string]interface{}{
			"title":       content.Title,
			"authors":     content.Authors,
			"abstract":    content.Abstract,
			"upvotes":     content.Upvotes,
			"collections": content.Collections,
		},
	}, nil
}

func (w *huggingfaceExtractorWrapper) Name() string {
	return w.inner.Name()
}

func (w *huggingfaceExtractorWrapper) ContentType() detector.ContentType {
	return w.inner.ContentType()
}

// genericExtractorWrapper wraps web.GenericWebExtractor
type genericExtractorWrapper struct {
	inner *web.GenericWebExtractor
}

func (w *genericExtractorWrapper) Extract(ctx context.Context, url string) (*Content, error) {
	content, err := w.inner.Extract(ctx, url)
	if err != nil {
		return nil, err
	}

	return &Content{
		Raw: content.Content,
		Metadata: map[string]interface{}{
			"title":   content.Title,
			"authors": content.Authors,
		},
	}, nil
}

func (w *genericExtractorWrapper) Name() string {
	return w.inner.Name()
}

func (w *genericExtractorWrapper) ContentType() detector.ContentType {
	return w.inner.ContentType()
}
