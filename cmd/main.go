package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/slipperypenguin/flipboard-scraper/pkg"
)

// HTMLParser handles parsing of manual HTML exports
type HTMLParser struct {
	debug bool
}

// NewHTMLParser creates a new HTML parser
func NewHTMLParser(debug bool) *HTMLParser {
	return &HTMLParser{debug: debug}
}

// ParseHTMLFile parses a manually exported HTML file and extracts URLs
func (p *HTMLParser) ParseHTMLFile(filename string) ([]pkg.Article, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	doc, err := goquery.NewDocumentFromReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var articles []pkg.Article
	urlPattern := regexp.MustCompile(`https?://[^\s<>"']+`)

	if p.debug {
		log.Printf("Starting to parse HTML file: %s", filename)

		// Debug: count total elements
		totalItems := doc.Find("li.item-list__item").Length()
		log.Printf("Found %d total item-list elements", totalItems)
	}

	// Method 1: Look for URLs in specific CSS classes
	doc.Find("p.css-qfvlye, p.css-shlg1t").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if urlPattern.MatchString(text) {
			matches := urlPattern.FindAllString(text, -1)
			for _, match := range matches {
				if p.isValidURL(match) {
					article := pkg.Article{
						Title:       p.generateTitleFromURL(match),
						URL:         match,
						ActualURL:   match,
						Summary:     "",
						Date:        time.Now(),
						Source:      p.extractDomain(match),
						ScrapedFrom: filename,
					}

					p.enrichArticleFromContext(s, &article)
					articles = append(articles, article)

					if p.debug {
						log.Printf("Found URL in CSS class: %s", match)
					}
				}
			}
		}
	})

	// Method 2: Look for URLs in article content
	doc.Find("article").Each(func(i int, s *goquery.Selection) {
		s.Find("p, div, span").Each(func(j int, content *goquery.Selection) {
			text := strings.TrimSpace(content.Text())
			matches := urlPattern.FindAllString(text, -1)

			for _, match := range matches {
				if p.isValidURL(match) && !p.isDuplicate(articles, match) {
					article := pkg.Article{
						Title:       p.generateTitleFromURL(match),
						URL:         match,
						ActualURL:   match,
						Summary:     "",
						Date:        time.Now(),
						Source:      p.extractDomain(match),
						ScrapedFrom: filename,
					}

					p.enrichArticleFromContext(s, &article)
					articles = append(articles, article)

					if p.debug {
						log.Printf("Found URL in article: %s", match)
					}
				}
			}
		})
	})

	// Method 3: Scan ALL text content for missed URLs
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if len(text) > 10 && urlPattern.MatchString(text) {
			matches := urlPattern.FindAllString(text, -1)

			for _, match := range matches {
				if p.isValidURL(match) && !p.isDuplicate(articles, match) {
					article := pkg.Article{
						Title:       p.generateTitleFromURL(match),
						URL:         match,
						ActualURL:   match,
						Summary:     "",
						Date:        time.Now(),
						Source:      p.extractDomain(match),
						ScrapedFrom: filename,
					}

					articles = append(articles, article)

					if p.debug {
						log.Printf("Found URL in fallback scan: %s", match)
					}
				}
			}
		}
	})

	if p.debug {
		log.Printf("Total URLs extracted: %d", len(articles))
	}

	return articles, nil
}

func (p *HTMLParser) enrichArticleFromContext(selection *goquery.Selection, article *pkg.Article) {
	parent := selection.Parent()
	for i := 0; i < 5; i++ {
		if parent.Length() == 0 {
			break
		}

		parent.Find("time").Each(func(i int, timeEl *goquery.Selection) {
			timeText := strings.TrimSpace(timeEl.Text())
			if parsedTime, err := p.parseTimeString(timeText); err == nil {
				article.Date = parsedTime
			}
		})

		parent = parent.Parent()
	}
}

func (p *HTMLParser) parseTimeString(timeStr string) (time.Time, error) {
	formats := []string{
		"Jan 2, 2006",
		"January 2, 2006",
		"2006-01-02",
		"Mon Jan 2, 2006",
		"Sep 12, 2021",
		"May 16, 2021",
		"Dec 30, 2020",
		"Apr 17, 2020",
		"Feb 13, 2016",
		"Jan 28, 2016",
		"Dec 15, 2015",
		"Dec 8, 2015",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time: %s", timeStr)
}

func (p *HTMLParser) isValidURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	if u.Host == "" {
		return false
	}

	// Skip Flipboard URLs
	if strings.Contains(u.Host, "flipboard.com") {
		return false
	}

	// Skip invalid characters
	if strings.Contains(rawURL, " ") || strings.Contains(rawURL, "<") || strings.Contains(rawURL, ">") {
		return false
	}

	// Reasonable length check
	if len(rawURL) < 10 || len(rawURL) > 500 {
		return false
	}

	return true
}

func (p *HTMLParser) isDuplicate(articles []pkg.Article, checkURL string) bool {
	for _, article := range articles {
		if article.ActualURL == checkURL {
			return true
		}
	}
	return false
}

func (p *HTMLParser) generateTitleFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	title := u.Host
	if u.Path != "/" && u.Path != "" {
		path := strings.Trim(u.Path, "/")
		path = strings.ReplaceAll(path, "-", " ")
		path = strings.ReplaceAll(path, "_", " ")
		path = strings.Title(path)
		title = title + " - " + path
	}

	if len(title) > 0 {
		title = strings.ToUpper(title[:1]) + title[1:]
	}

	return title
}

func (p *HTMLParser) extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func main() {
	inputFile := flag.String("input", "", "Input HTML file to parse (required)")
	outputFormat := flag.String("format", "csv", "Output format: csv or sqlite")
	outputFile := flag.String("output", "flipboard_export", "Output filename (without extension)")
	debug := flag.Bool("debug", false, "Enable debug output")
	flag.Parse()

	if *inputFile == "" {
		log.Fatal("Input file is required. Use -input=filename.html")
	}

	if _, err := os.Stat(*inputFile); os.IsNotExist(err) {
		log.Fatalf("Input file does not exist: %s", *inputFile)
	}

	fmt.Printf("Parsing HTML export: %s\n", *inputFile)

	parser := NewHTMLParser(*debug)
	articles, err := parser.ParseHTMLFile(*inputFile)
	if err != nil {
		log.Fatalf("Failed to parse HTML file: %v", err)
	}

	if len(articles) == 0 {
		log.Fatal("No articles found in HTML file")
	}

	fmt.Printf("Extracted %d URLs from HTML file\n", len(articles))

	// Show sample
	fmt.Println("\nSample of extracted URLs:")
	sampleCount := 5
	if len(articles) < sampleCount {
		sampleCount = len(articles)
	}

	for i := 0; i < sampleCount; i++ {
		article := articles[i]
		fmt.Printf("%d. %s\n   URL: %s\n", i+1, article.Title, article.ActualURL)
	}

	if len(articles) > sampleCount {
		fmt.Printf("... and %d more\n", len(articles)-sampleCount)
	}

	// Export using your existing exporters
	fmt.Printf("\nExporting to %s format...\n", *outputFormat)

	switch *outputFormat {
	case "csv":
		exporter := pkg.NewCSVExporter(*outputFile + ".csv")
		if err := exporter.Export(articles); err != nil {
			log.Fatalf("Failed to export to CSV: %v", err)
		}
		fmt.Printf("Articles exported to %s.csv\n", *outputFile)

	case "sqlite":
		exporter := pkg.NewSQLiteExporter(*outputFile + ".db")
		if err := exporter.Export(articles); err != nil {
			log.Fatalf("Failed to export to SQLite: %v", err)
		}
		fmt.Printf("Articles exported to %s.db\n", *outputFile)

	default:
		log.Fatalf("Unsupported export format: %s", *outputFormat)
	}

	fmt.Println("HTML parsing and export complete!")

	// Generate stats
	stats := pkg.GenerateStats(articles)
	pkg.PrintStats(stats)
}
