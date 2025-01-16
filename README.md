# flipboard-scraper
A Go application to facilitate [Flipboard](https://flipboard.com) magazine exports

## Features
- Web scraping using [colly](github.com/gocolly/colly/v2), which handles JavaScript-rendered content
- Support exports to both CSV and SQLite formats
- Error handling, input validation, and test coverage
- Rate Limiting: Configurable requests per second via the `-rate-limit` flag. Rate limiting applies across all concurrent requests
- Error Handling:
	- Context support for cancellation and timeouts
	- Basic error handling and input validation
	- Graceful shutdown on interrupt signals
	- Warning instead of fatal error if some URLs fail
- Concurrent Scraping:
	- Support for multiple URLs via the `-urls` flag
	- Configurable concurrency via the `-concurrent` flag
	- Configurable timeout via the `-timeout` flag
	- Uses `errgroup` for controlled concurrent execution
	- Mutex protection for shared data



## Usage
Install the required dependencies:
	`go mod tidy`


Run the program with:
```
go run cmd/main.go -url "https://flipboard.com/@sliperrypenguin/code-blog-learning-6evfsnosy"
```


By default, it will export to CSV. To export to SQLite:
```
go run cmd/main.go -url "https://flipboard.com/@sliperrypenguin/code-blog-learning-6evfsnosy" -format sqlite
```


You can also use the scraper with multiple URLs and configure its behavior:
```
./flipboard-scraper -urls="https://flipboard.com/magazine1,https://flipboard.com/magazine2" -concurrent=3 -rate-limit=2 -timeout=180
```

