package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
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
	// Headless controls whether Chrome runs in headless mode
	Headless bool
	// ManualLogin allows manual login before scraping
	ManualLogin bool
}

// DefaultConfig returns the default scraper configuration
func DefaultConfig() ScraperConfig {
	return ScraperConfig{
		ConcurrentRequests: 2,
		RequestsPerSecond:  0.5,
		Timeout:            15 * time.Minute,
		UserAgent:          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		UseJavaScript:      true,
		MaxPages:           0,
		Debug:              false,
		ScrollDelay:        3 * time.Second,
		MaxScrolls:         150,
		Headless:           true,
		ManualLogin:        false,
	}
}

// Article represents a single Flipboard article with decoded URL support
type Article struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`          // Original Flipboard URL
	ActualURL   string    `json:"actual_url"`   // Decoded real URL
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
	config     ScraperConfig
	rateLimiter *rate.Limiter
	mutex      sync.RWMutex
}

// NewMagazineScraper creates a new magazine scraper with the given configuration
func NewMagazineScraper(config ScraperConfig) *MagazineScraper {
	return &MagazineScraper{
		config:      config,
		rateLimiter: rate.NewLimiter(rate.Limit(config.RequestsPerSecond), 1),
	}
}

// ScrapeURLs scrapes multiple Flipboard magazine URLs concurrently
func (s *MagazineScraper) ScrapeURLs(ctx context.Context, urls []string) ([]Article, error) {
	var allArticles []Article
	var mu sync.Mutex

	// Create error group for concurrent processing
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(s.config.ConcurrentRequests)

	for _, url := range urls {
		url := url // Capture loop variable
		g.Go(func() error {
			// Wait for rate limiter
			if err := s.rateLimiter.Wait(ctx); err != nil {
				return err
			}

			if s.config.Debug {
				log.Printf("Starting to scrape: %s", url)
			}

			articles, err := s.scrapeMagazine(ctx, url)
			if err != nil {
				log.Printf("⚠️  Failed to scrape %s: %v", url, err)
				return nil // Don't fail the entire operation
			}

			mu.Lock()
			allArticles = append(allArticles, articles...)
			mu.Unlock()

			if s.config.Debug {
				log.Printf("Scraped %d articles from %s", len(articles), url)
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return allArticles, err
	}

	// Deduplicate articles by actual URL or title
	return s.deduplicateArticles(allArticles), nil
}

// scrapeMagazine scrapes a single Flipboard magazine using the enhanced method
func (s *MagazineScraper) scrapeMagazine(ctx context.Context, magazineURL string) ([]Article, error) {
	if !s.isValidFlipboardURL(magazineURL) {
		return nil, fmt.Errorf("invalid Flipboard URL: %s", magazineURL)
	}

	// Set up Chrome options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", s.config.Headless),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.UserAgent(s.config.UserAgent),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	chromeCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	if s.config.Debug {
		log.Printf("🌐 Opening browser for %s", magazineURL)
	}

	// Navigate to magazine
	err := chromedp.Run(chromeCtx, chromedp.Navigate(magazineURL))
	if err != nil {
		return nil, fmt.Errorf("failed to navigate to magazine: %w", err)
	}

	// Wait for page to load
	err = chromedp.Run(chromeCtx, chromedp.Sleep(5*time.Second))
	if err != nil {
		return nil, fmt.Errorf("failed to wait for page load: %w", err)
	}

	// Handle manual login if configured
	if s.config.ManualLogin && !s.config.Headless {
		fmt.Print("Please login manually if needed, then press Enter to continue: ")
		fmt.Scanln()
	}

	// Perform infinite scroll if configured
	if s.config.MaxScrolls > 0 {
		if s.config.Debug {
			log.Printf("🌊 Starting infinite scroll (max %d scrolls)", s.config.MaxScrolls)
		}
		err = s.performInfiniteScroll(chromeCtx)
		if err != nil {
			log.Printf("⚠️  Infinite scroll warning: %v", err)
		}
	}

	// Extract articles using JavaScript evaluation
	if s.config.Debug {
		log.Printf("🔓 Extracting and decoding URLs from %s", magazineURL)
	}

	var articlesJSON string
	err = chromedp.Run(chromeCtx,
		chromedp.Evaluate(`
			const articles = document.querySelectorAll('article');
			const results = [];

			articles.forEach(article => {
				const title = article.querySelector('h1, h2, h3, h4, h5, h6')?.textContent?.trim() ||
							  Array.from(article.querySelectorAll('a')).find(a => a.textContent.trim().length > 15)?.textContent?.trim();

				if (!title || title.length < 10) return;

				// Skip navigation items
				const titleLower = title.toLowerCase();
				if (titleLower.includes('code libraries') || titleLower.includes('tech stuff') ||
					titleLower.includes('tutorials') || titleLower.includes('gopher hole') ||
					titleLower.includes('websites')) return;

				// Get the Flipboard URL
				let flipboardURL = '';
				const links = article.querySelectorAll('a[href]');
				for (const link of links) {
					const href = link.getAttribute('href');
					if (href && href.length > 30) {
						flipboardURL = href.startsWith('http') ? href : 'https://flipboard.com' + href;
						break;
					}
				}

				const summary = article.querySelector('p')?.textContent?.trim() || '';

				results.push({
					title: title,
					url: flipboardURL,
					summary: summary.length > 200 ? summary.substring(0, 200) + '...' : summary
				});
			});

			JSON.stringify(results, null, 2);
		`, &articlesJSON),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to extract articles: %w", err)
	}

	// Parse and process articles
	var rawArticles []Article
	if err := json.Unmarshal([]byte(articlesJSON), &rawArticles); err != nil {
		return nil, fmt.Errorf("failed to parse articles JSON: %w", err)
	}

	// Process articles with URL decoding
	var processedArticles []Article
	externalCount := 0

	for _, article := range rawArticles {
		processed := article
		processed.ScrapedFrom = "html"

		// Decode Flipboard URL to get actual URL
		processed.ActualURL = DecodeFlipboardURL(article.URL)

		// Clean tracking parameters from actual URL
		if processed.ActualURL != "" {
			processed.ActualURL = CleanURL(processed.ActualURL)
		}

		// Set source from actual URL
		if processed.ActualURL != "" {
			processed.Source = ExtractSourceFromURL(processed.ActualURL)
			if !strings.Contains(processed.ActualURL, "flipboard.com") {
				externalCount++
			}
		}

		processedArticles = append(processedArticles, processed)
	}

	if s.config.Debug {
		log.Printf("📊 Processed %d articles (%d with external URLs) from %s",
			len(processedArticles), externalCount, magazineURL)
	}

	return processedArticles, nil
}

// performInfiniteScroll performs infinite scrolling with progress tracking
func (s *MagazineScraper) performInfiniteScroll(ctx context.Context) error {
	scrollCount := 0
	lastCount := 0
	stableCount := 0
	maxStableCount := 8

	for scrollCount < s.config.MaxScrolls && stableCount < maxStableCount {
		// Get current article count
		var currentCount int
		err := chromedp.Run(ctx,
			chromedp.Evaluate(`document.querySelectorAll('article').length`, &currentCount),
		)
		if err != nil {
			return fmt.Errorf("failed to get article count: %w", err)
		}

		// Check if count has stabilized
		if currentCount == lastCount {
			stableCount++
		} else {
			stableCount = 0
			lastCount = currentCount
		}

		// Scroll to bottom
		err = chromedp.Run(ctx,
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil),
		)
		if err != nil {
			return fmt.Errorf("failed to scroll: %w", err)
		}

		// Wait for new content to load
		err = chromedp.Run(ctx, chromedp.Sleep(s.config.ScrollDelay))
		if err != nil {
			return fmt.Errorf("failed to wait after scroll: %w", err)
		}

		scrollCount++

		if s.config.Debug && scrollCount%10 == 0 {
			log.Printf("Scroll %d - Articles: %d", scrollCount, currentCount)
		}
	}

	if s.config.Debug {
		log.Printf("Finished scrolling after %d attempts (stable count: %d)", scrollCount, stableCount)
	}

	return nil
}

// deduplicateArticles removes duplicate articles based on actual URL or title
func (s *MagazineScraper) deduplicateArticles(articles []Article) []Article {
	seen := make(map[string]bool)
	var deduplicated []Article

	for _, article := range articles {
		// Use actual URL as primary key, fall back to title
		key := article.ActualURL
		if key == "" {
			key = article.Title
		}

		if !seen[key] && key != "" {
			seen[key] = true
			deduplicated = append(deduplicated, article)
		}
	}

	return deduplicated
}

// isValidFlipboardURL validates that the URL is a Flipboard magazine URL
func (s *MagazineScraper) isValidFlipboardURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return strings.Contains(u.Host, "flipboard.com") &&
		   (strings.Contains(u.Path, "/@") || strings.Contains(u.Path, "/magazine/"))
}
