package pkg

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Exporter interface for different export formats
type Exporter interface {
	Export(articles []Article) error
}

// CSVExporter handles exporting articles to CSV format with URL decoding support
type CSVExporter struct {
	filename string
}

// NewCSVExporter creates a new CSV exporter
func NewCSVExporter(filename string) *CSVExporter {
	return &CSVExporter{filename: filename}
}

// Export writes articles to a CSV file with decoded URLs
func (e *CSVExporter) Export(articles []Article) error {
	file, err := os.Create(e.filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			panic(err)
		}
	}(file)

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header with new URL decoding fields
	if err := writer.Write([]string{
		"Title",
		"URL",          // Final URL (ActualURL if available, otherwise original URL)
		"Original_URL", // Original Flipboard URL
		"Actual_URL",   // Decoded external URL
		"Summary",
		"Date",
		"Source",
		"ScrapedFrom",
	}); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data
	for _, article := range articles {
		// Use actual URL if available, otherwise use original URL
		finalURL := article.ActualURL
		if finalURL == "" {
			finalURL = article.URL
		}

		if err := writer.Write([]string{
			article.Title,
			finalURL,
			article.URL,
			article.ActualURL,
			article.Summary,
			article.Date.Format(time.RFC3339),
			article.Source,
			article.ScrapedFrom,
		}); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	return nil
}

// SQLiteExporter handles exporting articles to SQLite database with URL decoding support
type SQLiteExporter struct {
	dbPath string
}

// NewSQLiteExporter creates a new SQLite exporter
func NewSQLiteExporter(dbPath string) *SQLiteExporter {
	return &SQLiteExporter{dbPath: dbPath}
}

// Export writes articles to a SQLite database with consistent schema
func (e *SQLiteExporter) Export(articles []Article) error {
	db, err := sql.Open("sqlite3", e.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			panic(err)
		}
	}(db)

	// Create table with given schema for URL decoding
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS articles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			url TEXT,                -- Final URL (actual_url if available, otherwise original_url)
			original_url TEXT,       -- Original Flipboard URL
			actual_url TEXT,         -- Decoded external URL
			summary TEXT,
			date DATETIME,
			source TEXT,
			scraped_from TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(actual_url, title) ON CONFLICT IGNORE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Create indexes for better query performance
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_articles_url ON articles(url)`,
		`CREATE INDEX IF NOT EXISTS idx_articles_actual_url ON articles(actual_url)`,
		`CREATE INDEX IF NOT EXISTS idx_articles_source ON articles(source)`,
		`CREATE INDEX IF NOT EXISTS idx_articles_date ON articles(date)`,
		`CREATE INDEX IF NOT EXISTS idx_articles_scraped_from ON articles(scraped_from)`,
	}

	for _, indexSQL := range indexes {
		if _, err := db.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Prepare insert statement
	stmt, err := db.Prepare(`
		INSERT INTO articles (
			title, url, original_url, actual_url, summary, date,
			source, scraped_from
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func(stmt *sql.Stmt) {
		err := stmt.Close()
		if err != nil {
			panic(err)
		}
	}(stmt)

	// Insert articles
	for _, article := range articles {
		// Use actual URL if available, otherwise use original URL
		finalURL := article.ActualURL
		if finalURL == "" {
			finalURL = article.URL
		}

		_, err = stmt.Exec(
			article.Title,
			finalURL,
			article.URL,
			article.ActualURL,
			article.Summary,
			article.Date,
			article.Source,
			article.ScrapedFrom,
		)
		if err != nil {
			return fmt.Errorf("failed to insert article: %w", err)
		}
	}

	return nil
}

// ExportStats provides statistics about the exported articles
type ExportStats struct {
	TotalArticles   int
	ExternalURLs    int
	WithSummaries   int
	SourceBreakdown map[string]int
	ScrapedFrom     map[string]int
}

// GenerateStats generates export statistics for the given articles
func GenerateStats(articles []Article) ExportStats {
	stats := ExportStats{
		TotalArticles:   len(articles),
		SourceBreakdown: make(map[string]int),
		ScrapedFrom:     make(map[string]int),
	}

	for _, article := range articles {
		// Count external vs Flipboard URLs
		if article.ActualURL != "" && !strings.Contains(article.ActualURL, "flipboard.com") {
			stats.ExternalURLs++
		}

		// Count articles with summaries
		if article.Summary != "" {
			stats.WithSummaries++
		}

		// Track source breakdown
		if article.Source != "" {
			stats.SourceBreakdown[article.Source]++
		}

		// Track scraping method
		stats.ScrapedFrom[article.ScrapedFrom]++
	}

	return stats
}

// PrintStats prints export statistics in a readable format
func PrintStats(stats ExportStats) {
	fmt.Printf("📈 Export Statistics:\n")
	fmt.Printf("   • Total articles: %d\n", stats.TotalArticles)
	fmt.Printf("   • Articles with external URLs: %d (%.1f%%)\n",
		stats.ExternalURLs,
		float64(stats.ExternalURLs)*100/float64(stats.TotalArticles))
	fmt.Printf("   • Articles with summaries: %d (%.1f%%)\n",
		stats.WithSummaries,
		float64(stats.WithSummaries)*100/float64(stats.TotalArticles))

	if len(stats.SourceBreakdown) > 0 {
		fmt.Printf("   • Top sources:\n")
		count := 0
		for source, num := range stats.SourceBreakdown {
			if count >= 5 { // Show top 5 sources
				break
			}
			fmt.Printf("     - %s: %d articles\n", source, num)
			count++
		}
	}
}
