package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/slipperypenguin/flipboard-scraper/pkg"
)

func main() {
	// Check if first argument is a command
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "scrape":
		runScrapeCommand()
	case "combine":
		runCombineCommand()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Flipboard Magazine Tool")
	fmt.Println("Usage:")
	fmt.Println("  flipboard-scraper scrape [options]    - Scrape Flipboard magazines")
	fmt.Println("  flipboard-scraper combine [options]   - Combine CSV files")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  scrape   - Scrape articles from Flipboard magazines")
	fmt.Println("  combine  - Combine multiple magazine CSV files into SQLite")
	fmt.Println("")
	fmt.Println("For command-specific help:")
	fmt.Println("  flipboard-scraper scrape -h")
	fmt.Println("  flipboard-scraper combine -h")
}

func runScrapeCommand() {
	// Create a new flag set for scrape command
	scrapeFlags := flag.NewFlagSet("scrape", flag.ExitOnError)

	// Existing scrape flags (keeping your current implementation)
	magazineURL := scrapeFlags.String("url", "", "Flipboard magazine URL to scrape")

	// Parse scrape flags
	err := scrapeFlags.Parse(os.Args[2:])
	if err != nil {
		return
	}

	// Validate required parameters
	if *magazineURL == "" {
		fmt.Println("❌ Magazine URL is required")
		scrapeFlags.Usage()
		os.Exit(1)
	}

	// Run the existing scrape logic (adapted from your current main.go)
	fmt.Printf("🔍 Starting Flipboard magazine scraper...\n")
	fmt.Printf("📖 Magazine: %s\n", *magazineURL)

	// ... (rest of your existing scrape logic)
	// I'm showing the structure - you'd move your existing main.go scrape logic here
}

func runCombineCommand() {
	// Create a new flag set for combine command
	combineFlags := flag.NewFlagSet("combine", flag.ExitOnError)

	// Combine-specific flags
	inputDir := combineFlags.String("input", ".", "Directory containing CSV files to combine")
	outputFile := combineFlags.String("output", "combined_magazines", "Output filename (without .db extension)")
	debug := combineFlags.Bool("debug", false, "Enable debug logging")
	dryRun := combineFlags.Bool("dry-run", false, "Show what would be processed without actually combining")

	combineFlags.Usage = func() {
		_, err := fmt.Fprintf(os.Stderr, "Usage: %s combine [options]\n", os.Args[0])
		if err != nil {
			return
		}
		fmt.Print("\nCombine multiple magazine CSV files into a single SQLite database.\n")
		fmt.Print("\nExpected CSV format: Title,URL,Magazine,Date,Source\n")
		fmt.Print("\nOptions:\n")
		combineFlags.PrintDefaults()

		fmt.Print("\nExamples:\n")
		_, err = fmt.Fprintf(os.Stderr, "  %s combine -input ./magazines -output combined_clean\n", os.Args[0])
		if err != nil {
			return
		}
		_, err = fmt.Fprintf(os.Stderr, "  %s combine -input ./magazines -debug -dry-run\n", os.Args[0])
		if err != nil {
			return
		}
	}

	// Parse combine flags
	err := combineFlags.Parse(os.Args[2:])
	if err != nil {
		return
	}

	// Validate input directory exists
	if _, err := os.Stat(*inputDir); os.IsNotExist(err) {
		log.Fatalf("❌ Input directory does not exist: %s", *inputDir)
	}

	fmt.Printf("📁 CSV Combiner Starting...\n")
	fmt.Printf("📂 Input directory: %s\n", *inputDir)
	fmt.Printf("🎯 Output file: %s.db\n", *outputFile)

	startTime := time.Now()

	// Create CSV combiner
	combiner := pkg.NewCSVCombiner(*inputDir, *debug)

	// Process CSV files
	fmt.Printf("🔄 Processing CSV files...\n")
	articles, err := combiner.CombineCSVFiles()
	if err != nil {
		log.Fatalf("❌ Failed to combine CSV files: %v", err)
	}

	if len(articles) == 0 {
		log.Fatalf("❌ No valid articles found in CSV files")
	}

	fmt.Printf("✅ Successfully processed %d unique articles\n", len(articles))

	// Show statistics
	stats := combiner.GetStats()
	fmt.Printf("\n📊 Processing Statistics:\n")
	fmt.Printf("   • Unique URL+Title combinations: %d\n", stats["unique_url_titles"])
	fmt.Printf("   • Total records processed: %d\n", stats["total_processed"])

	// Show sample of cleaned URLs
	fmt.Printf("\n🔗 Sample of cleaned articles:\n")
	sampleCount := 5
	if len(articles) < sampleCount {
		sampleCount = len(articles)
	}

	for i := 0; i < sampleCount; i++ {
		article := articles[i]
		fmt.Printf("%d. %s\n", i+1, article.Title)
		fmt.Printf("   Clean URL: %s\n", article.URL)
		if article.Source != "" {
			fmt.Printf("   Source: %s\n", article.Source)
		}
		fmt.Printf("   From: %s\n", article.ScrapedFrom)
		fmt.Println()
	}

	if len(articles) > sampleCount {
		fmt.Printf("... and %d more\n\n", len(articles)-sampleCount)
	}

	// Dry run check
	if *dryRun {
		fmt.Printf("🏃 Dry run complete - no files were created\n")
		return
	}

	// Export to SQLite using existing exporter
	fmt.Printf("💾 Exporting to SQLite database...\n")
	outputPath := *outputFile + ".db"

	exporter := pkg.NewSQLiteExporter(outputPath)
	if err := exporter.Export(articles); err != nil {
		log.Fatalf("❌ Failed to export to SQLite: %v", err)
	}

	elapsed := time.Since(startTime)
	fmt.Printf("✅ Successfully exported to %s\n", outputPath)
	fmt.Printf("⏱️  Total processing time: %v\n", elapsed.Round(time.Second))

	// Provide usage tips
	fmt.Printf("\n💡 Usage tips:\n")
	fmt.Printf("   • Query with: sqlite3 %s\n", outputPath)
	fmt.Printf("   • View schema: sqlite3 %s \".schema\"\n", outputPath)
	fmt.Printf("   • Count articles: sqlite3 %s \"SELECT COUNT(*) FROM articles;\"\n", outputPath)
	fmt.Printf("   • Top sources: sqlite3 %s \"SELECT source, COUNT(*) FROM articles GROUP BY source ORDER BY COUNT(*) DESC LIMIT 10;\"\n", outputPath)

	fmt.Printf("\n🎉 CSV combination complete!\n")
}
