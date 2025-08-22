# flipboard-exporter
A Go application to facilitate [Flipboard](https://flipboard.com) magazine exports.

Flipboard doesn't provide a native way to locally export magazine articles, nor do they provide a developer API to accomplish this. This app was created to address that gap.

## Features
- Complete historical archive - gets ALL articles, not just recent ones
- URL decoding, cleaning, and deduplication
- JavaScript-powered extraction using [`chromedp`](github.com/chromedp/chromedp) for dynamic content
  - Enhanced article parsing directly from DOM with improved selectors
  - Smart rate limiting that is respectful to Flipboard's servers
- 'quick sample' mode for testing without full extraction
- Multiple export formats
  - CSV with decoded URL columns
  - SQLite with indexes, metadata, and duplicate prevention
- Manual login support for adhering to Flipboard's authentication requirements

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
go build -o flipboard-scraper cmd/flipboard-scraper/main.go
```

## 🧪 Testing
Run the comprehensive test suite:
```bash
go test ./pkg -v
```

## 🚀 Usage
### Basic Usage (Complete Archive)
```bash
./flipboard-scraper -urls="https://flipboard.com/@sliperrypenguin/code-blog-learning-6evfsnosy"
```

### Quick Sample (Test Mode)
```bash
./flipboard-scraper \
  -urls="https://flipboard.com/@user/magazine" \
  -quick-sample=true \
  -debug=true
```

### Advanced Options
```bash
./flipboard-scraper \
  -urls="https://flipboard.com/@user/magazine1,https://flipboard.com/@user/magazine2" \
  -format=sqlite \
  -output=complete_archive \
  -max-scrolls=300 \
  -scroll-delay=5 \
  -timeout=2400 \
  -debug=true
```

### Command Line Options
| Flag | Description | Default | Notes |
|------|-------------|---------|-------|
| `-urls` | Comma-separated Flipboard magazine URLs (required) | - | Complete magazine URLs |
| `-concurrent` | Maximum concurrent requests | `1` | Keep low (1-2) for chromedp stability |
| `-debug` | Enable verbose logging | `false` | Helpful for troubleshooting |
| `-format` | Export format: `csv` or `sqlite` | `csv` | SQLite recommended for large datasets |
| `-filename` | Output filename (without extension) | `articles` | Will create .csv or .db file |
| `-max-scrolls` | Maximum scroll attempts for infinite scroll | `150` | 0 = no scrolling, higher = more complete |
| `-quick-sample` | Quick sample mode (first page only) | `false` | For testing purposes |
| `-rate-limit` | Maximum requests per second | `0.5` | Lower values more respectful |
| `-scroll-delay` | Delay between scrolls in seconds | `3` | Increase for slow-loading content |
| `-timeout` | Total timeout in seconds | `900` | Increase for large magazines |
| `-user-agent` | Custom User-Agent string | (Chrome default) | Use realistic browser agent |

#### Optimization Tips
- For large magazines: Increase `-max-scrolls=300` and `-scroll-delay=5`
- For slow networks: Increase `-timeout=2400` (40 minutes)
- For debugging: Enable `-debug=true`. Will also show real-time progress.
- For testing: Use `-quick-sample=true`
- Troubleshooting timeout errors: Increase `-timeout` and reduce `-concurrent` to 1
- Troubleshooting incomplete results: Increase `-max-scrolls` and `-scroll-delay`

## 💡 Pro Tips
- Always test with `-quick-sample=true` first
- Large magazines can take 30+ minutes with full scrolling, please be patient!
- Use SQLite for large datasets (>500 articles)
- Keep rate limits low and concurrent requests minimal in order to respect rate limits

### Sample Enhanced SQLite Queries:
```bash
sqlite3 research_archive.db "SELECT source, COUNT(*) FROM articles GROUP BY source ORDER BY COUNT(*) DESC;"
```

```sql
-- Get all articles with successfully decoded URLs
SELECT title, actual_url, source, date
FROM articles
WHERE actual_url IS NOT NULL AND actual_url != ''
ORDER BY date DESC;

-- Find articles by decoded source domain
SELECT source, COUNT(*) as article_count,
       AVG(CASE WHEN actual_url != '' THEN 1.0 ELSE 0.0 END) as decode_rate
FROM articles
GROUP BY source
HAVING article_count > 5
ORDER BY decode_rate DESC;

-- Quality analysis: decode success rate
SELECT
    COUNT(*) as total_articles,
    SUM(CASE WHEN actual_url != '' THEN 1 ELSE 0 END) as decoded_articles,
    ROUND(100.0 * SUM(CASE WHEN actual_url != '' THEN 1 ELSE 0 END) / COUNT(*), 1) as decode_percentage
FROM articles;

-- Top sources by article count
SELECT source, COUNT(*) as articles,
       GROUP_CONCAT(DISTINCT title, ' | ') as sample_titles
FROM articles
WHERE source IS NOT NULL
GROUP BY source
ORDER BY articles DESC
LIMIT 10;
```
