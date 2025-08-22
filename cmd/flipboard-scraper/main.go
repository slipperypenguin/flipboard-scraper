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
		output         = flag.String("output", "articles", "Output filename (without extension)")
		concurrent     = flag.Int("concurrent", 1, "Maximum number of concurrent requests (recommended: 1-2 for chromedp)")
		rateLimit      = flag.Float64("rate-limit", 0.5, "Maximum requests per second (lower for chromedp)")
		timeoutSeconds = flag.Int("timeout", 900, "Timeout in seconds (increased for chromedp)")
		userAgent      = flag.String("user-agent", "", "User-Agent string for requests (uses default if empty)")
		maxScrolls     = flag.Int("max-scrolls", 150, "Maximum scrolls for infinite scroll (0 = disabled)")
		scrollDelay    = flag.Int("scroll-delay", 3, "Delay between scrolls in seconds")
		debug          = flag.Bool("debug", false, "Enable debug logging")
		quickSample    = flag.Bool("quick-sample", false, "Quick sample mode: scrape first page only")
	)

	flag.Parse()

	fmt.Println("🗞️ Flipboard Magazine Exporter")
	fmt.Println()

	if *urls == "" {
		log.Fatal("Please provide Flipboard magazine URLs using the -urls flag")
	}

	// Quick sample mode limits scrolling
	if *quickSample {
		*maxScrolls = 0
		fmt.Println("🚀 Quick sample mode: scraping first page only")
	}

	// Create context that can be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutSeconds)*time.Second)
	defer cancel()

	// Handle interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		fmt.Println("\n🛑 Received interrupt signal. Cleaning up...")
		cancel()
	}()

	// Configure scraper settings
	config := pkg.ScraperConfig{
		ConcurrentRequests: *concurrent,
		RequestsPerSecond:  *rateLimit,
		Timeout:            time.Duration(*timeoutSeconds) * time.Second,
		MaxScrolls:         *maxScrolls,
		ScrollDelay:        time.Duration(*scrollDelay) * time.Second,
		Debug:              *debug,
		UseJavaScript:      true, // Always required for Flipboard
	}

	// Set user agent (use default if not specified)
	if *userAgent != "" {
		config.UserAgent = *userAgent
	} else {
		config.UserAgent = pkg.DefaultConfig().UserAgent
	}

	scraper := pkg.NewMagazineScraper(config)

	// Split magazine URLs and clean them
	urlList := strings.Split(*urls, ",")
	for i, url := range urlList {
		urlList[i] = strings.TrimSpace(url)
	}

	// Display configuration
	fmt.Printf("🛸 Starting extraction of %d Flipboard magazine(s)...\n", len(urlList))
	if *debug {
		fmt.Printf("⚙️ Configuration:\n")
		fmt.Printf("   • Max scrolls: %d\n", *maxScrolls)
		fmt.Printf("   • Scroll delay: %ds\n", *scrollDelay)
		fmt.Printf("   • Rate limit: %.1f/sec\n", *rateLimit)
		fmt.Printf("   • Concurrent: %d\n", *concurrent)
		fmt.Printf("   • Timeout: %ds\n", *timeoutSeconds)
	}

	// Estimate time (very rough)
	if !*quickSample {
		estimatedMinutes := len(urlList) * 3 // Rough estimate: 3 minutes per magazine with scrolling
		if estimatedMinutes > 2 {
			fmt.Printf("⏱️  Estimated time: %d-%d minutes (depends on magazine size and scroll settings)\n",
				estimatedMinutes, estimatedMinutes*2)
		}
	}

	fmt.Println("🔐 Manual login to Flipboard is required - browser will open for you to login")
	fmt.Println()
	startTime := time.Now()

	// Extract articles from given magazine urls
	articles, err := scraper.ScrapeURLs(ctx, urlList)
	if err != nil {
		log.Printf("⚠️  Warning: Some URLs may have failed: %v", err)
	}

	elapsed := time.Since(startTime)

	if len(articles) == 0 {
		log.Fatal("❌ No articles were scraped. Try enabling debug mode (-debug=true) to troubleshoot.")
	}
	fmt.Printf("✅ Successfully scraped %d articles in %v\n", len(articles), elapsed.Round(time.Second))

	// Generate and display statistics
	stats := pkg.GenerateStats(articles)
	pkg.PrintStats(stats)

	// Show sample of decoded URLs
	fmt.Println("\n🔗 Sample of decoded URLs:")
	sampleCount := 5
	if len(articles) < sampleCount {
		sampleCount = len(articles)
	}

	for i := 0; i < sampleCount; i++ {
		article := articles[i]
		fmt.Printf("%d. %s\n", i+1, article.Title)
		if article.ActualURL != "" {
			fmt.Printf("   Decoded URL: %s\n", article.ActualURL)
			if article.Source != "" {
				fmt.Printf("   Source: %s\n", article.Source)
			}
		} else {
			fmt.Printf("   Flipboard URL: %s\n", article.URL)
		}
		fmt.Println()
	}

	if len(articles) > sampleCount {
		fmt.Printf("... and %d more\n\n", len(articles)-sampleCount)
	}

	// Export based on chosen format
	fmt.Printf("💾 Exporting to %s format...\n", *format)

	switch *format {
	case "csv":
		exporter := pkg.NewCSVExporter(*output + ".csv")
		if err := exporter.Export(articles); err != nil {
			log.Fatalf("❌ Failed to export to CSV: %v", err)
		}
		fmt.Printf("✔️ Articles exported to %s.csv\n", *output)

	case "sqlite":
		exporter := pkg.NewSQLiteExporter(*output + ".db")
		if err := exporter.Export(articles); err != nil {
			log.Fatalf("❌ Failed to export to SQLite: %v", err)
		}
		fmt.Printf("✔️ Articles exported to %s.db\n", *output)

		// Some SQLite info
		fmt.Println("\n🗄️  SQLite database includes:")
		fmt.Println("   • Consistent schema with URL decoding support")
		fmt.Println("   • Indexes on url, actual_url, source, date for fast queries")
		fmt.Println("   • Duplicate prevention based on actual_url + title")

	default:
		log.Fatalf("❌ Unsupported export format: %s", *format)
	}

	fmt.Println("\n🏁 Magazine extraction with URL decoding complete!")

	// Provide quality assessment
	decodedPercentage := float64(stats.ExternalURLs) * 100 / float64(stats.TotalArticles)
	if decodedPercentage > 80 {
		fmt.Printf("Successfully decoded %.1f%% of article URLs\n", decodedPercentage)
	} else {
		fmt.Printf("⚠️ Low decode rate: only %.1f%% of URLs were decoded. Consider adjusting settings.\n", decodedPercentage)
	}

	// Provide usage tips
	fmt.Println()
	if *quickSample {
		fmt.Println("💡 This was a quick sample. For complete archives, remove -quick-sample and increase -max-scrolls")
	} else if stats.ExternalURLs < 50 {
		fmt.Println("💡 Tips for better results:")
		fmt.Println("   • Increase scroll attempts: -max-scrolls=200")
		fmt.Println("   • Add scroll delay: -scroll-delay=5")
	}
}
