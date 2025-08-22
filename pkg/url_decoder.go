package pkg

import (
	"net/url"
	"strings"
)

// DecodeFlipboardURL extracts the actual URL from Flipboard redirect URLs
func DecodeFlipboardURL(flipboardURL string) string {
	if flipboardURL == "" {
		return ""
	}

	// Parse the Flipboard URL
	u, err := url.Parse(flipboardURL)
	if err != nil {
		return ""
	}

	// Look for the 'url' parameter in Flipboard redirect URLs
	if strings.Contains(flipboardURL, "/redirect?url=") {
		query := u.Query()
		encodedURL := query.Get("url")
		if encodedURL != "" {
			decoded, err := url.QueryUnescape(encodedURL)
			if err == nil {
				return decoded
			}
		}
	}

	// If it's a Flipboard story URL, try to extract from path
	if strings.Contains(u.Path, "/story/") || strings.Contains(u.Path, "/@") {
		// Some Flipboard URLs encode the original URL in the path
		// For now, return the Flipboard URL - could be enhanced later
		return flipboardURL
	}

	return ""
}

// CleanURL removes tracking parameters from URLs
func CleanURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// List of tracking parameters to remove
	trackingParams := map[string]bool{
		"utm_source":   true,
		"utm_medium":   true,
		"utm_campaign": true,
		"utm_content":  true,
		"utm_term":     true,
		"fbclid":       true,
		"gclid":        true,
		"ref":          true,
		"referrer":     true,
		"source":       true,
		"campaign":     true,
		"mc_cid":       true,
		"mc_eid":       true,
		"_ga":          true,
		"_gid":         true,
	}

	// Clean query parameters
	query := u.Query()
	for param := range query {
		if trackingParams[param] {
			query.Del(param)
		}
	}

	u.RawQuery = query.Encode()
	return u.String()
}

// ExtractSourceFromURL extracts the hostname from a URL to use as source
func ExtractSourceFromURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	return u.Hostname()
}
