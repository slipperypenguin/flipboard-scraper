package pkg

import (
	"encoding/csv"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CSVRecord represents a record from the magazine CSV files
type CSVRecord struct {
	Title    string
	URL      string
	Magazine string
	Date     string
	Source   string
}

// CSVCombiner handles combining multiple CSV files
type CSVCombiner struct {
	inputDir  string
	debug     bool
	dedupeMap map[string]bool // key: URL+Title for deduplication
	cleaner   *BradyCleaner   // Brady suffix cleaner
}

// NewCSVCombiner creates a new CSV combiner
func NewCSVCombiner(inputDir string, debug bool) *CSVCombiner {
	return &CSVCombiner{
		inputDir:  inputDir,
		debug:     debug,
		dedupeMap: make(map[string]bool),
		cleaner:   NewBradyCleaner(),
	}
}

// CombineCSVFiles reads all CSV files from directory and combines them
func (c *CSVCombiner) CombineCSVFiles() ([]Article, error) {
	var allArticles []Article
	csvFiles, err := c.findCSVFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to find CSV files: %w", err)
	}

	if c.debug {
		fmt.Printf("Found %d CSV files to process\n", len(csvFiles))
	}

	for _, csvFile := range csvFiles {
		if c.debug {
			fmt.Printf("Processing: %s\n", csvFile)
		}

		articles, err := c.processCSVFile(csvFile)
		if err != nil {
			fmt.Printf("Warning: failed to process %s: %v\n", csvFile, err)
			continue
		}

		allArticles = append(allArticles, articles...)
		if c.debug {
			fmt.Printf("  Added %d articles\n", len(articles))
		}
	}

	if c.debug {
		fmt.Printf("Total articles before deduplication: %d\n", len(allArticles))
		fmt.Printf("Unique articles after deduplication: %d\n", len(allArticles))
	}

	return allArticles, nil
}

// findCSVFiles recursively finds all CSV files in the input directory
func (c *CSVCombiner) findCSVFiles() ([]string, error) {
	var csvFiles []string

	err := filepath.WalkDir(c.inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(strings.ToLower(path), ".csv") {
			csvFiles = append(csvFiles, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(csvFiles) == 0 {
		return nil, fmt.Errorf("no CSV files found in directory: %s", c.inputDir)
	}

	return csvFiles, nil
}

// processCSVFile processes a single CSV file and converts to Article structs
func (c *CSVCombiner) processCSVFile(filename string) ([]Article, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Validate header format
	expectedHeader := []string{"Title", "URL", "Magazine", "Date", "Source"}
	if !c.validateHeader(header, expectedHeader) {
		return nil, fmt.Errorf("invalid header format. Expected: %v, Got: %v", expectedHeader, header)
	}

	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV records: %w", err)
	}

	var articles []Article
	for i, record := range records {
		if len(record) != 5 {
			if c.debug {
				fmt.Printf("  Skipping row %d: invalid column count (%d)\n", i+2, len(record))
			}
			continue
		}

		csvRecord := CSVRecord{
			Title:    strings.TrimSpace(record[0]),
			URL:      strings.TrimSpace(record[1]),
			Magazine: strings.TrimSpace(record[2]),
			Date:     strings.TrimSpace(record[3]),
			Source:   strings.TrimSpace(record[4]),
		}

		// Skip empty records
		if csvRecord.Title == "" || csvRecord.URL == "" {
			continue
		}

		// Clean the URL and title using Brady cleaning logic
		cleanedURL, cleanedTitle := c.cleaner.CleanBoth(csvRecord.URL, csvRecord.Title)

		// Check for duplicates (URL + Title combination) using cleaned data
		dedupeKey := c.createDedupeKey(cleanedURL, cleanedTitle)
		if c.dedupeMap[dedupeKey] {
			if c.debug {
				fmt.Printf("  Skipping duplicate: %s\n", cleanedTitle)
			}
			continue
		}
		c.dedupeMap[dedupeKey] = true

		// Convert to Article struct
		article, err := c.convertToArticle(csvRecord, cleanedURL, cleanedTitle, filename)
		if err != nil {
			if c.debug {
				fmt.Printf("  Warning: failed to convert record %d: %v\n", i+2, err)
			}
			continue
		}

		articles = append(articles, article)
	}

	return articles, nil
}

// validateHeader checks if the CSV header matches expected format
func (c *CSVCombiner) validateHeader(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}

	for i, expectedCol := range expected {
		actualCol := strings.TrimSpace(actual[i])
		if !strings.EqualFold(actualCol, expectedCol) {
			return false
		}
	}

	return true
}

// createDedupeKey creates a key for deduplication based on URL and Title
func (c *CSVCombiner) createDedupeKey(url, title string) string {
	// Normalize for comparison
	normalizedURL := strings.ToLower(strings.TrimSpace(url))
	normalizedTitle := strings.ToLower(strings.TrimSpace(title))
	return normalizedURL + "|" + normalizedTitle
}

// convertToArticle converts CSVRecord to Article struct
func (c *CSVCombiner) convertToArticle(csvRecord CSVRecord, cleanedURL, cleanedTitle, sourceFile string) (Article, error) {
	// Parse date
	var parsedDate time.Time
	var err error

	// Try multiple date formats
	dateFormats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
		"01/02/2006",
		"1/2/2006",
		"2006/01/02",
	}

	for _, format := range dateFormats {
		parsedDate, err = time.Parse(format, csvRecord.Date)
		if err == nil {
			break
		}
	}

	// If all date parsing failed, use current time and log warning
	if err != nil {
		if c.debug {
			fmt.Printf("  Warning: could not parse date '%s', using current time\n", csvRecord.Date)
		}
		parsedDate = time.Now()
	}

	// Extract source from cleaned URL if CSV source is empty
	finalSource := csvRecord.Source
	if finalSource == "" && cleanedURL != "" {
		finalSource = ExtractSourceFromURL(cleanedURL)
	}

	// Create Article struct compatible with existing schema
	article := Article{
		Title:       cleanedTitle, // Use cleaned title
		URL:         cleanedURL,   // Use cleaned URL as primary URL
		ActualURL:   cleanedURL,   // Same as URL since these are direct URLs
		Summary:     "",           // Not available in CSV
		Date:        parsedDate,
		Source:      finalSource,
		ScrapedFrom: fmt.Sprintf("csv:%s", filepath.Base(sourceFile)),
	}

	return article, nil
}

// GetStats returns statistics about the combination process
func (c *CSVCombiner) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_processed":   len(c.dedupeMap),
		"unique_url_titles": len(c.dedupeMap),
	}
}
