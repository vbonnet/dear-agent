package cache

import "testing"

func TestNormalizeYouTube(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "StandardYouTubeURL",
			input:    "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			expected: "https://youtube.com/watch?v=dQw4w9WgXcQ",
			wantErr:  false,
		},
		{
			name:     "YouTubeWithTrackingParams",
			input:    "https://youtube.com/watch?v=dQw4w9WgXcQ&utm_source=twitter&utm_medium=social",
			expected: "https://youtube.com/watch?v=dQw4w9WgXcQ",
			wantErr:  false,
		},
		{
			name:     "YouTubeShortURL",
			input:    "https://youtu.be/dQw4w9WgXcQ",
			expected: "https://youtube.com/watch?v=dQw4w9WgXcQ",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Normalize(tt.input, "youtube")
			if (err != nil) != tt.wantErr {
				t.Errorf("Normalize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("Normalize() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestNormalizeArxiv(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "ArxivWithVersion",
			input:    "https://arxiv.org/abs/2601.20802v1",
			expected: "https://arxiv.org/abs/2601.20802",
			wantErr:  false,
		},
		{
			name:     "ArxivWithoutVersion",
			input:    "https://arxiv.org/abs/2601.20802",
			expected: "https://arxiv.org/abs/2601.20802",
			wantErr:  false,
		},
		{
			name:     "ArxivWithDifferentVersion",
			input:    "https://arxiv.org/abs/2601.20802v2",
			expected: "https://arxiv.org/abs/2601.20802",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Normalize(tt.input, "arxiv")
			if (err != nil) != tt.wantErr {
				t.Errorf("Normalize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("Normalize() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestNormalizeGeneric(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "HTTPSWithWWW",
			input:    "https://www.example.com/article",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "WithTrailingSlash",
			input:    "https://example.com/article/",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "WithUTMParams",
			input:    "https://example.com/article?utm_source=google&utm_medium=cpc",
			expected: "https://example.com/article",
			wantErr:  false,
		},
		{
			name:     "MixedCase",
			input:    "HTTPS://WWW.Example.COM/Article",
			expected: "https://example.com/Article",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Normalize(tt.input, "article")
			if (err != nil) != tt.wantErr {
				t.Errorf("Normalize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("Normalize() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
