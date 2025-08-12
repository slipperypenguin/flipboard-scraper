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
	"github.com/gocolly/colly/v2/debug"
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
	// UserAgent to use for HTTP requests
	UserAgent string
	// UseJavaScript enables JavaScript rendering (requires Chrome/Chromium)
	UseJavaScript bool
	// MaxPages limits how many pages to scrape (0 = no limit)
	MaxPages int
	// Debug enables verbose logging
	Debug bool
}

// DefaultConfig returns the default scraper configuration
func DefaultConfig() ScraperConfig {
	return ScraperConfig{
		ConcurrentRequests: 3,
		RequestsPerSecond:  1.0,
		Timeout:            5 * time.Minute,
		UserAgent:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		UseJavaScript:      false, // Set to true if you have Chrome/Chromium installed
		MaxPages:           10,    // Reasonable default to prevent infinite scraping
		Debug:              false,
	}
}

// Article represents a single Flipboard article
type Article struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Summary     string    `json:"summary"`
	Date        time.Time `json:"date"`
	Author      string    `json:"author,omitempty"`
	ImageURL    string    `json:"image_url,omitempty"`
	Source      string    `json:"source,omitempty"`
	GUID        string    `json:"guid,omitempty"`
	ScrapedFrom string    `json:"scraped_from"` // "rss" or "html"
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
	// Create collector with appropriate configuration
	var c *colly.Collector

	if config.UseJavaScript {
		// JavaScript-enabled scraping (requires Chrome/Chromium)
		c = colly.NewCollector(
			colly.UserAgent(config.UserAgent),
		)
		// Note: For full JavaScript support, you'd need to integrate with chromedp or similar
		// This is a placeholder for the JavaScript-enabled approach
	} else {
		// Standard HTTP scraping
		c = colly.NewCollector(
			colly.UserAgent(config.UserAgent),
		)
	}

	// Enable debug mode if requested
	if config.Debug {
		c.SetDebugger(&debug.LogDebugger{})
	}

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

	var allArticles []Article
	s.mu.Lock()
	allArticles = make([]Article, 0, len(urls)*50) // Pre-allocate for more articles
	s.mu.Unlock()

	// Process each URL concurrently
	for _, url := range urls {
		url := url // Create new variable for closure
		g.Go(func() error {
			// Wait for rate limiter
			if err := s.limiter.Wait(ctx); err != nil {
				return fmt.Errorf("rate limiter wait failed: %w", err)
			}

			// Try both RSS and HTML scraping approaches
			articles, err := s.scrapeURLWithFallback(ctx, url)
			if err != nil {
				return fmt.Errorf("failed to scrape %s: %w", url, err)
			}

			// Safely append results
			s.mu.Lock()
			allArticles = append(allArticles, articles...)
			s.mu.Unlock()

			return nil
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return allArticles, fmt.Errorf("scraping error: %w", err)
	}

	return allArticles, nil
}

// scrapeURLWithFallback tries multiple approaches to get maximum historical content
func (s *MagazineScraper) scrapeURLWithFallback(ctx context.Context, url string) ([]Article, error) {
	var allArticles []Article
	var errors []string

	// Approach 1: Try RSS first (gets recent items quickly)
	if s.config.Debug {
		log.Printf("Trying RSS approach for %s", url)
	}

	rssArticles, err := s.scrapeRSSFeed(ctx, url)
	if err != nil {
		errors = append(errors, fmt.Sprintf("RSS failed: %v", err))
		if s.config.Debug {
			log.Printf("RSS scraping failed for %s: %v", url, err)
		}
	} else {
		allArticles = append(allArticles, rssArticles...)
		if s.config.Debug {
			log.Printf("RSS scraping found %d articles for %s", len(rssArticles), url)
		}
	}

	// Approach 2: Try HTML scraping with pagination (gets historical content)
	if s.config.Debug {
		log.Printf("Trying HTML approach for %s", url)
	}

	htmlArticles, err := s.scrapeHTMLWithPagination(ctx, url)
	if err != nil {
		errors = append(errors, fmt.Sprintf("HTML failed: %v", err))
		if s.config.Debug {
			log.Printf("HTML scraping failed for %s: %v", url, err)
		}
	} else {
		// Deduplicate articles (HTML might contain articles we already got from RSS)
		deduped := s.deduplicateArticles(allArticles, htmlArticles)
		allArticles = append(allArticles, deduped...)
		if s.config.Debug {
			log.Printf("HTML scraping found %d new articles for %s", len(deduped), url)
		}
	}

	if len(allArticles) == 0 {
		return nil, fmt.Errorf("all approaches failed: %s", strings.Join(errors, "; "))
	}

	return allArticles, nil
}

// scrapeRSSFeed attempts to scrape using RSS (gets recent articles)
func (s *MagazineScraper) scrapeRSSFeed(ctx context.Context, url string) ([]Article, error) {
	// Convert to RSS URL
	rssURL, err := s.convertToRSSURL(url)
	if err != nil {
		return nil, err
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", rssURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", s.config.UserAgent)

	// Make HTTP request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch RSS feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RSS feed returned status %d", resp.StatusCode)
	}

	// For now, return empty to focus on HTML approach
	// TODO: Implement RSS parsing from previous version
	return []Article{}, nil
}

// scrapeHTMLWithPagination attempts to scrape HTML with pagination support
func (s *MagazineScraper) scrapeHTMLWithPagination(ctx context.Context, baseURL string) ([]Article, error) {
	if !strings.HasPrefix(baseURL, "https://flipboard.com/") {
		return nil, fmt.Errorf("invalid Flipboard URL: %s", baseURL)
	}

	var allArticles []Article
	currentPage := 1

	for currentPage <= s.config.MaxPages {
		select {
		case <-ctx.Done():
			return allArticles, ctx.Err()
		default:
		}

		// Wait for rate limiter
		if err := s.limiter.Wait(ctx); err != nil {
			return allArticles, err
		}

		// Construct paginated URL (this is a guess - would need to reverse engineer Flipboard's pagination)
		pageURL := s.buildPageURL(baseURL, currentPage)

		if s.config.Debug {
			log.Printf("Scraping page %d: %s", currentPage, pageURL)
		}

		// Scrape this page
		pageArticles, hasMore, err := s.scrapePage(ctx, pageURL)
		if err != nil {
			if s.config.Debug {
				log.Printf("Failed to scrape page %d: %v", currentPage, err)
			}
			break
		}

		allArticles = append(allArticles, pageArticles...)

		if s.config.Debug {
			log.Printf("Page %d yielded %d articles", currentPage, len(pageArticles))
		}

		// If no more pages or no articles found, stop
		if !hasMore || len(pageArticles) == 0 {
			break
		}

		currentPage++
	}

	return allArticles, nil
}

// buildPageURL constructs a URL for a specific page (needs reverse engineering)
func (s *MagazineScraper) buildPageURL(baseURL string, page int) string {
	// This is speculative - Flipboard might use different pagination mechanisms:
	// Option 1: Query parameter
	if page == 1 {
		return baseURL
	}
	return fmt.Sprintf("%s?page=%d", baseURL, page)

	// Option 2: Path-based pagination
	// return fmt.Sprintf("%s/page/%d", baseURL, page)

	// Option 3: AJAX/JSON API (would require different handling)
	// return fmt.Sprintf("%s/api/items?offset=%d", baseURL, (page-1)*20)
}

// scrapePage scrapes a single page and returns articles + whether more pages exist
func (s *MagazineScraper) scrapePage(ctx context.Context, pageURL string) ([]Article, bool, error) {
	var articles []Article
	var hasNextPage bool
	var scrapeError error

	// Create a new collector instance for this page
	c := s.collector.Clone()

	// Set up article extraction
	c.OnHTML("article, .item, .story, .post", func(e *colly.HTMLElement) {
		article := Article{
			Title:       cleanText(e.ChildText("h1, h2, h3, h4, .title, .headline")),
			URL:         e.ChildAttr("a", "href"),
			Summary:     cleanText(e.ChildText(".description, .summary, .excerpt, p")),
			Date:        time.Now(), // Default to current time if we can't parse
			ScrapedFrom: "html",
		}

		// Try to extract image
		if imgSrc := e.ChildAttr("img", "src"); imgSrc != "" {
			article.ImageURL = imgSrc
		}

		// Try to extract source/author
		if source := cleanText(e.ChildText(".source, .author, .byline")); source != "" {
			article.Source = source
		}

		// Only add articles with at least a title
		if article.Title != "" {
			articles = append(articles, article)
		}
	})

	// Check for pagination indicators
	c.OnHTML(".next, .pagination, .load-more", func(e *colly.HTMLElement) {
		hasNextPage = true
	})

	// Handle errors
	c.OnError(func(r *colly.Response, err error) {
		scrapeError = fmt.Errorf("request failed with status %d: %w", r.StatusCode, err)
	})

	// Visit the page
	err := c.Visit(pageURL)
	if err != nil {
		return nil, false, fmt.Errorf("failed to visit page: %w", err)
	}

	// Wait for completion
	c.Wait()

	if scrapeError != nil {
		return nil, false, scrapeError
	}

	return articles, hasNextPage, nil
}

// convertToRSSURL converts a Flipboard magazine URL to its RSS feed URL
func (s *MagazineScraper) convertToRSSURL(url string) (string, error) {
	if !strings.HasPrefix(url, "https://flipboard.com/") {
		return "", fmt.Errorf("invalid Flipboard URL: %s", url)
	}

	if strings.HasSuffix(url, ".rss") {
		return url, nil
	}

	return url + ".rss", nil
}

// deduplicateArticles removes duplicate articles between two slices
func (s *MagazineScraper) deduplicateArticles(existing, new []Article) []Article {
	existingURLs := make(map[string]bool)
	for _, article := range existing {
		if article.URL != "" {
			existingURLs[article.URL] = true
		}
	}

	var deduplicated []Article
	for _, article := range new {
		if article.URL == "" || !existingURLs[article.URL] {
			deduplicated = append(deduplicated, article)
		}
	}

	return deduplicated
}

// cleanText removes extra whitespace and normalizes text
func cleanText(text string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}
