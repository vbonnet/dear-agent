package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadabilityParser_ExtractArticle(t *testing.T) {
	tests := []struct {
		name        string
		htmlContent string
		wantErr     bool
		errType     error
		minLength   int
		wantTitle   string
	}{
		{
			name: "valid article",
			htmlContent: `
<!DOCTYPE html>
<html>
<head><title>Test Article</title></head>
<body>
	<article>
		<h1>Test Article</h1>
		<p>This is a test article with sufficient content to be extracted successfully. ` +
				strings.Repeat("Lorem ipsum dolor sit amet. ", 20) + `</p>
	</article>
</body>
</html>`,
			wantErr:   false,
			minLength: 100,
			wantTitle: "Test Article",
		},
		{
			name: "empty content",
			htmlContent: `
<!DOCTYPE html>
<html>
<head><title>Empty</title></head>
<body><p>Short</p></body>
</html>`,
			wantErr: true,
			errType: ErrEmptyContent,
		},
		{
			name: "paywall detected",
			htmlContent: `
<!DOCTYPE html>
<html>
<head><title>Paywall Article</title></head>
<body>
	<article>
		<h1>Paywall Article</h1>
		<p>This article requires a subscription. Subscribe to continue reading this premium content. ` +
				strings.Repeat("More paywall content here. ", 20) + `</p>
	</article>
</body>
</html>`,
			wantErr: true,
			errType: ErrPaywallDetected,
		},
		{
			name: "article with HTML entities",
			htmlContent: `
<!DOCTYPE html>
<html>
<head><title>Entities Test</title></head>
<body>
	<article>
		<h1>Test &amp; Entities</h1>
		<p>Content with &lt;tags&gt; and &quot;quotes&quot; and &nbsp; spaces. ` +
				strings.Repeat("More content here. ", 20) + `</p>
	</article>
</body>
</html>`,
			wantErr:   false,
			minLength: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.htmlContent))
			}))
			defer server.Close()

			parser := NewReadabilityParser(server.Client())
			content, err := parser.ExtractArticle(context.Background(), server.URL)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ExtractArticle() expected error but got none")
					return
				}
				if tt.errType != nil && err != tt.errType {
					t.Errorf("ExtractArticle() error = %v, want %v", err, tt.errType)
				}
				return
			}

			if err != nil {
				t.Errorf("ExtractArticle() unexpected error: %v", err)
				return
			}

			if content == nil {
				t.Errorf("ExtractArticle() returned nil content")
				return
			}

			if len(content.Content) < tt.minLength {
				t.Errorf("ExtractArticle() content length = %d, want >= %d", len(content.Content), tt.minLength)
			}

			if tt.wantTitle != "" && !strings.Contains(content.Title, tt.wantTitle) {
				t.Errorf("ExtractArticle() title = %q, want to contain %q", content.Title, tt.wantTitle)
			}
		})
	}
}

func TestReadabilityParser_HTTPErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "404 not found",
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
		{
			name:       "500 server error",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
		{
			name:       "200 OK",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == http.StatusOK {
					w.Write([]byte(`
<!DOCTYPE html>
<html>
<head><title>Test</title></head>
<body><article><p>` + strings.Repeat("Content here. ", 30) + `</p></article></body>
</html>`))
				}
			}))
			defer server.Close()

			parser := NewReadabilityParser(server.Client())
			_, err := parser.ExtractArticle(context.Background(), server.URL)

			if tt.wantErr && err == nil {
				t.Errorf("ExtractArticle() expected error for status %d", tt.statusCode)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ExtractArticle() unexpected error: %v", err)
			}
		})
	}
}

func TestHTMLToPlainText(t *testing.T) {
	tests := []struct {
		name  string
		html  string
		want  string
	}{
		{
			name: "simple HTML",
			html: "<p>Hello <strong>world</strong></p>",
			want: "Hello world",
		},
		{
			name: "nested tags",
			html: "<div><p>Nested <span>content</span> here</p></div>",
			want: "Nested content here",
		},
		{
			name: "multiple spaces",
			html: "<p>Multiple     spaces    here</p>",
			want: "Multiple spaces here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlToPlainText(tt.html)
			if got != tt.want {
				t.Errorf("htmlToPlainText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeHTMLEntities(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ampersand",
			input: "Fish &amp; Chips",
			want:  "Fish & Chips",
		},
		{
			name:  "quotes",
			input: "&quot;Hello&quot;",
			want:  "\"Hello\"",
		},
		{
			name:  "non-breaking space",
			input: "Word&nbsp;Space",
			want:  "Word Space",
		},
		{
			name:  "mdash",
			input: "Range&mdash;here",
			want:  "Range—here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeHTMLEntities(tt.input)
			if got != tt.want {
				t.Errorf("decodeHTMLEntities() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectPaywall(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "no paywall",
			content: "This is regular article content",
			want:    false,
		},
		{
			name:    "subscribe indicator",
			content: "Subscribe to continue reading this article",
			want:    true,
		},
		{
			name:    "login required",
			content: "Please sign in to continue reading",
			want:    true,
		},
		{
			name:    "premium content",
			content: "This is premium content for members only",
			want:    true,
		},
		{
			name:    "case insensitive",
			content: "SUBSCRIPTION REQUIRED to view this article",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectPaywall(tt.content)
			if got != tt.want {
				t.Errorf("detectPaywall() = %v, want %v", got, tt.want)
			}
		})
	}
}
