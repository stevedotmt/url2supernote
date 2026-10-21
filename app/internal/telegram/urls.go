package telegram

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf16"
)

var bareURL = regexp.MustCompile(`https?://[^\s<>"]+`)

// MessageURLs returns every distinct http(s) URL in a message's text or caption, in order of appearance.
func MessageURLs(m *Message) []*url.URL {
	text, entities := m.Text, m.Entities
	if text == "" {
		text, entities = m.Caption, m.CaptionEntities
	}

	var candidates []string
	if len(entities) > 0 {
		units := utf16.Encode([]rune(text))
		for _, e := range entities {
			switch e.Type {
			case "url":
				if e.Offset >= 0 && e.Length > 0 && e.Offset+e.Length <= len(units) {
					candidates = append(candidates, string(utf16.Decode(units[e.Offset:e.Offset+e.Length])))
				}
			case "text_link":
				candidates = append(candidates, e.URL)
			}
		}
	} else {
		for _, match := range bareURL.FindAllString(text, -1) {
			candidates = append(candidates, strings.TrimRight(match, ".,;:!?)]}'"))
		}
	}

	var urls []*url.URL
	seen := map[string]bool{}
	for _, candidate := range candidates {
		u := parseHTTP(candidate)
		if u == nil || seen[u.String()] {
			continue
		}
		seen[u.String()] = true
		urls = append(urls, u)
	}
	return urls
}

func parseHTTP(s string) *url.URL {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil
	}
	return u
}
