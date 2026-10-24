package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stevedotmt/url2supernote/app/internal/article"
)

var update = flag.Bool("update", false, "rewrite golden files")

var savedAt = time.Date(2026, 10, 20, 8, 30, 0, 0, time.UTC)

type fakeImages map[string][]byte

func (f fakeImages) FetchImage(_ context.Context, u *url.URL) ([]byte, error) {
	if data, ok := f[u.String()]; ok {
		return data, nil
	}
	return nil, errors.New("not found")
}

func pngImage(t *testing.T, width, height int, fill color.Color) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sampleArticle() article.Article {
	return article.Article{
		URL:       &url.URL{Scheme: "https", Host: "blog.example.com", Path: "/posts/reading"},
		Title:     `Reading & "Writing" <Offline>`,
		Byline:    "Ada Lovelace",
		SiteName:  "Example Blog",
		Language:  "en-GB",
		Published: time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC),
		Content: `<div id="readability-page-1" class="page">
<p>First paragraph with a <a href="/about">relative link</a>, a <a href="javascript:alert(1)">script link</a> and an <a href="mailto:ada@example.com">email</a>.</p>
<script>alert("x")</script>
<iframe src="https://video.example.com"></iframe>
<figure><img src="/images/photo.png" alt="A photo"/><figcaption>Caption</figcaption></figure>
<p><img src="https://cdn.example.com/missing.png" alt="gone"/>Text after a missing image.</p>
<p><img src="data:image/png;base64,AAAA" alt="inline"/><custom-tag>Unknown <b>tags</b> are unwrapped.</custom-tag></p>
<p>Control` + "\x01" + ` characters vanish.<br>Line two.</p>
<table><tr><th colspan="2" style="color:red">Head</th></tr><tr><td>a</td><td>b</td></tr></table>
<ol start="3"><li>three</li></ol>
</div>`,
	}
}

func readZip(t *testing.T, data []byte) (*zip.Reader, map[string][]byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name] = content
	}
	return zr, files
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from golden file %s:\n%s", name, path, got)
	}
}

func assertWellFormed(t *testing.T, name string, content []byte) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(content))
	dec.Strict = true
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("%s is not well-formed XML: %v", name, err)
		}
	}
}

func TestBuild(t *testing.T) {
	b := NewBuilder(fakeImages{"https://blog.example.com/images/photo.png": pngImage(t, 3000, 1000, color.NRGBA{R: 200, A: 255})})

	data, err := b.Build(context.Background(), sampleArticle(), savedAt)
	if err != nil {
		t.Fatal(err)
	}

	zr, files := readZip(t, data)

	first := zr.File[0]
	if first.Name != "mimetype" || first.Method != zip.Store || len(first.Extra) != 0 || string(files["mimetype"]) != "application/epub+zip" {
		t.Errorf("first entry = %s (method %d, %d extra bytes), want stored mimetype without extra fields", first.Name, first.Method, len(first.Extra))
	}

	for _, name := range []string{"META-INF/container.xml", "OEBPS/content.opf", "OEBPS/article.xhtml", "OEBPS/nav.xhtml", "OEBPS/toc.ncx"} {
		assertWellFormed(t, name, files[name])
	}
	assertGolden(t, "content.opf", files["OEBPS/content.opf"])
	assertGolden(t, "article.xhtml", files["OEBPS/article.xhtml"])

	img, err := jpeg.Decode(bytes.NewReader(files["OEBPS/images/001.jpg"]))
	if err != nil {
		t.Fatalf("embedded image: %v", err)
	}
	if got := img.Bounds().Size(); got != (image.Point{X: MaxImageWidth, Y: 468}) {
		t.Errorf("image size = %v, want 1404x468", got)
	}
	if _, ok := img.(*image.Gray); !ok {
		t.Errorf("image is %T, want grayscale", img)
	}
	if len(files) != 8 {
		t.Errorf("EPUB has %d entries, want 8", len(files))
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	images := fakeImages{"https://blog.example.com/images/photo.png": pngImage(t, 64, 64, color.Black)}

	first, err := NewBuilder(images).Build(context.Background(), sampleArticle(), savedAt)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewBuilder(images).Build(context.Background(), sampleArticle(), savedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Error("two builds of the same article differ")
	}
}

func TestBuildRejectsTooManyImages(t *testing.T) {
	a := sampleArticle()
	a.Content = `<img src="a.png"/><img src="b.png"/><img src="a.png"/><img src="c.png"/>`
	b := NewBuilder(fakeImages{})
	b.MaxImages = 2

	_, err := b.Build(context.Background(), a, savedAt)

	if !errors.Is(err, ErrTooManyImages) {
		t.Errorf("err = %v, want ErrTooManyImages", err)
	}
}

func TestBuildRejectsLargeEPUB(t *testing.T) {
	b := NewBuilder(fakeImages{})
	b.MaxBytes = 1000

	_, err := b.Build(context.Background(), sampleArticle(), savedAt)

	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestBuildStopsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewBuilder(fakeImages{}).Build(ctx, sampleArticle(), savedAt)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestBuildFallsBackToSiteForTitleAndDefaultLanguage(t *testing.T) {
	a := sampleArticle()
	a.Title = ""
	a.Language = "not a language"

	data, err := NewBuilder(fakeImages{}).Build(context.Background(), a, savedAt)
	if err != nil {
		t.Fatal(err)
	}

	_, files := readZip(t, data)
	opf := string(files["OEBPS/content.opf"])
	if !strings.Contains(opf, "<dc:title>Example Blog</dc:title>") || !strings.Contains(opf, "<dc:language>en</dc:language>") {
		t.Errorf("content.opf = %s", opf)
	}
}

func TestGrayscaleJPEG(t *testing.T) {
	t.Run("composites transparency onto white", func(t *testing.T) {
		data, err := grayscaleJPEG(pngImage(t, 32, 32, color.Transparent))
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if y := img.(*image.Gray).GrayAt(16, 16).Y; y < 250 {
			t.Errorf("transparent pixel became %d, want white", y)
		}
	})

	t.Run("keeps small images at their size", func(t *testing.T) {
		data, err := grayscaleJPEG(pngImage(t, 300, 200, color.Black))
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Width != 300 || cfg.Height != 200 {
			t.Errorf("size = %dx%d, want 300x200", cfg.Width, cfg.Height)
		}
	})

	t.Run("skips tracking pixels", func(t *testing.T) {
		if _, err := grayscaleJPEG(pngImage(t, 1, 1, color.Black)); !errors.Is(err, errImageSkipped) {
			t.Errorf("err = %v, want errImageSkipped", err)
		}
	})

	t.Run("rasterizes SVG", func(t *testing.T) {
		svg := `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="200" height="100" viewBox="0 0 200 100"><rect width="100" height="100" fill="black"/></svg>`
		data, err := grayscaleJPEG([]byte(svg))
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 100 {
			t.Errorf("size = %v, want 200x100", b)
		}
		gray := img.(*image.Gray)
		if y := gray.GrayAt(50, 50).Y; y > 10 {
			t.Errorf("filled pixel = %d, want black", y)
		}
		if y := gray.GrayAt(150, 50).Y; y < 250 {
			t.Errorf("background pixel = %d, want white", y)
		}
	})

	t.Run("rejects data that is not an image", func(t *testing.T) {
		if _, err := grayscaleJPEG([]byte("not an image")); err == nil {
			t.Error("err = nil")
		}
	})
}
