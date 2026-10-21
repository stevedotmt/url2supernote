package delivery_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stevedotmt/url2supernote/app/internal/article"
	"github.com/stevedotmt/url2supernote/app/internal/delivery"
)

func TestFilename(t *testing.T) {
	host := &url.URL{Scheme: "https", Host: "www.example.com"}
	tests := []struct {
		name string
		a    article.Article
		want string
	}{
		{"title and site", article.Article{Title: "Hello World", SiteName: "The Blog"}, "Hello World — The Blog.epub"},
		{"falls back to host", article.Article{Title: "Hello", URL: host}, "Hello — www.example.com.epub"},
		{"no title", article.Article{URL: host}, "www.example.com.epub"},
		{"nothing", article.Article{}, "Article.epub"},
		{"unsafe characters", article.Article{Title: `A/B: "C" <D>?`, SiteName: "S|T"}, "A B C D — S T.epub"},
		{"whitespace and control characters", article.Article{Title: "  Two\n\tlines\x00 ", SiteName: "Site"}, "Two lines — Site.epub"},
		{"trailing dots", article.Article{Title: "Wait...", SiteName: "Site"}, "Wait — Site.epub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := delivery.Filename(tt.a); got != tt.want {
				t.Errorf("Filename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFilenameTruncatesByRunes(t *testing.T) {
	got := delivery.Filename(article.Article{Title: strings.Repeat("é", 300), SiteName: strings.Repeat("s", 300)})

	want := strings.Repeat("é", 100) + " — " + strings.Repeat("s", 40) + ".epub"
	if got != want {
		t.Errorf("Filename() = %q, want %q", got, want)
	}
}
