package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"pkg"
	"strings"
	"time"
)

func main() {
	var (
		urls           = flag.String("urls", "", "Comma-separated list of Flipboard magazine URLs to scrape")
		format         = flag.String("format", "csv", "Export format (csv or sqlite)")
		output         = flag.String("output", "articles", "Output file (without extension)")
		concurrent     = flag.Int("concurrent", 3, "Maximum number of concurrent requests")
		rateLimit      = flag.Float64("rate-limit", 1.0, "Maximum requests per second")
		timeoutSeconds = flag.Int("timeout", 120, "Timeout in seconds")
	)

	flag.Parse()

	if *urls == "" {
		log.Fatal("Please provide Flipboard magazine URLs using the -urls flag")
	}

	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		fmt.Println("\nReceived interrupt signal. Cleaning up...")
		cancel()
	}()

	// Configure and create scraper
	config := pkg.ScraperConfig{
		ConcurrentRequests: *concurrent,
		RequestsPerSecond:  *rateLimit,
		Timeout:           time.Duration(*timeoutSeconds) * time.Second,
	}
	scraper := pkg.NewMagazineScraper(config)

	// Split URLs and clean them
	urlList := strings.Split(*urls, ",")
	for i, url := range urlList {
		urlList[i] = strings.TrimSpace(url)
	}

	// Scrape URLs
	articles, err := scraper.ScrapeURLs(ctx, urlList)
	if err != nil {
		log.Printf("Warning: Some URLs may have failed: %v", err)
	}

	if len(articles) == 0 {
		log.Fatal("No articles were scraped")
	}

	fmt.Printf("Found %d articles\n", len(articles))

	// Export based on chosen format
	switch *format {
	case "csv":
		exporter := pkg.NewCSVExporter(*output + ".csv")
		if err := exporter.Export(articles); err != nil {
			log.Fatalf("Failed to export to CSV: %v", err)
		}
		fmt.Printf("Articles exported to %s.csv\n", *output)

	case "sqlite":
		exporter := pkg.NewSQLiteExporter(*output + ".db")
		if err := exporter.Export(articles); err != nil {
			log.Fatalf("Failed to export to SQLite: %v", err)
		}
		fmt.Printf("Articles exported to %s.db\n", *output)

	default:
		log.Fatalf("Unsupported export format: %s", *format)
	}
}
