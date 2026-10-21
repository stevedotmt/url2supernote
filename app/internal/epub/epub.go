// Package epub packages an article.Article as an EPUB 3 file suited to a Supernote.
package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"embed"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/stevedotmt/url2supernote/app/internal/article"
)

const (
	DefaultMaxImages = 50
	DefaultMaxBytes  = 15 << 20
	defaultLanguage  = "en"
)

var (
	ErrTooManyImages = errors.New("the article has too many images")
	ErrTooLarge      = errors.New("the EPUB is too large")
)

//go:embed templates
var templateFS embed.FS

var templates = template.Must(template.New("epub").Funcs(template.FuncMap{"x": escape}).ParseFS(templateFS, "templates/*.tmpl"))

var languageTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{1,8})*$`)

// Builder packages articles as EPUB files.
type Builder struct {
	Images    ImageFetcher
	MaxImages int
	MaxBytes  int
}

// NewBuilder returns a Builder with the default image and size limits.
func NewBuilder(images ImageFetcher) *Builder {
	return &Builder{Images: images, MaxImages: DefaultMaxImages, MaxBytes: DefaultMaxBytes}
}

// Build renders a as an EPUB, embedding its images in grayscale at Supernote resolution.
func (b *Builder) Build(ctx context.Context, a article.Article, savedAt time.Time) ([]byte, error) {
	nodes, err := parseFragment(a.Content)
	if err != nil {
		return nil, fmt.Errorf("parse article content: %w", err)
	}

	sources := imageURLs(nodes, a.URL)
	if len(sources) > b.MaxImages {
		return nil, fmt.Errorf("%w: %d, the limit is %d", ErrTooManyImages, len(sources), b.MaxImages)
	}

	images := fetchImages(ctx, b.Images, sources)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	hrefs := make(map[string]string, len(images))
	for _, img := range images {
		hrefs[img.source.String()] = img.href
	}

	data, err := pack(newPage(a, savedAt, renderXHTML(nodes, a.URL, hrefs), images), images, savedAt)
	if err != nil {
		return nil, err
	}
	if len(data) > b.MaxBytes {
		return nil, fmt.Errorf("%w: %d bytes, the limit is %d", ErrTooLarge, len(data), b.MaxBytes)
	}
	return data, nil
}

type manifestImage struct {
	ID   string
	Href string
}

type page struct {
	Identifier  string
	Language    string
	Title       string
	Byline      string
	Site        string
	URL         string
	Published   string
	PublishedOn string
	Modified    string
	SavedOn     string
	Body        string
	Images      []manifestImage
}

func newPage(a article.Article, savedAt time.Time, body string, images []embeddedImage) page {
	p := page{
		Identifier: identifier(a.URL.String()),
		Language:   language(a.Language),
		Title:      a.Title,
		Byline:     a.Byline,
		Site:       a.Site(),
		URL:        a.URL.String(),
		Modified:   savedAt.UTC().Format(time.RFC3339),
		SavedOn:    savedAt.UTC().Format("2 January 2006"),
		Body:       body,
	}
	if p.Title == "" {
		p.Title = p.Site
	}
	if !a.Published.IsZero() {
		p.Published = a.Published.UTC().Format("2006-01-02")
		p.PublishedOn = a.Published.UTC().Format("2 January 2006")
	}
	for i, img := range images {
		p.Images = append(p.Images, manifestImage{ID: fmt.Sprintf("img%03d", i+1), Href: img.href})
	}
	return p
}

func identifier(source string) string {
	sum := sha1.Sum([]byte(source))
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

func language(tag string) string {
	if languageTag.MatchString(tag) {
		return tag
	}
	return defaultLanguage
}

func pack(p page, images []embeddedImage, modified time.Time) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	write := func(header *zip.FileHeader, content []byte) error {
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = w.Write(content)
		return err
	}
	add := func(name string, method uint16, content []byte) error {
		return write(&zip.FileHeader{Name: name, Method: method, Modified: modified}, content)
	}

	if err := write(&zip.FileHeader{Name: "mimetype", Method: zip.Store}, []byte("application/epub+zip")); err != nil {
		return nil, err
	}
	for _, f := range []struct{ name, source string }{
		{"META-INF/container.xml", "templates/container.xml"},
		{"OEBPS/style.css", "templates/style.css"},
	} {
		content, err := templateFS.ReadFile(f.source)
		if err != nil {
			return nil, err
		}
		if err := add(f.name, zip.Deflate, content); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"content.opf", "article.xhtml", "nav.xhtml", "toc.ncx"} {
		var rendered strings.Builder
		if err := templates.ExecuteTemplate(&rendered, name+".tmpl", p); err != nil {
			return nil, fmt.Errorf("render %s: %w", name, err)
		}
		if err := add("OEBPS/"+name, zip.Deflate, []byte(rendered.String())); err != nil {
			return nil, err
		}
	}
	for _, img := range images {
		if err := add("OEBPS/"+img.href, zip.Store, img.data); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
