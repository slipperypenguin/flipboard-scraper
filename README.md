# flipboard-scraper
A Go application to facilitate [Flipboard](https://flipboard.com) magazine exports


## 🎯 How It Works
### **Phase 1: RSS for Recent Content**
- Fetches the magazine's RSS feed (recent 10-25 articles)
- Fast and reliable for new content
- Note: RSS feeds only contain recent articles, not the complete magazine history. To get ALL historical content, this scraper uses a hybrid approach with both RSS and HTML scraping.

### **Phase 2: HTML Scraping for Historical Archives**
- Scrapes the actual Flipboard magazine pages
- Supports pagination to get ALL historical articles
- Uses respectful rate limiting and error handling
- Can optionally use JavaScript rendering for dynamic content

### **Phase 3: Smart Deduplication**
- Combines results from both approaches
- Removes duplicates based on article URLs
- Provides complete, comprehensive archive

## ✨ Features
- **Complete Historical Archive** - Gets ALL articles, not just recent ones
- **Hybrid Scraping Strategy** - RSS + HTML pagination for maximum coverage
- **Multiple Export Formats** - CSV and SQLite with rich metadata
- **Concurrent Processing** - Configurable parallel requests for speed
- **Smart Rate Limiting** - Respectful to Flipboard's servers
- **Pagination Support** - Automatically navigates through all magazine pages
- **Error Recovery** - Continues scraping even if some pages fail
- **Deduplication** - Intelligent removal of duplicate articles
- **Flexible Configuration** - Highly customizable scraping behavior
- **Debug Mode** - Verbose logging for troubleshooting


## 📋 Installation
1. Clone the repository:
```bash
git clone https://github.com/slipperypenguin/flipboard-scraper.git
cd flipboard-scraper
```

2. Install dependencies:
```bash
go mod tidy
```

3. Build the application:
```bash
go build -o flipboard-scraper cmd/main.go
```


## 🚀 Usage
### **Basic Usage (Complete Archive)**
```bash
./flipboard-scraper -urls="https://flipboard.com/@sliperrypenguin/code-blog-learning-6evfsnosy"
```

### **Advanced Configuration**
```bash
./flipboard-scraper \
  -urls="https://flipboard.com/@user/magazine1,https://flipboard.com/@user/magazine2" \
  -format=sqlite \
  -output=complete_archive \
  -concurrent=3 \
  -rate-limit=1.0 \
  -max-pages=50 \
  -timeout=600 \
  -debug=true
```

### **Command Line Options**
| Flag | Description | Default | Notes |
|------|-------------|---------|-------|
| `-urls` | Comma-separated Flipboard magazine URLs (required) | - | Complete magazine URLs |
| `-format` | Export format: `csv` or `sqlite` | `csv` | SQLite recommended for large datasets |
| `-output` | Output filename (without extension) | `articles` | Will create .csv or .db file |
| `-concurrent` | Maximum concurrent requests | `3` | Higher = faster, but more load on servers |
| `-rate-limit` | Maximum requests per second | `1.0` | Be respectful to avoid blocking |
| `-timeout` | Total timeout in seconds | `300` | Increase for large magazines |
| `-max-pages` | Maximum pages to scrape per magazine | `10` | Set to 0 for unlimited |
| `-debug` | Enable verbose logging | `false` | Helpful for troubleshooting |
| `-user-agent` | Custom User-Agent string | (Chrome default) | Use realistic browser agent |

### **Examples**
**Complete Archive of Single Magazine:**
```bash
./flipboard-scraper \
  -urls="https://flipboard.com/@techcrunch/startup-news" \
  -format=sqlite \
  -max-pages=0 \
  -debug=true
```

**Multiple Magazines with Custom Limits:**
```bash
./flipboard-scraper \
  -urls="https://flipboard.com/@wired/ai,https://flipboard.com/@verge/tech" \
  -format=csv \
  -max-pages=25 \
  -concurrent=2 \
  -rate-limit=0.5
```

**Fast Recent Content Only:**
```bash
./flipboard-scraper \
  -urls="https://flipboard.com/@magazine" \
  -max-pages=1 \
  -rate-limit=2.0
```


## 📊 Output Formats
### **CSV Export**
- **Title** - Article headline
- **URL** - Original article link
- **Summary** - Article description/excerpt
- **Date** - Publication date (RFC3339 format)
- **Author** - Article author/source
- **ImageURL** - Featured image URL
- **Source** - Publication source
- **ScrapedFrom** - "rss" or "html" (for tracking data source)

### **SQLite Database**
Recommended for large datasets and complex queries:
- All CSV fields plus database features
- **Indexes** on URL, date, and source for fast queries
- **Unique constraints** to prevent duplicates
- **Full-text search** capabilities
- **Efficient storage** for thousands of articles

**Sample SQLite Queries:**
```sql
-- Get all articles from last 90 days
SELECT * FROM articles 
WHERE date > datetime('now', '-90 days') 
ORDER BY date DESC;

-- Find articles by keyword in title
SELECT title, url, date FROM articles 
WHERE title LIKE '%AI%' OR title LIKE '%artificial intelligence%';

-- Count articles by source
SELECT source, COUNT(*) as article_count 
FROM articles 
GROUP BY source 
ORDER BY article_count DESC;

-- Get articles scraped via HTML (historical content)
SELECT COUNT(*) FROM articles WHERE scraped_from = 'html';
```


## 🔧 Technical Architecture
### **Hybrid Scraping Strategy**
```
1. RSS Phase (Fast)
   ├── Fetch magazine.rss
   ├── Parse recent 10-25 articles
   └── Extract structured metadata

2. HTML Phase (Complete)
   ├── Navigate to magazine page
   ├── Extract articles from HTML
   ├── Follow pagination links
   ├── Continue until all pages scraped
   └── Handle dynamic content loading

3. Consolidation
   ├── Merge RSS + HTML results
   ├── Deduplicate by URL
   ├── Enrich with metadata
   └── Export to chosen format
```

### **Pagination Detection**
The scraper automatically detects and follows:
- **Next page buttons** (`.next`, `.pagination`)
- **Load more buttons** (`.load-more`)
- **URL-based pagination** (`?page=N`)
- **API endpoints** (if discovered)

### **Error Handling**
- **Network timeouts** - Automatic retries with backoff
- **Rate limiting** - Intelligent throttling to avoid blocks
- **Missing pages** - Graceful handling of 404s
- **Malformed content** - Continues scraping valid articles
- **Context cancellation** - Clean shutdown on Ctrl+C


## ⚠️ Important Considerations
### **Respectful Scraping**
- **Rate limiting** is enforced to avoid overwhelming servers
- **User-Agent** identifies the scraper appropriately
- **Timeout handling** prevents hanging requests
- **Error recovery** minimizes retry storms

### **Content Volume Expectations**
- **Small magazines** (< 100 articles): Complete in < 1 minute
- **Medium magazines** (100-1000 articles): 5-15 minutes
- **Large magazines** (> 1000 articles): 30+ minutes
- **Use `-max-pages`** to limit scraping scope if needed

### **Legal & Ethical Usage**
- Respects publicly available content only
- No bypassing of authentication or paywalls
- Rate limiting prevents server overload
- Users responsible for compliance with ToS


## 🐛 Troubleshooting
### **"No articles found"**
```bash
# Enable debug mode to see what's happening
./flipboard-scraper -urls="YOUR_URL" -debug=true

# Try with just recent content first
./flipboard-scraper -urls="YOUR_URL" -max-pages=1
```

### **"Rate limited" or timeouts**
```bash
# Reduce rate and increase timeout
./flipboard-scraper -urls="YOUR_URL" -rate-limit=0.5 -timeout=900
```

### **"Too many pages"**
```bash
# Limit the scope
./flipboard-scraper -urls="YOUR_URL" -max-pages=20
```

### **Debug Information**
With `-debug=true`, you'll see:
- Which URLs are being accessed
- How many articles found per page
- Pagination detection results
- Error details and retry attempts


## 📄 License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
