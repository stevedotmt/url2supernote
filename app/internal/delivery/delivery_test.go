package delivery_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/stevedotmt/url2supernote/app/internal/article"
	"github.com/stevedotmt/url2supernote/app/internal/delivery"
	"github.com/stevedotmt/url2supernote/app/internal/epub"
	"github.com/stevedotmt/url2supernote/app/internal/extract"
	"github.com/stevedotmt/url2supernote/app/internal/syncengine"
)

var savedAt = time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)

type fakeExtractor struct {
	article article.Article
	err     error
}

func (f fakeExtractor) Extract(_ context.Context, u *url.URL) (article.Article, error) {
	if f.err != nil {
		return article.Article{}, f.err
	}
	a := f.article
	a.URL = u
	return a, nil
}

type fakeBuilder struct {
	err     error
	savedAt time.Time
}

func (f *fakeBuilder) Build(_ context.Context, a article.Article, at time.Time) ([]byte, error) {
	f.savedAt = at
	if f.err != nil {
		return nil, f.err
	}
	return []byte("epub:" + a.Title), nil
}

type fakeEngine struct {
	files     map[string][]byte
	existsErr error
	putErr    error
}

func newFakeEngine(names ...string) *fakeEngine {
	e := &fakeEngine{files: map[string][]byte{}}
	for _, name := range names {
		e.files[name] = []byte("existing")
	}
	return e
}

func (f *fakeEngine) Exists(_ context.Context, name string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	_, ok := f.files[name]
	return ok, nil
}

func (f *fakeEngine) Put(_ context.Context, name string, content []byte) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.files[name] = content
	return nil
}

var articleURL = &url.URL{Scheme: "https", Host: "example.com", Path: "/post"}

func newService(ex delivery.Extractor, b delivery.Builder, e syncengine.Engine) *delivery.Service {
	return &delivery.Service{
		Extractor: ex,
		Builder:   b,
		Engine:    e,
		Budget:    delivery.DefaultBudget,
		Now:       func() time.Time { return savedAt },
	}
}

func TestDeliverStoresNewArticle(t *testing.T) {
	engine := newFakeEngine()
	builder := &fakeBuilder{}
	svc := newService(fakeExtractor{article: article.Article{Title: "Hello", SiteName: "Example"}}, builder, engine)

	got := svc.Deliver(context.Background(), articleURL)

	if got.Outcome != delivery.Delivered {
		t.Fatalf("outcome = %s (%v), want delivered", got.Outcome, got.Err)
	}
	if got.Filename != "Hello — Example.epub" || got.Title != "Hello" {
		t.Errorf("filename, title = %q, %q", got.Filename, got.Title)
	}
	if string(engine.files[got.Filename]) != "epub:Hello" {
		t.Errorf("stored %q", engine.files[got.Filename])
	}
	if !builder.savedAt.Equal(savedAt) {
		t.Errorf("built with savedAt %v, want %v", builder.savedAt, savedAt)
	}
}

func TestDeliverSkipsExistingFile(t *testing.T) {
	engine := newFakeEngine("Hello — Example.epub")
	svc := newService(fakeExtractor{article: article.Article{Title: "Hello", SiteName: "Example"}}, &fakeBuilder{}, engine)

	got := svc.Deliver(context.Background(), articleURL)

	if got.Outcome != delivery.Duplicate {
		t.Fatalf("outcome = %s, want duplicate", got.Outcome)
	}
	if string(engine.files["Hello — Example.epub"]) != "existing" {
		t.Error("existing file was overwritten")
	}
}

func TestDeliverFailures(t *testing.T) {
	ok := fakeExtractor{article: article.Article{Title: "Hello"}}
	tests := []struct {
		name       string
		extractor  fakeExtractor
		builder    *fakeBuilder
		engine     *fakeEngine
		wantReason string
	}{
		{"unreadable", fakeExtractor{err: extract.ErrUnreadable}, &fakeBuilder{}, newFakeEngine(), "couldn't extract a readable article"},
		{"not html", fakeExtractor{err: fmt.Errorf("%w: application/pdf", extract.ErrNotHTML)}, &fakeBuilder{}, newFakeEngine(), "the link is not a web page"},
		{"fetch", fakeExtractor{err: fmt.Errorf("%w: HTTP 404", extract.ErrFetch)}, &fakeBuilder{}, newFakeEngine(), "couldn't fetch the page"},
		{"page too large", fakeExtractor{err: extract.ErrPageTooLarge}, &fakeBuilder{}, newFakeEngine(), "the page is too large"},
		{"too many images", ok, &fakeBuilder{err: epub.ErrTooManyImages}, newFakeEngine(), "the article has too many images"},
		{"epub too large", ok, &fakeBuilder{err: epub.ErrTooLarge}, newFakeEngine(), "the article is too large"},
		{"auth on exists", ok, &fakeBuilder{}, &fakeEngine{existsErr: syncengine.ErrUnauthorized}, "Sync Engine authorization expired"},
		{"auth on put", ok, &fakeBuilder{}, &fakeEngine{files: map[string][]byte{}, putErr: syncengine.ErrUnauthorized}, "Sync Engine authorization expired"},
		{"no destination", ok, &fakeBuilder{}, &fakeEngine{existsErr: syncengine.ErrDestinationNotFound}, "the Destination folder doesn't exist on the Sync Engine"},
		{"budget", ok, &fakeBuilder{err: context.DeadlineExceeded}, newFakeEngine(), "took longer than 45s"},
		{"unknown", ok, &fakeBuilder{err: errors.New("boom")}, newFakeEngine(), "something went wrong"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newService(tt.extractor, tt.builder, tt.engine).Deliver(context.Background(), articleURL)

			if got.Outcome != delivery.Failed {
				t.Fatalf("outcome = %s, want failed", got.Outcome)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if got.Err == nil {
				t.Error("Err is nil")
			}
		})
	}
}

type blockingExtractor struct{}

func (blockingExtractor) Extract(ctx context.Context, _ *url.URL) (article.Article, error) {
	<-ctx.Done()
	return article.Article{}, ctx.Err()
}

func TestDeliverEnforcesBudget(t *testing.T) {
	svc := newService(blockingExtractor{}, &fakeBuilder{}, newFakeEngine())
	svc.Budget = 10 * time.Millisecond

	got := svc.Deliver(context.Background(), articleURL)

	if got.Outcome != delivery.Failed || got.Reason != "took longer than 10ms" {
		t.Errorf("got %s %q", got.Outcome, got.Reason)
	}
}
