package pkg

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
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
	// ScrollDelay is the delay between scroll actions for infinite scroll
	ScrollDelay time.Duration
	// MaxScrolls limits the number of scroll attempts for infinite scroll
	MaxScrolls int
}

// DefaultConfig returns the default scraper configuration
func DefaultConfig() ScraperConfig {
	return ScraperConfig{
		ConcurrentRequests: 3,
		RequestsPerSecond:  1.0,
		Timeout:            5 * time.Minute,
		UserAgent:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		UseJavaScript:      true, // Now defaults to true for Flipboard
		MaxPages:           0,    // No limit by default
		Debug:              false,
		ScrollDelay:        2 * time.Second,
		MaxScrolls:         100, // Reasonable default for infinite scroll
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

	c = colly.NewCollector(
		colly.UserAgent(config.UserAgent),
	)

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
	allArticles = make([]Article, 0, len(urls)*500) // Pre-allocate for more articles
	s.mu.Unlock()

	// Process each URL concurrently
	for _, url := range urls {
		url := url // Create new variable for closure
		g.Go(func() error {
			// Wait for rate limiter
			if err := s.limiter.Wait(ctx); err != nil {
				return fmt.Errorf("rate limiter wait failed: %w", err)
			}

			// Use chromedp for JavaScript-heavy content
			articles, err := s.scrapeWithChromedp(ctx, url)
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

// scrapeWithChromedp uses chromedp to scrape JavaScript-heavy Flipboard content
func (s *MagazineScraper) scrapeWithChromedp(ctx context.Context, targetURL string) ([]Article, error) {
	if !strings.HasPrefix(targetURL, "https://flipboard.com/") {
		return nil, fmt.Errorf("invalid Flipboard URL: %s", targetURL)
	}

	if s.config.Debug {
		log.Printf("Starting chromedp scraping for %s", targetURL)
	}

	// Create a new context for chromedp
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.UserAgent(s.config.UserAgent),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	chromeCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	var articles []Article
	var htmlContent string

	// Run chromedp tasks
	err := chromedp.Run(chromeCtx,
		// Navigate to the page
		chromedp.Navigate(targetURL),
		
		// Wait for the page to load initially
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		
		// Wait a bit for initial content to load
		chromedp.Sleep(3*time.Second),
		
		// Perform infinite scroll to load all content
		chromedp.ActionFunc(func(ctx context.Context) error {
			return s.performInfiniteScroll(ctx)
		}),
		
		// Extract the final HTML content
		chromedp.OuterHTML(`html`, &htmlContent),
	)

	if err != nil {
		return nil, fmt.Errorf("chromedp error: %w", err)
	}

	if s.config.Debug {
		log.Printf("Successfully loaded page content (%d chars)", len(htmlContent))
	}

	// Parse the HTML content to extract articles
	articles = s.parseFlipboardHTML(htmlContent)

	if s.config.Debug {
		log.Printf("Extracted %d articles from %s", len(articles), targetURL)
	}

	return articles, nil
}

// performInfiniteScroll scrolls down the page to load all content
func (s *MagazineScraper) performInfiniteScroll(ctx context.Context) error {
	if s.config.Debug {
		log.Printf("Starting infinite scroll (max scrolls: %d)", s.config.MaxScrolls)
	}

	scrollCount := 0
	lastHeight := 0

	for scrollCount < s.config.MaxScrolls {
		// Get current page height
		var currentHeight int
		err := chromedp.Evaluate(`document.body.scrollHeight`, &currentHeight).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to get page height: %w", err)
		}

		// If height hasn't changed, we've probably reached the end
		if currentHeight == lastHeight {
			if s.config.Debug {
				log.Printf("Page height unchanged (%d), stopping scroll", currentHeight)
			}
			break
		}

		// Scroll to bottom
		err = chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to scroll: %w", err)
		}

		// Wait for new content to load
		time.Sleep(s.config.ScrollDelay)

		lastHeight = currentHeight
		scrollCount++

		if s.config.Debug && scrollCount%10 == 0 {
			log.Printf("Completed %d scrolls, page height: %d", scrollCount, currentHeight)
		}
	}

	if s.config.Debug {
		log.Printf("Finished scrolling after %d attempts", scrollCount)
	}

	return nil
}

// parseFlipboardHTML parses the HTML content and extracts articles
func (s *MagazineScraper) parseFlipboardHTML(htmlContent string) []Article {
	var articles []Article
	
	// Create a new colly collector for parsing
	c := colly.NewCollector()
	
	// Look for various article patterns in Flipboard
	// Flipboard uses different selectors, we'll try multiple patterns
	selectors := []string{
		`[data-test-id="story"]`,           // Main story elements
		`[data-testid="story"]`,            // Alternative test id
		`.story`,                           // Story class
		`.story-item`,                      // Story item class
		`.magazine-story`,                  // Magazine story class
		`article`,                          // Standard article tags
		`[role="article"]`,                 // ARIA article role
		`.flip-story`,                      // Flipboard story class
		`.story-tile`,                      // Story tile class
	}
	
	for _, selector := range selectors {
		c.OnHTML(selector, func(e *colly.HTMLElement) {
			article := s.extractArticleFromElement(e)
			if article.Title != "" && article.URL != "" {
				articles = append(articles, article)
			}
		})
	}
	
	// Visit the HTML content
	c.OnHTML("html", func(e *colly.HTMLElement) {
		// This will trigger the article extraction
	})
	
	// Create a temporary reader for the HTML content
	c.Visit("data:text/html," + url.QueryEscape(htmlContent))
	
	// Deduplicate articles based on URL
	articles = s.deduplicateArticles(articles)
	
	return articles
}

// extractArticleFromElement extracts article data from a single HTML element
func (s *MagazineScraper) extractArticleFromElement(e *colly.HTMLElement) Article {
	article := Article{
		Date:        time.Now(),
		ScrapedFrom: "html",
	}
	
	// Extract title from various possible selectors
	titleSelectors := []string{
		"h1", "h2", "h3", "h4", 
		".title", ".headline", ".story-title",
		"[data-test-id='story-title']", "[data-testid='story-title']",
		".flip-story-title",
	}
	
	for _, sel := range titleSelectors {
		if title := cleanText(e.ChildText(sel)); title != "" {
			article.Title = title
			break
		}
	}
	
	// Extract URL from various possible selectors
	urlSelectors := []string{
		"a[href]", 
		"[data-test-id='story-link']", "[data-testid='story-link']",
		".story-link", ".flip-story-link",
	}
	
	for _, sel := range urlSelectors {
		if href := e.ChildAttr(sel, "href"); href != "" {
			article.URL = s.normalizeURL(href)
			break
		}
	}
	
	// Extract summary/description
	summarySelectors := []string{
		".description", ".summary", ".excerpt", 
		".story-description", ".flip-story-summary",
		"p", ".text",
	}
	
	for _, sel := range summarySelectors {
		if summary := cleanText(e.ChildText(sel)); summary != "" && len(summary) > 20 {
			article.Summary = summary
			if len(article.Summary) > 500 {
				article.Summary = article.Summary[:500] + "..."
			}
			break
		}
	}
	
	// Extract image URL
	imgSelectors := []string{
		"img[src]", ".story-image img", ".flip-story-image img",
		"[data-test-id='story-image'] img", "[data-testid='story-image'] img",
	}
	
	for _, sel := range imgSelectors {
		if imgSrc := e.ChildAttr(sel, "src"); imgSrc != "" {
			article.ImageURL = s.normalizeURL(imgSrc)
			break
		}
	}
	
	// Extract source/author
	sourceSelectors := []string{
		".source", ".author", ".byline", ".publication",
		".story-source", ".flip-story-source",
		"[data-test-id='story-source']", "[data-testid='story-source']",
	}
	
	for _, sel := range sourceSelectors {
		if source := cleanText(e.ChildText(sel)); source != "" {
			article.Source = source
			break
		}
	}
	
	// Try to extract date if available
	dateSelectors := []string{
		"time[datetime]", ".date", ".timestamp", 
		".story-date", ".flip-story-date",
		"[data-test-id='story-date']", "[data-testid='story-date']",
	}
	
	for _, sel := range dateSelectors {
		if dateStr := e.ChildAttr(sel, "datetime"); dateStr != "" {
			if parsedDate, err := time.Parse(time.RFC3339, dateStr); err == nil {
				article.Date = parsedDate
				break
			}
		}
		if dateText := cleanText(e.ChildText(sel)); dateText != "" {
			// Try various date formats
			formats := []string{
				"2006-01-02T15:04:05Z07:00",
				"2006-01-02 15:04:05",
				"January 2, 2006",
				"Jan 2, 2006",
				"2006-01-02",
			}
			for _, format := range formats {
				if parsedDate, err := time.Parse(format, dateText); err == nil {
					article.Date = parsedDate
					break
				}
			}
		}
	}
	
	return article
}

// normalizeURL converts relative URLs to absolute URLs
func (s *MagazineScraper) normalizeURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	
	// If it's already absolute, return as-is
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	
	// If it starts with //, prepend https:
	if strings.HasPrefix(rawURL, "//") {
		return "https:" + rawURL
	}
	
	// If it starts with /, it's relative to flipboard.com
	if strings.HasPrefix(rawURL, "/") {
		return "https://flipboard.com" + rawURL
	}
	
	// Otherwise, assume it's a path relative to flipboard.com
	return "https://flipboard.com/" + rawURL
}

// deduplicateArticles removes duplicate articles based on URL
func (s *MagazineScraper) deduplicateArticles(articles []Article) []Article {
	seen := make(map[string]bool)
	var deduplicated []Article
	
	for _, article := range articles {
		// Skip articles without URLs or titles
		if article.URL == "" || article.Title == "" {
			continue
		}
		
		// Skip if we've already seen this URL
		if seen[article.URL] {
			continue
		}
		
		seen[article.URL] = true
		deduplicated = append(deduplicated, article)
	}
	
	return deduplicated
}

// cleanText removes extra whitespace and normalizes text
func cleanText(text string) string {
	// Remove extra whitespace and normalize
	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return text
}