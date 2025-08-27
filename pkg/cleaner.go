// pkg/brady_cleaner.go - Targeted cleaning for Brady suffix removal
package pkg

import (
	"regexp"
	"strings"
)

// BradyCleaner handles removal of Brady-style suffixes from your magazine data
type BradyCleaner struct {
	// Regex for detecting Brady patterns
	bradyRegex *regexp.Regexp
}

// NewBradyCleaner creates a new Brady cleaner
func NewBradyCleaner() *BradyCleaner {
	// Match Brady patterns at the end of strings
	// Handles: Brady, BradyJul, BradyAvatarSF, BradyFDRJul, BradyE, etc.
	// With or without separators: /Brady, -Brady, Brady (direct append)
	regexPattern := `[/\-\s]*Brady[A-Za-z0-9]*\s*$`
	regex := regexp.MustCompile(regexPattern)

	return &BradyCleaner{
		bradyRegex: regex,
	}
}

// CleanTitle removes Brady suffixes from titles while preserving the format
func (bc *BradyCleaner) CleanTitle(rawTitle string) string {
	if rawTitle == "" {
		return ""
	}

	title := strings.TrimSpace(rawTitle)

	// Remove Brady patterns from the end
	cleaned := bc.bradyRegex.ReplaceAllString(title, "")

	return strings.TrimSpace(cleaned)
}

// CleanURL removes Brady suffixes and tracking parameters from URLs
func (bc *BradyCleaner) CleanURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	// Remove Brady patterns first
	cleanURL := bc.bradyRegex.ReplaceAllString(rawURL, "")
	cleanURL = strings.TrimSpace(cleanURL)

	// Then use existing CleanURL function for tracking parameters
	return CleanURL(cleanURL)
}

// CleanBoth cleans both URL and title, removing Brady patterns
func (bc *BradyCleaner) CleanBoth(rawURL, rawTitle string) (cleanURL, cleanTitle string) {
	cleanURL = bc.CleanURL(rawURL)
	cleanTitle = bc.CleanTitle(rawTitle)
	return cleanURL, cleanTitle
}

// IsBradyPattern checks if a string contains Brady patterns (for debugging)
func (bc *BradyCleaner) IsBradyPattern(text string) bool {
	return bc.bradyRegex.MatchString(text)
}
