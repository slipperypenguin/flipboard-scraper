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

// CSVExporter handles exporting articles to CSV format
type CSVExporter struct {
	filename string
}

// NewCSVExporter creates a new CSV exporter
func NewCSVExporter(filename string) *CSVExporter {
	return &CSVExporter{filename: filename}
}

// Export writes articles to a CSV file
func (e *CSVExporter) Export(articles []Article) error {
	file, err := os.Create(e.filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header with new fields
	if err := writer.Write([]string{
		"Title", 
		"URL", 
		"Summary", 
		"Date", 
		"Author", 
		"ImageURL",
		"Source",
		"GUID",
		"ScrapedFrom",
	}); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data
	for _, article := range articles {
		if err := writer.Write([]string{
			article.Title,
			article.URL,
			article.Summary,
			article.Date.Format(time.RFC3339),
			article.Author,
			article.ImageURL,
			article.Source,
			article.GUID,
			article.ScrapedFrom,
		}); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	return nil
}

// SQLiteExporter handles exporting articles to SQLite database
type SQLiteExporter struct {
	dbPath string
}

// NewSQLiteExporter creates a new SQLite exporter
func NewSQLiteExporter(dbPath string) *SQLiteExporter {
	return &SQLiteExporter{dbPath: dbPath}
}

// Export writes articles to a SQLite database
func (e *SQLiteExporter) Export(articles []Article) error {
	db, err := sql.Open("sqlite3", e.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create table with enhanced schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS articles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			url TEXT,
			summary TEXT,
			date DATETIME,
			author TEXT,
			image_url TEXT,
			source TEXT,
			guid TEXT,
			scraped_from TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(url) ON CONFLICT REPLACE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Create index on common query fields
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_articles_date ON articles(date);
		CREATE INDEX IF NOT EXISTS idx_articles_author ON articles(author);
		CREATE INDEX IF NOT EXISTS idx_articles_source ON articles(source);
		CREATE INDEX IF NOT EXISTS idx_articles_scraped_from ON articles(scraped_from);
		CREATE INDEX IF NOT EXISTS idx_articles_url ON articles(url);
	`)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	// Insert articles
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO articles (title, url, summary, date, author, image_url, source, guid, scraped_from)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, article := range articles {
		_, err := stmt.Exec(
			article.Title,
			article.URL,
			article.Summary,
			article.Date,
			article.Author,
			article.ImageURL,
			article.Source,
			article.GUID,
			article.ScrapedFrom,
		)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to insert article: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
