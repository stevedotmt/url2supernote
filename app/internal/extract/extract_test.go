package extract_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stevedotmt/url2supernote/app/internal/extract"
)

func serve(t *testing.T, handler http.HandlerFunc) *url.URL {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func page(t *testing.T, contentType string, body []byte) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
	}
}

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/article.html")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExtract(t *testing.T) {
	var userAgent string
	base := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/posts/paper", http.StatusMovedPermanently)
			return
		}
		userAgent = r.UserAgent()
		page(t, "text/html; charset=utf-8", fixture(t))(w, r)
	})

	got, err := extract.New(http.DefaultClient).Extract(context.Background(), base.JoinPath("old"))
	if err != nil {
		t.Fatal(err)
	}

	if got.URL.Path != "/posts/paper" {
		t.Errorf("URL = %s, want the redirect target", got.URL)
	}
	if got.Title != "Why Paper Still Wins" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.SiteName != "Field Notes" || got.Byline != "Grace Hopper" || got.Language != "en" {
		t.Errorf("SiteName, Byline, Language = %q, %q, %q", got.SiteName, got.Byline, got.Language)
	}
	if want := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC); !got.Published.Equal(want) {
		t.Errorf("Published = %v, want %v", got.Published, want)
	}
	for _, want := range []string{"E-ink devices close much of that gap", base.String() + "/img/desk.jpg"} {
		if !strings.Contains(got.Content, want) {
			t.Errorf("Content is missing %q", want)
		}
	}
	for _, unwanted := range []string{"window.tracking", "Subscribe", "All rights reserved", "Another post"} {
		if strings.Contains(got.Content, unwanted) {
			t.Errorf("Content contains clutter %q", unwanted)
		}
	}
	if userAgent != extract.DefaultUserAgent {
		t.Errorf("User-Agent = %q", userAgent)
	}
}

func TestExtractDecodesDeclaredCharset(t *testing.T) {
	latin1 := strings.ReplaceAll(string(fixture(t)), `<meta charset="utf-8">`, "")
	latin1 = strings.ReplaceAll(latin1, "Why Paper Still Wins</h1>", "Caf\xe9 Paper</h1>")
	base := serve(t, page(t, "text/html; charset=iso-8859-1", []byte(latin1)))

	got, err := extract.New(http.DefaultClient).Extract(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Content+got.Title, "Café") {
		t.Errorf("Latin-1 text was not decoded: title %q", got.Title)
	}
}

func TestExtractFailures(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    error
	}{
		{"not found", func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }, extract.ErrFetch},
		{"pdf", page(t, "application/pdf", []byte("%PDF-1.7")), extract.ErrNotHTML},
		{"too large", page(t, "text/html", []byte(strings.Repeat("a", 2048))), extract.ErrPageTooLarge},
		{"no article", page(t, "text/html", []byte("<html><body><p>Hi.</p></body></html>")), extract.ErrUnreadable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := extract.New(http.DefaultClient)
			e.MaxBytes = 1024

			_, err := e.Extract(context.Background(), serve(t, tt.handler))

			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestExtractUnreachable(t *testing.T) {
	base := serve(t, page(t, "text/html", nil))
	base.Host = "127.0.0.1:1"

	_, err := extract.New(http.DefaultClient).Extract(context.Background(), base)

	if !errors.Is(err, extract.ErrFetch) {
		t.Errorf("err = %v, want ErrFetch", err)
	}
}
