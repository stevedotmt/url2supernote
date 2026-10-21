// Package extract turns a web page into an article.Article using reader-mode extraction.
package extract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"codeberg.org/readeck/go-readability/v2"
	"golang.org/x/net/html/charset"

	"github.com/stevedotmt/url2supernote/app/internal/article"
)

var (
	ErrFetch        = errors.New("could not fetch the page")
	ErrNotHTML      = errors.New("the page is not HTML")
	ErrPageTooLarge = errors.New("the page is too large")
	ErrUnreadable   = errors.New("could not extract a readable article")
)

const (
	DefaultMaxBytes   = 5 << 20
	DefaultUserAgent  = "Mozilla/5.0 (compatible; url2supernote; +https://github.com/stevedotmt/url2supernote)"
	minReadableLength = 200
)

// Extractor fetches pages over plain HTTP and extracts their readable content.
type Extractor struct {
	Client    *http.Client
	MaxBytes  int64
	UserAgent string
}

// New returns an Extractor with the default page size limit and user agent.
func New(client *http.Client) *Extractor {
	return &Extractor{Client: client, MaxBytes: DefaultMaxBytes, UserAgent: DefaultUserAgent}
}

// Extract fetches the page at u and returns its readable content.
func (e *Extractor) Extract(ctx context.Context, u *url.URL) (article.Article, error) {
	body, finalURL, contentType, err := e.fetch(ctx, u)
	if err != nil {
		return article.Article{}, err
	}

	decoded, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return article.Article{}, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}

	parsed, err := readability.FromReader(decoded, finalURL)
	if err != nil {
		return article.Article{}, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if parsed.Node == nil {
		return article.Article{}, ErrUnreadable
	}

	var text strings.Builder
	if err := parsed.RenderText(&text); err != nil {
		return article.Article{}, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if len(strings.TrimSpace(text.String())) < minReadableLength {
		return article.Article{}, ErrUnreadable
	}

	var content strings.Builder
	if err := parsed.RenderHTML(&content); err != nil {
		return article.Article{}, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}

	published, err := parsed.PublishedTime()
	if err != nil {
		published = time.Time{}
	}

	return article.Article{
		URL:       finalURL,
		Title:     strings.TrimSpace(parsed.Title()),
		Byline:    strings.TrimSpace(parsed.Byline()),
		SiteName:  strings.TrimSpace(parsed.SiteName()),
		Language:  strings.TrimSpace(parsed.Language()),
		Published: published,
		Content:   content.String(),
	}, nil
}

func (e *Extractor) fetch(ctx context.Context, u *url.URL) ([]byte, *url.URL, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, "", fmt.Errorf("%w: %w", ErrFetch, err)
	}
	req.Header.Set("User-Agent", e.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.1")

	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, nil, "", fmt.Errorf("%w: %w", ErrFetch, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, nil, "", fmt.Errorf("%w: HTTP %d", ErrFetch, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !isHTML(contentType) {
		return nil, nil, "", fmt.Errorf("%w: %s", ErrNotHTML, contentType)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, e.MaxBytes+1))
	if err != nil {
		return nil, nil, "", fmt.Errorf("%w: %w", ErrFetch, err)
	}
	if int64(len(body)) > e.MaxBytes {
		return nil, nil, "", ErrPageTooLarge
	}

	return body, resp.Request.URL, contentType, nil
}

func isHTML(contentType string) bool {
	if contentType == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}
