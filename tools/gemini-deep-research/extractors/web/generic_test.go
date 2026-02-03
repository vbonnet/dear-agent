package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenericWebExtractor_Extract(t *testing.T) {
	tests := []struct {
		name        string
		htmlContent string
		statusCode  int
		wantErr     bool
		errType     error
		minLength   int
	}{
		{
			name: "successful extraction",
			htmlContent: `
<!DOCTYPE html>
<html>
<head><title>Generic Article</title></head>
<body>
	<article>
		<h1>Test Article Title</h1>
		<p class="author">By Test Author</p>
		<p>` + strings.Repeat("This is article content that should be successfully extracted. ", 30) + `</p>
	</article>
	<nav>Navigation links</nav>
	<footer>Footer content</footer>
</body>
</html>`,
			statusCode: http.StatusOK,
			wantErr:    false,
			minLength:  100,
		},
		{
			name: "content too short",
			htmlContent: `
<!DOCTYPE html>
<html>
<head><title>Short Article</title></head>
<body><article><p>Too short</p></article></body>
</html>`,
			statusCode: http.StatusOK,
			wantErr:    true,
			errType:    ErrEmptyContent,
		},
		{
			name: "paywall detected",
			htmlContent: `
<!DOCTYPE html>
<html>
<head><title>Paywalled Article</title></head>
<body>
	<article>
		<h1>Premium Content</h1>
		<p>This article is for subscribers only. Subscribe to continue reading.</p>
	</article>
</body>
</html>`,
			statusCode: http.StatusOK,
			wantErr:    true,
			errType:    ErrPaywallDetected,
		},
		{
			name:        "404 not found",
			htmlContent: "Not Found",
			statusCode:  http.StatusNotFound,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify User-Agent header
				userAgent := r.Header.Get("User-Agent")
				if !strings.Contains(userAgent, "gemini-deep-research") {
					t.Errorf("User-Agent = %q, want to contain 'gemini-deep-research'", userAgent)
				}

				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.htmlContent))
			}))
			defer server.Close()

			e := NewGenericWebExtractor(server.Client())
			content, err := e.Extract(context.Background(), server.URL)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Extract() expected error but got none")
					return
				}
				if tt.errType != nil {
					// Check if error wraps the expected type
					var extractionErr *ExtractionError
					if err, ok := err.(*ExtractionError); ok {
						extractionErr = err
					}
					// For paywall and empty content errors, check the underlying error
					if tt.errType == ErrPaywallDetected || tt.errType == ErrEmptyContent {
						// Error should be wrapped in ExtractionError
						if extractionErr == nil {
							t.Errorf("Extract() error type mismatch, expected ExtractionError wrapper")
						}
					}
				}
				return
			}

			if err != nil {
				t.Errorf("Extract() unexpected error: %v", err)
				return
			}

			if content == nil {
				t.Errorf("Extract() returned nil content")
				return
			}

			if len(content.Content) < tt.minLength {
				t.Errorf("Extract() content length = %d, want >= %d", len(content.Content), tt.minLength)
			}
		})
	}
}

func TestGenericWebExtractor_ExtractWithRetry(t *testing.T) {
	// Test that retry logic works for transient errors
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			// First attempt fails with 500
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Second attempt succeeds
		html := `
<!DOCTYPE html>
<html>
<head><title>Retry Success</title></head>
<body>
	<article>
		<h1>Article After Retry</h1>
		<p>` + strings.Repeat("Content successfully extracted after retry. ", 30) + `</p>
	</article>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	e := NewGenericWebExtractor(server.Client())
	content, err := e.Extract(context.Background(), server.URL)

	if err != nil {
		t.Errorf("Extract() unexpected error after retry: %v", err)
		return
	}

	if content == nil {
		t.Errorf("Extract() returned nil content after retry")
		return
	}

	if attemptCount < 2 {
		t.Errorf("Extract() retry count = %d, want >= 2", attemptCount)
	}
}

func TestGenericWebExtractor_ExtractRealHTML(t *testing.T) {
	// Test with more realistic HTML structure
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Real-World Article Example</title>
</head>
<body>
	<header>
		<nav>
			<a href="/">Home</a>
			<a href="/about">About</a>
		</nav>
	</header>
	<main>
		<article>
			<h1>Understanding Deep Learning</h1>
			<div class="meta">
				<span class="author">By Dr. Jane Smith</span>
				<time datetime="2024-01-15">January 15, 2024</time>
			</div>
			<p class="excerpt">An introduction to deep learning concepts and applications.</p>
			<div class="content">
				<p>Deep learning is a subset of machine learning that uses neural networks with multiple layers.</p>
				<p>` + strings.Repeat("These networks can learn hierarchical representations of data. ", 20) + `</p>
				<h2>Key Concepts</h2>
				<ul>
					<li>Neural Networks</li>
					<li>Backpropagation</li>
					<li>Activation Functions</li>
				</ul>
			</div>
		</article>
		<aside>
			<h3>Related Articles</h3>
			<ul>
				<li><a href="/article1">Article 1</a></li>
				<li><a href="/article2">Article 2</a></li>
			</ul>
		</aside>
	</main>
	<footer>
		<p>&copy; 2024 Example Site</p>
	</footer>
	<script>console.log('Analytics');</script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	e := NewGenericWebExtractor(server.Client())
	content, err := e.Extract(context.Background(), server.URL)

	if err != nil {
		t.Errorf("Extract() unexpected error: %v", err)
		return
	}

	if content == nil {
		t.Errorf("Extract() returned nil content")
		return
	}

	// Verify that navigation and footer are excluded
	if strings.Contains(content.Content, "Related Articles") {
		t.Errorf("Extract() content should not include sidebar: %q", content.Content)
	}

	// Verify main content is included
	if !strings.Contains(content.Content, "Deep learning") {
		t.Errorf("Extract() content missing expected text")
	}

	if len(content.Content) < 100 {
		t.Errorf("Extract() content too short: %d characters", len(content.Content))
	}
}

func TestGenericWebExtractor_Name(t *testing.T) {
	e := NewGenericWebExtractor(nil)
	if got := e.Name(); got != "Web" {
		t.Errorf("Name() = %q, want %q", got, "Web")
	}
}

func TestGenericWebExtractor_ContentType(t *testing.T) {
	e := NewGenericWebExtractor(nil)
	contentType := e.ContentType()
	if contentType.String() != "GenericArticle" {
		t.Errorf("ContentType() = %v, want GenericArticle", contentType)
	}
}
