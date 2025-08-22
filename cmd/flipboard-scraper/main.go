package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/slipperypenguin/flipboard-scraper/pkg"
)

func main() {
	var (
		urls           = flag.String("urls", "", "Comma-separated list of Flipboard magazine URLs to scrape")
		format         = flag.String("format", "csv", "Export format (csv or sqlite)")
		output         = flag.String("output", "articles", "Output file (without extension)")
		concurrent     = flag.Int("concurrent", 2, "Maximum number of concurrent requests (recommended: 1-2 for chromedp)")
		rateLimit      = flag.Float64("rate-limit", 0.5, "Maximum requests per second (lower for chromedp)")
		timeoutSeconds = flag.Int("timeout", 900, "Timeout in seconds (increased for chromedp)")
		userAgent      = flag.String("user-agent", "", "User-Agent string for requests (uses default if empty)")
		maxScrolls     = flag.Int("max-scrolls", 200, "Maximum scrolls for infinite scroll (0 = unlimited)")
		scrollDelay    = flag.Int("scroll-delay", 2, "Delay between scrolls in seconds")
		debug          = flag.Bool("debug", false, "Enable debug logging")
		useJS          = flag.Bool("javascript", true, "Enable JavaScript rendering (required for Flipboard)")
	)

	flag.Parse()

	if *urls == "" {
		log.Fatal("Please provide Flipboard magazine URLs using the -urls flag")
	}

	// Validate that JavaScript is enabled for Flipboard
	if !*useJS {
		fmt.Println("⚠️  Warning: JavaScript rendering is disabled, but Flipboard requires it.")
		fmt.Println("   Enabling JavaScript automatically for best results.")
		*useJS = true
	}

	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		fmt.Println("\n🛑 Received interrupt signal. Cleaning up...")
		cancel()
	}()

	// Configure and create scraper
	config := pkg.ScraperConfig{
		ConcurrentRequests: *concurrent,
		RequestsPerSecond:  *rateLimit,
		Timeout:           time.Duration(*timeoutSeconds) * time.Second,
		MaxScrolls:        *maxScrolls,
		ScrollDelay:       time.Duration(*scrollDelay) * time.Second,
		Debug:             *debug,
		UseJavaScript:     *useJS,
	}

	// Set user agent (use default if not specified)
	if *userAgent != "" {
		config.UserAgent = *userAgent
	} else {
		config.UserAgent = pkg.DefaultConfig().UserAgent
	}

	scraper := pkg.NewMagazineScraper(config)

	// Split URLs and clean them
	urlList := strings.Split(*urls, ",")
	for i, url := range urlList {
		urlList[i] = strings.TrimSpace(url)
	}

	fmt.Printf("🚀 Starting chromedp-powered scraping of %d Flipboard magazine(s)...\n", len(urlList))
	if *debug {
		fmt.Printf("📊 Configuration: max-scrolls=%d, scroll-delay=%ds, rate-limit=%.1f/sec, concurrent=%d\n", 
			*maxScrolls, *scrollDelay, *rateLimit, *concurrent)
	}
	
	fmt.Println("📱 Using headless Chrome to handle JavaScript content...")

	// Display expected time estimate
	estimatedMinutes := len(urlList) * 2 // Rough estimate: 2 minutes per magazine
	if estimatedMinutes > 1 {
		fmt.Printf("⏱️  Estimated time: %d-%d minutes (depends on magazine size)\n", 
			estimatedMinutes, estimatedMinutes*2)
	}

	startTime := time.Now()

	// Scrape URLs
	articles, err := scraper.ScrapeURLs(ctx, urlList)
	if err != nil {
		log.Printf("⚠️  Warning: Some URLs may have failed: %v", err)
	}

	elapsed := time.Since(startTime)

	if len(articles) == 0 {
		log.Fatal("❌ No articles were scraped. Try enabling debug mode (-debug=true) to troubleshoot.")
	}

	fmt.Printf("✅ Successfully scraped %d articles in %v\n", len(articles), elapsed.Round(time.Second))

	// Show some statistics
	if len(articles) > 0 {
		fmt.Printf("📈 Statistics:\n")
		fmt.Printf("   • Total articles: %d\n", len(articles))
		fmt.Printf("   • Articles per minute: %.1f\n", float64(len(articles))/elapsed.Minutes())
		
		// Count articles with different attributes
		withImages := 0
		withSummaries := 0
		for _, article := range articles {
			if article.ImageURL != "" {
				withImages++
			}
			if article.Summary != "" {
				withSummaries++
			}
		}
		fmt.Printf("   • Articles with images: %d (%.1f%%)\n", withImages, float64(withImages)*100/float64(len(articles)))
		fmt.Printf("   • Articles with summaries: %d (%.1f%%)\n", withSummaries, float64(withSummaries)*100/float64(len(articles)))
	}

	// Export based on chosen format
	switch *format {
	case "csv":
		exporter := pkg.NewCSVExporter(*output + ".csv")
		if err := exporter.Export(articles); err != nil {
			log.Fatalf("❌ Failed to export to CSV: %v", err)
		}
		fmt.Printf("💾 Articles exported to %s.csv\n", *output)

	case "sqlite":
		exporter := pkg.NewSQLiteExporter(*output + ".db")
		if err := exporter.Export(articles); err != nil {
			log.Fatalf("❌ Failed to export to SQLite: %v", err)
		}
		fmt.Printf("💾 Articles exported to %s.db\n", *output)

	default:
		log.Fatalf("❌ Unsupported export format: %s", *format)
	}

	fmt.Println("🎉 Complete magazine archive extraction finished!")
	
	if *debug {
		fmt.Printf("🔍 Debug info: All articles were scraped using chromedp with JavaScript rendering\n")
	}
	
	// Provide usage tips
	if len(articles) < 50 {
		fmt.Println("💡 Tip: If you expected more articles, try increasing -max-scrolls or -scroll-delay")
	}
}