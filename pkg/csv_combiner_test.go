package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCSVCombiner_ProcessCSVFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()

	// Create test CSV content
	testCSV := `Title,URL,Magazine,Date,Source
"Test Article 1","https://example.com/article1?utm_source=test&utm_campaign=social","Tech Magazine","2023-01-15","example.com"
"Test Article 2","https://blog.com/post2?fbclid=abc123&ref=twitter","Dev Blog","2023-01-16","blog.com"
"Duplicate Article","https://example.com/duplicate","Tech Magazine","2023-01-17","example.com"
"Duplicate Article","https://example.com/duplicate","Tech Magazine","2023-01-17","example.com"
"","","","",""`

	// Write test CSV file
	csvFile := filepath.Join(tempDir, "test.csv")
	err := os.WriteFile(csvFile, []byte(testCSV), 0644)
	if err != nil {
		t.Fatalf("Failed to write test CSV: %v", err)
	}

	// Create combiner and process file
	combiner := NewCSVCombiner(tempDir, true)
	articles, err := combiner.processCSVFile(csvFile)
	if err != nil {
		t.Fatalf("Failed to process CSV file: %v", err)
	}

	// Test expectations
	expectedCount := 3 // 2 unique articles + 1 duplicate that should be removed
	if len(articles) != expectedCount {
		t.Errorf("Expected %d articles, got %d", expectedCount, len(articles))
	}

	// Check URL cleaning
	for _, article := range articles {
		if strings.Contains(article.URL, "utm_") || strings.Contains(article.URL, "fbclid") {
			t.Errorf("URL was not properly cleaned: %s", article.URL)
		}
	}

	// Check that duplicate was removed (should only have 1 instance of "Duplicate Article")
	duplicateCount := 0
	for _, article := range articles {
		if article.Title == "Duplicate Article" {
			duplicateCount++
		}
	}
	if duplicateCount != 1 {
		t.Errorf("Expected 1 instance of duplicate article, got %d", duplicateCount)
	}
}

func TestCSVCombiner_ValidateHeader(t *testing.T) {
	combiner := NewCSVCombiner(".", false)

	tests := []struct {
		name     string
		header   []string
		expected bool
	}{
		{
			name:     "Valid header",
			header:   []string{"Title", "URL", "Magazine", "Date", "Source"},
			expected: true,
		},
		{
			name:     "Valid header with different case",
			header:   []string{"title", "url", "magazine", "date", "source"},
			expected: true,
		},
		{
			name:     "Valid header with extra spaces",
			header:   []string{" Title ", " URL ", " Magazine ", " Date ", " Source "},
			expected: true,
		},
		{
			name:     "Invalid header - wrong order",
			header:   []string{"URL", "Title", "Magazine", "Date", "Source"},
			expected: false,
		},
		{
			name:     "Invalid header - missing column",
			header:   []string{"Title", "URL", "Magazine", "Date"},
			expected: false,
		},
		{
			name:     "Invalid header - extra column",
			header:   []string{"Title", "URL", "Magazine", "Date", "Source", "Extra"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedHeader := []string{"Title", "URL", "Magazine", "Date", "Source"}
			result := combiner.validateHeader(tt.header, expectedHeader)
			if result != tt.expected {
				t.Errorf("validateHeader(%v) = %v, want %v", tt.header, result, tt.expected)
			}
		})
	}
}

func TestCSVCombiner_CreateDedupeKey(t *testing.T) {
	combiner := NewCSVCombiner(".", false)

	tests := []struct {
		name        string
		url1        string
		title1      string
		url2        string
		title2      string
		shouldMatch bool
	}{
		{
			name:        "Exact match",
			url1:        "https://example.com",
			title1:      "Test Article",
			url2:        "https://example.com",
			title2:      "Test Article",
			shouldMatch: true,
		},
		{
			name:        "Case insensitive match",
			url1:        "https://Example.COM",
			title1:      "Test Article",
			url2:        "https://example.com",
			title2:      "test article",
			shouldMatch: true,
		},
		{
			name:        "Different URLs",
			url1:        "https://example.com",
			title1:      "Test Article",
			url2:        "https://different.com",
			title2:      "Test Article",
			shouldMatch: false,
		},
		{
			name:        "Different titles",
			url1:        "https://example.com",
			title1:      "Test Article 1",
			url2:        "https://example.com",
			title2:      "Test Article 2",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1 := combiner.createDedupeKey(tt.url1, tt.title1)
			key2 := combiner.createDedupeKey(tt.url2, tt.title2)

			match := key1 == key2
			if match != tt.shouldMatch {
				t.Errorf("Expected match=%v for keys '%s' and '%s'", tt.shouldMatch, key1, key2)
			}
		})
	}
}

func TestCSVCombiner_CombineCSVFiles(t *testing.T) {
	// Create a temporary directory with multiple CSV files
	tempDir := t.TempDir()

	// Create first CSV file
	csv1 := `Title,URL,Magazine,Date,Source
"Article 1","https://example.com/1?utm_source=test","Magazine A","2023-01-15","example.com"
"Article 2","https://blog.com/2","Magazine A","2023-01-16","blog.com"`

	// Create second CSV file with one duplicate
	csv2 := `Title,URL,Magazine,Date,Source
"Article 3","https://news.com/3","Magazine B","2023-01-17","news.com"
"Article 1","https://example.com/1?utm_source=test","Magazine B","2023-01-15","example.com"`

	// Write CSV files
	err := os.WriteFile(filepath.Join(tempDir, "magazine1.csv"), []byte(csv1), 0644)
	if err != nil {
		t.Fatalf("Failed to write CSV1: %v", err)
	}

	err = os.WriteFile(filepath.Join(tempDir, "magazine2.csv"), []byte(csv2), 0644)
	if err != nil {
		t.Fatalf("Failed to write CSV2: %v", err)
	}

	// Create combiner and process all files
	combiner := NewCSVCombiner(tempDir, true)
	articles, err := combiner.CombineCSVFiles()
	if err != nil {
		t.Fatalf("Failed to combine CSV files: %v", err)
	}

	// Should have 3 unique articles (duplicate "Article 1" removed)
	expectedCount := 3
	if len(articles) != expectedCount {
		t.Errorf("Expected %d articles, got %d", expectedCount, len(articles))
	}

	// Check that URLs were cleaned
	for _, article := range articles {
		if strings.Contains(article.URL, "utm_") {
			t.Errorf("URL not cleaned properly: %s", article.URL)
		}
	}
}

// Benchmark test for large CSV processing
func BenchmarkCSVCombiner_ProcessLargeFile(b *testing.B) {
	tempDir := b.TempDir()

	// Create a large CSV file
	var csvContent strings.Builder
	csvContent.WriteString("Title,URL,Magazine,Date,Source\n")

	for i := 0; i < 1000; i++ {
		csvContent.WriteString(fmt.Sprintf("\"Article %d\",\"https://example.com/article%d?utm_source=test\",\"Magazine\",\"2023-01-15\",\"example.com\"\n", i, i))
	}

	csvFile := filepath.Join(tempDir, "large.csv")
	err := os.WriteFile(csvFile, []byte(csvContent.String()), 0644)
	if err != nil {
		b.Fatalf("Failed to write large CSV: %v", err)
	}

	combiner := NewCSVCombiner(tempDir, false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := combiner.processCSVFile(csvFile)
		if err != nil {
			b.Fatalf("Failed to process large CSV: %v", err)
		}
	}
}
