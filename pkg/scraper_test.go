package pkg

import (
	"context"
	"testing"
	"time"
)

func TestCleanText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  Hello   World  ", "Hello World"},
		{"\tTest\nString\r\n", "Test String"},
		{"Normal text", "Normal text"},
		{"", ""},
	}

	for _, tt := range tests {
		result := cleanText(tt.input)
		if result != tt.expected {
			t.Errorf("cleanText(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestNewMagazineScraper(t *testing.T) {
	config := DefaultConfig()
	scraper := NewMagazineScraper(config)
	if scraper == nil {
		t.Error("NewMagazineScraper() returned nil")
	}
	if scraper.collector == nil {
		t.Error("NewMagazineScraper() returned scraper with nil collector")
	}
	if scraper.limiter == nil {
		t.Error("NewMagazineScraper() returned scraper with nil rate limiter")
	}
}

func TestScrapeURLValidation(t *testing.T) {
	scraper := NewMagazineScraper(DefaultConfig())
	ctx := context.Background()
	_, err := scraper.ScrapeURL(ctx, "http://invalid-url.com")
	if err == nil {
		t.Error("Expected error for invalid Flipboard URL")
	}
}

func TestScrapeURLsValidation(t *testing.T) {
	scraper := NewMagazineScraper(DefaultConfig())
	ctx := context.Background()

	// Test empty URLs
	_, err := scraper.ScrapeURLs(ctx, []string{})
	if err == nil {
		t.Error("Expected error for empty URL list")
	}

	// Test context cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)
	_, err = scraper.ScrapeURLs(ctx, []string{"https://flipboard.com/test"})
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	if config.ConcurrentRequests <= 0 {
		t.Error("Default ConcurrentRequests should be positive")
	}
	if config.RequestsPerSecond <= 0 {
		t.Error("Default RequestsPerSecond should be positive")
	}
	if config.Timeout <= 0 {
		t.Error("Default Timeout should be positive")
	}
}
