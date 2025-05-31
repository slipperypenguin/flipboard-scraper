package pkg

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
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
	// Debug enables verbose logging
	Debug bool
}

// DefaultConfig returns the default scraper configuration
func DefaultConfig() ScraperConfig {
	return ScraperConfig{
		ConcurrentRequests: 3,
		RequestsPerSecond:  1.0,
		Timeout:            2 * time.Minute,
		Debug:              true,
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
	// Set up HTTP client with timeouts
	httpClient := &http.Client{
		Timeout: 30 * time.Second, // Individual request timeout
		Transport: &http.Transport{
			DisableKeepAlives:   true,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			DisableCompression:  false,
			MaxIdleConns:        config.ConcurrentRequests,
			MaxIdleConnsPerHost: config.ConcurrentRequests,
		},
	}

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		colly.MaxDepth(1),
		colly.AllowURLRevisit(),
	)

	// Configure the collector
	c.WithTransport(httpClient.Transport)
	c.SetRequestTimeout(30 * time.Second)

	// Only enable debug logging if configured
	if config.Debug {
		c.OnRequest(func(r *colly.Request) {
			log.Printf("[Scraper Debug] Making request to: %v", r.URL)
		})
		c.OnResponse(func(r *colly.Response) {
			log.Printf("[Scraper Debug] Got response from: %v (status: %d, length: %d)", r.Request.URL, r.StatusCode, len(r.Body))
			
			// TODO: remove once debugging complete
			r.Save("scrape-export.html")
		})
		c.OnError(func(r *colly.Response, err error) {
			log.Printf("[Scraper Debug] Error on %v: %v", r.Request.URL, err)
		})
	}

	// Set up rate limiting with burst capacity
	limiter := rate.NewLimiter(rate.Limit(config.RequestsPerSecond), 3) // Allow burst of 3

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

	if s.config.Debug {
		log.Printf("[Scraper] Starting to scrape %d URLs", len(urls))
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
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				// Wait for rate limiter
				if err := s.limiter.Wait(ctx); err != nil {
					return fmt.Errorf("rate limiter wait failed: %w", err)
				}

				if s.config.Debug {
					log.Printf("[Scraper] Starting to scrape URL: %s", url)
				}

				// Scrape single URL with retries
				pageArticles, err := s.scrapeURLWithRetry(ctx, url, 3)
				if err != nil {
					if s.config.Debug {
						log.Printf("[Scraper] Error scraping %s: %v", url, err)
					}
					return fmt.Errorf("failed to scrape %s: %w", url, err)
				}

				if s.config.Debug {
					log.Printf("[Scraper] Found %d articles on %s", len(pageArticles), url)
				}

				// Safely append results
				s.mu.Lock()
				articles = append(articles, pageArticles...)
				s.mu.Unlock()

				return nil
			}
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return articles, fmt.Errorf("scraping error: %w", err)
	}

	if s.config.Debug {
		log.Printf("[Scraper] Completed scraping. Total articles found: %d", len(articles))
	}

	return articles, nil
}

// scrapeURLWithRetry attempts to scrape a URL with retries
func (s *MagazineScraper) scrapeURLWithRetry(ctx context.Context, url string, maxRetries int) ([]Article, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retrying
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
			if s.config.Debug {
				log.Printf("[Scraper] Retry attempt %d for URL: %s", attempt+1, url)
			}
		}

		articles, err := s.scrapeURL(ctx, url)
		if err == nil {
			return articles, nil
		}
		lastErr = err

		// Don't retry on certain errors
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "403") {
			return nil, err
		}
	}
	return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
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
	var done = make(chan bool, 1)

	// Reset collector callbacks
	s.collector.OnHTML("a[data-source-url]", func(e *colly.HTMLElement) {
		if s.config.Debug {
			log.Printf("[Scraper Debug] Found article: %s", e.Text)
		}

		title := cleanText(e.Text)
		sourceURL := e.Attr("data-source-url")
		summary := cleanText(e.Attr("data-description"))

		// Skip if no title or URL
		if title == "" || sourceURL == "" {
			return
		}

		article := Article{
			Title:   title,
			URL:     sourceURL,
			Summary: summary,
			Date:    time.Now(), // Flipboard doesn't consistently expose article dates in HTML
		}

		s.mu.Lock()
		articles = append(articles, article)
		s.mu.Unlock()
	})

	// Set up error handling
	s.collector.OnError(func(r *colly.Response, err error) {
		scrapeErr = fmt.Errorf("request failed with status %d: %w", r.StatusCode, err)
		if s.config.Debug {
			log.Printf("[Scraper] Error on %s: %v", url, scrapeErr)
		}
	})

	// Start scraping in a goroutine
	go func() {
		if s.config.Debug {
			log.Printf("[Scraper] Starting visit to %s", url)
		}
		err := s.collector.Visit(url)
		if err != nil {
			scrapeErr = fmt.Errorf("failed to start scraping: %w", err)
			if s.config.Debug {
				log.Printf("[Scraper] Visit error on %s: %v", url, err)
			}
		}
		s.collector.Wait()
		done <- true
	}()

	// Wait for either completion or context cancellation
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("scraping cancelled: %w", ctx.Err())
	case <-done:
		if scrapeErr != nil {
			return nil, scrapeErr
		}
		if len(articles) == 0 {
			if s.config.Debug {
				log.Printf("[Scraper] No articles found on %s", url)
			}
		}
		return articles, nil
	}
}

// cleanText removes extra whitespace and normalizes text
func cleanText(text string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}
