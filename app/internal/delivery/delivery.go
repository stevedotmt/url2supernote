// Package delivery turns an Article into a file in the Owner's Destination.
package delivery

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime/debug"
	"time"

	"github.com/stevedotmt/url2supernote/app/internal/article"
	"github.com/stevedotmt/url2supernote/app/internal/epub"
	"github.com/stevedotmt/url2supernote/app/internal/extract"
	"github.com/stevedotmt/url2supernote/app/internal/syncengine"
	"github.com/stevedotmt/url2supernote/app/internal/webclient"
)

// DefaultBudget bounds a whole Delivery, from fetching the Article to storing the file.
const DefaultBudget = 45 * time.Second

// Outcome is how a Delivery ended.
type Outcome string

const (
	Delivered Outcome = "delivered"
	Duplicate Outcome = "duplicate"
	Failed    Outcome = "failed"
)

// Result describes a finished Delivery.
type Result struct {
	URL      *url.URL
	Title    string
	Filename string
	Outcome  Outcome
	// Reason is a short, Owner-facing explanation of a failed Delivery.
	Reason string
	Err    error
}

// Extractor turns a URL into a readable Article.
type Extractor interface {
	Extract(ctx context.Context, u *url.URL) (article.Article, error)
}

// Builder renders an Article as a file the Supernote can read.
type Builder interface {
	Build(ctx context.Context, a article.Article, savedAt time.Time) ([]byte, error)
}

// Service performs Deliveries.
type Service struct {
	Extractor Extractor
	Builder   Builder
	Engine    syncengine.Engine
	Budget    time.Duration
	Now       func() time.Time
}

// Deliver runs one Delivery for the Article at u. A panic while handling the untrusted page fails the
// Delivery instead of crashing the process.
func (s *Service) Deliver(ctx context.Context, u *url.URL) (result Result) {
	ctx, cancel := context.WithTimeout(ctx, s.Budget)
	defer cancel()

	result = Result{URL: u}
	defer func() {
		if r := recover(); r != nil {
			result = failed(result, s.Budget, fmt.Errorf("panic: %v\n%s", r, debug.Stack()))
		}
	}()

	a, err := s.Extractor.Extract(ctx, u)
	if err != nil {
		return failed(result, s.Budget, fmt.Errorf("extract: %w", err))
	}
	result.Title = a.Title
	result.Filename = Filename(a)

	exists, err := s.Engine.Exists(ctx, result.Filename)
	if err != nil {
		return failed(result, s.Budget, fmt.Errorf("check destination: %w", err))
	}
	if exists {
		result.Outcome = Duplicate
		return result
	}

	data, err := s.Builder.Build(ctx, a, s.Now())
	if err != nil {
		return failed(result, s.Budget, fmt.Errorf("build: %w", err))
	}

	if err := s.Engine.Put(ctx, result.Filename, data); err != nil {
		return failed(result, s.Budget, fmt.Errorf("store: %w", err))
	}

	result.Outcome = Delivered
	return result
}

func failed(r Result, budget time.Duration, err error) Result {
	r.Outcome = Failed
	r.Err = err
	r.Reason = reason(err, budget)
	return r
}

func reason(err error, budget time.Duration) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("took longer than %s", budget)
	case errors.Is(err, extract.ErrUnreadable):
		return "couldn't extract a readable article"
	case errors.Is(err, extract.ErrNotHTML):
		return "the link is not a web page"
	case errors.Is(err, extract.ErrPageTooLarge):
		return "the page is too large"
	case errors.Is(err, webclient.ErrNonPublicAddress):
		return "the link points to a private or local address"
	case errors.Is(err, extract.ErrFetch):
		return "couldn't fetch the page"
	case errors.Is(err, epub.ErrTooManyImages):
		return "the article has too many images"
	case errors.Is(err, epub.ErrTooLarge):
		return "the article is too large"
	case errors.Is(err, syncengine.ErrUnauthorized):
		return "Sync Engine authorization expired"
	case errors.Is(err, syncengine.ErrDestinationNotFound):
		return "the Destination folder is gone; run `url2supernote auth google` and pick it again"
	default:
		return "something went wrong"
	}
}
