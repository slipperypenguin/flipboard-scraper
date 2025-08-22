package pkg

import (
	"testing"
	"time"
)

func TestDecodeFlipboardURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Valid redirect URL",
			input:    "https://flipboard.com/redirect?url=https%3A//example.com/article",
			expected: "https://example.com/article",
		},
		{
			name:     "URL with complex encoding",
			input:    "https://flipboard.com/redirect?url=https%3A//blog.example.com/post%3Fid%3D123",
			expected: "https://blog.example.com/post?id=123",
		},
		{
			name:     "Non-redirect Flipboard URL",
			input:    "https://flipboard.com/@user/magazine/story",
			expected: "https://flipboard.com/@user/magazine/story",
		},
		{
			name:     "Empty URL",
			input:    "",
			expected: "",
		},
		{
			name:     "Invalid URL",
			input:    "not-a-url",
			expected: "",
		},
		{
			name:     "No redirect parameter",
			input:    "https://flipboard.com/somepage",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DecodeFlipboardURL(tt.input)
			if result != tt.expected {
				t.Errorf("DecodeFlipboardURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCleanURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL with UTM parameters",
			input:    "https://example.com/article?utm_source=flipboard&utm_medium=social&id=123",
			expected: "https://example.com/article?id=123",
		},
		{
			name:     "URL with Facebook tracking",
			input:    "https://example.com/post?fbclid=abc123&content=test",
			expected: "https://example.com/post?content=test",
		},
		{
			name:     "URL with multiple tracking params",
			input:    "https://blog.com/article?utm_campaign=test&gclid=xyz&ref=social&title=hello",
			expected: "https://blog.com/article?title=hello",
		},
		{
			name:     "Clean URL without tracking",
			input:    "https://example.com/clean-article",
			expected: "https://example.com/clean-article",
		},
		{
			name:     "Invalid URL",
			input:    "not-a-url",
			expected: "not-a-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanURL(tt.input)
			if result != tt.expected {
				t.Errorf("CleanURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractSourceFromURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple domain",
			input:    "https://example.com/article",
			expected: "example.com",
		},
		{
			name:     "Subdomain",
			input:    "https://blog.example.com/post/123",
			expected: "blog.example.com",
		},
		{
			name:     "With www",
			input:    "https://www.example.com/page",
			expected: "www.example.com",
		},
		{
			name:     "With port",
			input:    "https://example.com:8080/api",
			expected: "example.com",
		},
		{
			name:     "Empty URL",
			input:    "",
			expected: "",
		},
		{
			name:     "Invalid URL",
			input:    "not-a-url",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractSourceFromURL(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractSourceFromURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestScraperConfig(t *testing.T) {
	tests := []struct {
		name   string
		config ScraperConfig
		valid  bool
	}{
		{
			name: "Valid scraper config",
			config: ScraperConfig{
				ConcurrentRequests: 2,
				RequestsPerSecond:  0.5,
				Timeout:            15 * time.Minute,
				UserAgent:          "test",
				UseJavaScript:      true,
				MaxScrolls:         150,
				ScrollDelay:        3 * time.Second,
			},
			valid: true,
		},
		{
			name: "Quick mode config",
			config: ScraperConfig{
				ConcurrentRequests: 1,
				RequestsPerSecond:  1.0,
				Timeout:            5 * time.Minute,
				UserAgent:          "test",
				UseJavaScript:      true,
				MaxScrolls:         0, // No scrolling
				ScrollDelay:        2 * time.Second,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scraper := NewMagazineScraper(tt.config)
			if scraper == nil && tt.valid {
				t.Error("Expected valid scraper for valid config")
			}
			if scraper != nil {
				// Test that scraper config fields are properly set
				if scraper.config.MaxScrolls != tt.config.MaxScrolls {
					t.Errorf("MaxScrolls not set correctly: expected %d, got %d",
						tt.config.MaxScrolls, scraper.config.MaxScrolls)
				}
				if scraper.config.ScrollDelay != tt.config.ScrollDelay {
					t.Errorf("ScrollDelay not set correctly: expected %v, got %v",
						tt.config.ScrollDelay, scraper.config.ScrollDelay)
				}
			}
		})
	}
}

func TestArticleStructure(t *testing.T) {
	article := Article{
		Title:       "Test Article",
		URL:         "https://flipboard.com/redirect?url=https%3A//example.com/article",
		ActualURL:   "https://example.com/article",
		Summary:     "This is a test article",
		Date:        time.Now(),
		Source:      "example.com",
		ScrapedFrom: "html",
	}

	// Test that all fields are accessible
	if article.Title == "" {
		t.Error("Title should not be empty")
	}
	if article.ActualURL == "" {
		t.Error("ActualURL should not be empty")
	}
	if article.Source == "" {
		t.Error("Source should not be empty")
	}
	if article.ScrapedFrom != "html" {
		t.Error("ScrapedFrom should be 'html'")
	}
}

func TestGenerateStats(t *testing.T) {
	articles := []Article{
		{
			Title:       "Article 1",
			URL:         "https://flipboard.com/story1",
			ActualURL:   "https://example.com/article1",
			Summary:     "Summary 1",
			Source:      "example.com",
			ScrapedFrom: "html",
		},
		{
			Title:       "Article 2",
			URL:         "https://flipboard.com/story2",
			ActualURL:   "https://blog.com/article2",
			Summary:     "Summary 2",
			Source:      "blog.com",
			ScrapedFrom: "html",
		},
		{
			Title:       "Article 3",
			URL:         "https://flipboard.com/story3",
			ActualURL:   "", // No decoded URL
			Summary:     "",
			Source:      "",
			ScrapedFrom: "html",
		},
	}

	stats := GenerateStats(articles)

	// Test basic counts
	if stats.TotalArticles != 3 {
		t.Errorf("Expected 3 total articles, got %d", stats.TotalArticles)
	}
	if stats.ExternalURLs != 2 {
		t.Errorf("Expected 2 external URLs, got %d", stats.ExternalURLs)
	}
	if stats.WithSummaries != 2 {
		t.Errorf("Expected 2 articles with summaries, got %d", stats.WithSummaries)
	}

	// Test source breakdown
	if stats.SourceBreakdown["example.com"] != 1 {
		t.Errorf("Expected 1 article from example.com, got %d", stats.SourceBreakdown["example.com"])
	}
	if stats.SourceBreakdown["blog.com"] != 1 {
		t.Errorf("Expected 1 article from blog.com, got %d", stats.SourceBreakdown["blog.com"])
	}

	// Test scraped from tracking
	if stats.ScrapedFrom["html"] != 3 {
		t.Errorf("Expected 3 articles scraped from html, got %d", stats.ScrapedFrom["html"])
	}
}

func TestIsValidFlipboardURL(t *testing.T) {
	scraper := NewMagazineScraper(DefaultConfig())

	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "Valid magazine URL",
			url:      "https://flipboard.com/@user/magazine",
			expected: true,
		},
		{
			name:     "Valid magazine URL with ID",
			url:      "https://flipboard.com/@user/magazine-name-123abc",
			expected: true,
		},
		{
			name:     "Valid magazine path",
			url:      "https://flipboard.com/magazine/tech-news",
			expected: true,
		},
		{
			name:     "Invalid domain",
			url:      "https://example.com/@user/magazine",
			expected: false,
		},
		{
			name:     "Invalid path",
			url:      "https://flipboard.com/homepage",
			expected: false,
		},
		{
			name:     "Invalid URL format",
			url:      "not-a-url",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scraper.isValidFlipboardURL(tt.url)
			if result != tt.expected {
				t.Errorf("isValidFlipboardURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestDeduplicateArticles(t *testing.T) {
	scraper := NewMagazineScraper(DefaultConfig())

	articles := []Article{
		{
			Title:     "Article 1",
			URL:       "https://flipboard.com/story1",
			ActualURL: "https://example.com/article1",
		},
		{
			Title:     "Article 1", // Duplicate title
			URL:       "https://flipboard.com/story1-dup",
			ActualURL: "https://example.com/article1", // Same actual URL
		},
		{
			Title:     "Article 2",
			URL:       "https://flipboard.com/story2",
			ActualURL: "https://example.com/article2",
		},
		{
			Title:     "Article 3",
			URL:       "https://flipboard.com/story3",
			ActualURL: "", // No actual URL, will use title for dedup
		},
		{
			Title:     "Article 3", // Duplicate title, no actual URL
			URL:       "https://flipboard.com/story3-dup",
			ActualURL: "",
		},
	}

	deduplicated := scraper.deduplicateArticles(articles)

	if len(deduplicated) != 3 {
		t.Errorf("Expected 3 unique articles after deduplication, got %d", len(deduplicated))
	}

	// Check that we kept the right articles
	titles := make(map[string]bool)
	actualURLs := make(map[string]bool)

	for _, article := range deduplicated {
		if article.ActualURL != "" {
			if actualURLs[article.ActualURL] {
				t.Errorf("Found duplicate actual URL: %s", article.ActualURL)
			}
			actualURLs[article.ActualURL] = true
		} else {
			if titles[article.Title] {
				t.Errorf("Found duplicate title: %s", article.Title)
			}
			titles[article.Title] = true
		}
	}
}
