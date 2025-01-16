package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly/v2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

// ScraperConfig holds configuration for the magazine scraper
type ScraperConfig struct {
	// ConcurrentRequests is the maximum number of concurrent scraping requests
	ConcurrentRequests int
	// RequestsPerSecond is the maximum number of requests per second
	RequestsPerSecond float64
	// Timeout is the maximum time to wait for scraping to complete
	Timeout time.Duration
}

// DefaultConfig returns the default scraper configuration
func DefaultConfig() ScraperConfig {
	return ScraperConfig{
		ConcurrentRequests: 3,
		RequestsPerSecond:  1.0,
		Timeout:            2 * time.Minute,
	}
}

// Article represents a single Flipboard article
type Article struct {
	Title   string    `json:"title"`
	URL     string    `json:"url"`
	Summary string    `json:"summary"`
	Date    time.Time `json:"date"`
}

// MagazineScraper handles scraping of Flipboard magazines
type MagazineScraper struct {
	collector *colly.Collector
	limiter   *rate.Limiter
	config    ScraperConfig
	mu        sync.Mutex // protects articles during concurrent scraping
}

// NewMagazineScraper creates a new scraper instance with the given configuration
func NewMagazineScraper(config ScraperConfig) *MagazineScraper {
	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
		colly.MaxDepth(1),
	)

	// Set up rate limiting
	limiter := rate.NewLimiter(rate.Limit(config.RequestsPerSecond), 1)

	return &MagazineScraper{
		collector: c,
		limiter:   limiter,
		config:    config,
	}
}

// ScrapeURLs concurrently scrapes multiple Flipboard magazine URLs
func (s *MagazineScraper) ScrapeURLs(ctx context.Context, urls []string) ([]Article, error) {
	if len(urls) == 0 {
		return nil, errors.New("no URLs provided")
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()

	// Create an error group for concurrent execution
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.config.ConcurrentRequests)

	var articles []Article
	s.mu.Lock()
	articles = make([]Article, 0, len(urls)*10) // Pre-allocate with reasonable capacity
	s.mu.Unlock()

	// Process each URL concurrently
	for _, url := range urls {
		url := url // Create new variable for closure
		g.Go(func() error {
			// Wait for rate limiter
			if err := s.limiter.Wait(ctx); err != nil {
				return fmt.Errorf("rate limiter wait failed: %w", err)
			}

			// Scrape single URL
			pageArticles, err := s.scrapeURL(ctx, url)
			if err != nil {
				return fmt.Errorf("failed to scrape %s: %w", url, err)
			}

			// Safely append results
			s.mu.Lock()
			articles = append(articles, pageArticles...)
			s.mu.Unlock()

			return nil
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return articles, fmt.Errorf("scraping error: %w", err)
	}

	return articles, nil
}

// ScrapeURL scrapes a single Flipboard magazine URL
func (s *MagazineScraper) ScrapeURL(ctx context.Context, url string) ([]Article, error) {
	return s.scrapeURL(ctx, url)
}

// scrapeURL is the internal implementation for scraping a single URL
func (s *MagazineScraper) scrapeURL(ctx context.Context, url string) ([]Article, error) {
	if !strings.HasPrefix(url, "https://flipboard.com/") {
		return nil, fmt.Errorf("invalid Flipboard URL: %s", url)
	}

	var articles []Article
	var scrapeErr error
	var done = make(chan bool)

	// Set up callbacks
	s.collector.OnHTML("article.item", func(e *colly.HTMLElement) {
		article := Article{
			Title:   cleanText(e.ChildText("h3")),
			URL:     e.ChildAttr("a", "href"),
			Summary: cleanText(e.ChildText("p.description")),
			Date:    time.Now(), // Flipboard doesn't always expose article dates
		}

		// Only add articles with at least a title
		if article.Title != "" {
			articles = append(articles, article)
		}
	})

	// Set up error handling
	s.collector.OnError(func(r *colly.Response, err error) {
		scrapeErr = fmt.Errorf("request failed with status %d: %w", r.StatusCode, err)
	})

	// Start scraping in a goroutine
	go func() {
		err := s.collector.Visit(url)
		if err != nil {
			scrapeErr = fmt.Errorf("failed to start scraping: %w", err)
		}
		s.collector.Wait()
		close(done)
	}()

	// Wait for either completion or context cancellation
	select {
	case <-ctx.Done():
		s.collector.AllowURLRevisit = true // Reset collector state
		return nil, fmt.Errorf("scraping cancelled: %w", ctx.Err())
	case <-done:
		if scrapeErr != nil {
			return nil, scrapeErr
		}
		return articles, nil
	}
}

// cleanText removes extra whitespace and normalizes text
func cleanText(text string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}
