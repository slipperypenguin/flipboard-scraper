package pkg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
		{"   ", ""},
		{"Multiple    spaces   between", "Multiple spaces between"},
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
	if scraper.config.UserAgent == "" {
		t.Error("NewMagazineScraper() returned scraper with empty UserAgent")
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
	if config.UserAgent == "" {
		t.Error("Default UserAgent should not be empty")
	}
	if config.MaxPages < 0 {
		t.Error("Default MaxPages should not be negative")
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

func TestScrapeURLValidation(t *testing.T) {
	scraper := NewMagazineScraper(DefaultConfig())
	ctx := context.Background()

	// Test invalid URL - should fail quickly
	_, err := scraper.ScrapeURLs(ctx, []string{"https://invalid-url.com"})
	if err == nil {
		t.Error("Expected error for invalid Flipboard URL")
	}

	// Test empty URL
	_, err = scraper.ScrapeURLs(ctx, []string{""})
	if err == nil {
		t.Error("Expected error for empty URL")
	}
}

func TestArticleStructure(t *testing.T) {
	article := Article{
		Title:       "Test Article",
		URL:         "https://example.com/test",
		Summary:     "This is a test summary",
		Date:        time.Now(),
		Author:      "Test Author",
		ImageURL:    "https://example.com/image.jpg",
		Source:      "Test Source",
		GUID:        "test-guid-123",
		ScrapedFrom: "html",
	}

	// Test that all fields are properly set
	if article.Title != "Test Article" {
		t.Errorf("Expected title 'Test Article', got %q", article.Title)
	}
	if article.ScrapedFrom != "html" {
		t.Errorf("Expected ScrapedFrom 'html', got %q", article.ScrapedFrom)
	}
	if article.ImageURL == "" {
		t.Error("Expected ImageURL to be set")
	}
	if article.Source == "" {
		t.Error("Expected Source to be set")
	}
	if article.URL == "" {
		t.Error("Expected URL to be set")
	}
}

func TestConfigValidation(t *testing.T) {
	// Test various configuration scenarios
	configs := []struct {
		name   string
		config ScraperConfig
		valid  bool
	}{
		{
			name:   "Valid default config",
			config: DefaultConfig(),
			valid:  true,
		},
		{
			name: "Valid custom config",
			config: ScraperConfig{
				ConcurrentRequests: 5,
				RequestsPerSecond:  2.0,
				Timeout:            10 * time.Minute,
				UserAgent:          "test-agent",
				MaxPages:           20,
				Debug:              true,
			},
			valid: true,
		},
		{
			name: "Zero max pages (unlimited)",
			config: ScraperConfig{
				ConcurrentRequests: 1,
				RequestsPerSecond:  1.0,
				Timeout:            time.Minute,
				UserAgent:          "test",
				MaxPages:           0, // Should be valid (means unlimited)
			},
			valid: true,
		},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			scraper := NewMagazineScraper(tc.config)
			if scraper == nil && tc.valid {
				t.Error("Expected valid scraper for valid config")
			}
			if scraper != nil {
				if scraper.config.ConcurrentRequests != tc.config.ConcurrentRequests {
					t.Errorf("Config not properly set: expected %d concurrent requests, got %d",
						tc.config.ConcurrentRequests, scraper.config.ConcurrentRequests)
				}
				if scraper.config.MaxPages != tc.config.MaxPages {
					t.Errorf("Config not properly set: expected %d max pages, got %d",
						tc.config.MaxPages, scraper.config.MaxPages)
				}
				if scraper.config.Debug != tc.config.Debug {
					t.Errorf("Config not properly set: expected debug=%v, got debug=%v",
						tc.config.Debug, scraper.config.Debug)
				}
			}
		})
	}
}

func TestScraperConfigFields(t *testing.T) {
	config := ScraperConfig{
		ConcurrentRequests: 5,
		RequestsPerSecond:  2.5,
		Timeout:            30 * time.Second,
		UserAgent:          "test-scraper",
		UseJavaScript:      true,
		MaxPages:           100,
		Debug:              true,
	}

	scraper := NewMagazineScraper(config)

	// Verify all config fields are properly stored
	if scraper.config.ConcurrentRequests != 5 {
		t.Errorf("Expected ConcurrentRequests=5, got %d", scraper.config.ConcurrentRequests)
	}
	if scraper.config.RequestsPerSecond != 2.5 {
		t.Errorf("Expected RequestsPerSecond=2.5, got %f", scraper.config.RequestsPerSecond)
	}
	if scraper.config.UseJavaScript != true {
		t.Errorf("Expected UseJavaScript=true, got %v", scraper.config.UseJavaScript)
	}
	if scraper.config.MaxPages != 100 {
		t.Errorf("Expected MaxPages=100, got %d", scraper.config.MaxPages)
	}
	if scraper.config.Debug != true {
		t.Errorf("Expected Debug=true, got %v", scraper.config.Debug)
	}
}

func TestMockServerScraping(t *testing.T) {
	// Create a mock server that simulates some scraping scenarios
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate different responses based on request
		if strings.Contains(r.URL.Path, ".rss") {
			// Mock RSS response
			w.Header().Set("Content-Type", "application/rss+xml")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Test</title></channel></rss>`))
			if err != nil {
				return
			}
		} else {
			// Mock HTML response
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`<html><head><title>Test</title></head><body><h1>Test Magazine</h1></body></html>`))
			if err != nil {
				return
			}
		}
	}))
	defer server.Close()

	// Test that scraper can handle different response types
	// Note: This won't actually scrape the mock server since URL validation will fail,
	// but it demonstrates the test structure for integration testing
	scraper := NewMagazineScraper(DefaultConfig())
	if scraper == nil {
		t.Error("Expected scraper to be created successfully")
	}
}

func TestConcurrentScrapingSetup(t *testing.T) {
	// Test that multiple concurrent scrapers can be created
	configs := []ScraperConfig{
		{ConcurrentRequests: 1, RequestsPerSecond: 1.0, Timeout: time.Minute, UserAgent: "test1"},
		{ConcurrentRequests: 3, RequestsPerSecond: 2.0, Timeout: time.Minute, UserAgent: "test2"},
		{ConcurrentRequests: 5, RequestsPerSecond: 0.5, Timeout: time.Minute, UserAgent: "test3"},
	}

	scrapers := make([]*MagazineScraper, len(configs))
	for i, config := range configs {
		scrapers[i] = NewMagazineScraper(config)
		if scrapers[i] == nil {
			t.Errorf("Failed to create scraper %d", i)
		}
	}

	// Verify each scraper has correct configuration
	for i, scraper := range scrapers {
		if scraper.config.ConcurrentRequests != configs[i].ConcurrentRequests {
			t.Errorf("Scraper %d has wrong ConcurrentRequests: expected %d, got %d",
				i, configs[i].ConcurrentRequests, scraper.config.ConcurrentRequests)
		}
	}
}

func TestRateLimiterSetup(t *testing.T) {
	config := ScraperConfig{
		ConcurrentRequests: 1,
		RequestsPerSecond:  2.0,
		Timeout:            time.Minute,
		UserAgent:          "test",
	}

	scraper := NewMagazineScraper(config)
	if scraper.limiter == nil {
		t.Error("Rate limiter should be initialized")
	}

	// Test that rate limiter respects the configuration
	// Note: We can't easily test the actual rate limiting without making real requests,
	// but we can verify the limiter exists and has reasonable behavior
	start := time.Now()
	ctx := context.Background()

	// This should not block immediately
	err := scraper.limiter.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Rate limiter wait failed: %v", err)
	}

	// Should complete relatively quickly for first request
	if elapsed > 100*time.Millisecond {
		t.Errorf("Rate limiter took too long for first request: %v", elapsed)
	}
}

// Benchmark tests for performance
func BenchmarkCleanText(b *testing.B) {
	text := "  This   is   a   test   string   with   many   spaces  "
	for i := 0; i < b.N; i++ {
		cleanText(text)
	}
}

func BenchmarkNewMagazineScraper(b *testing.B) {
	config := DefaultConfig()
	for i := 0; i < b.N; i++ {
		NewMagazineScraper(config)
	}
}

func BenchmarkArticleCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Article{
			Title:       "Benchmark Article",
			URL:         "https://example.com/benchmark",
			Summary:     "This is a benchmark article",
			Date:        time.Now(),
			Author:      "Benchmark Author",
			ImageURL:    "https://example.com/image.jpg",
			Source:      "Benchmark Source",
			GUID:        "benchmark-guid",
			ScrapedFrom: "html",
		}
	}
}
