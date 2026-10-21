// Package article holds the readable form of an Article as extracted from the web.
package article

import (
	"net/url"
	"time"
)

// Article is the readable content of a web page the Owner wants on their Supernote.
type Article struct {
	URL       *url.URL
	Title     string
	Byline    string
	SiteName  string
	Language  string
	Published time.Time
	// Content is an HTML fragment containing only the article body.
	Content string
}

// Site returns the site name, falling back to the host when the page does not declare one.
func (a Article) Site() string {
	if a.SiteName != "" {
		return a.SiteName
	}
	if a.URL == nil {
		return ""
	}
	return a.URL.Hostname()
}
