package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"flag"
	"hash/crc32"
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
		data, err := grayscaleJPEG(context.Background(), pngImage(t, 32, 32, color.Transparent))
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
		data, err := grayscaleJPEG(context.Background(), pngImage(t, 300, 200, color.Black))
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
		if _, err := grayscaleJPEG(context.Background(), pngImage(t, 1, 1, color.Black)); !errors.Is(err, errImageSkipped) {
			t.Errorf("err = %v, want errImageSkipped", err)
		}
	})

	t.Run("rasterizes SVG", func(t *testing.T) {
		svg := `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="200" height="100" viewBox="0 0 200 100"><rect width="100" height="100" fill="black"/></svg>`
		data, err := grayscaleJPEG(context.Background(), []byte(svg))
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

	t.Run("rasterizes SVG text", func(t *testing.T) {
		svg := `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="100"><style>text { font-family: "Roboto", sans-serif; font-size: 40px; fill: #000000; }</style><text x="10" y="60">Hello</text></svg>`
		data, err := grayscaleJPEG(context.Background(), []byte(svg))
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		gray := img.(*image.Gray)
		dark := 0
		for _, p := range gray.Pix {
			if p < 100 {
				dark++
			}
		}
		if dark < 100 {
			t.Errorf("dark pixels = %d, want rendered text", dark)
		}
	})

	t.Run("rejects data that is not an image", func(t *testing.T) {
		if _, err := grayscaleJPEG(context.Background(), []byte("not an image")); err == nil {
			t.Error("err = nil")
		}
	})

	t.Run("skips images too large to decode without allocating them", func(t *testing.T) {
		if _, err := grayscaleJPEG(context.Background(), pngHeader(6000, 6000, 16)); !errors.Is(err, errImageSkipped) {
			t.Errorf("err = %v, want errImageSkipped", err)
		}
	})

	t.Run("skips SVGs too large to rasterize", func(t *testing.T) {
		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1e12"></svg>`
		if _, err := grayscaleJPEG(context.Background(), []byte(svg)); !errors.Is(err, errImageSkipped) {
			t.Errorf("err = %v, want errImageSkipped", err)
		}
	})

	t.Run("does not render files from the working directory into SVGs", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "secret.png"), pngImage(t, 64, 64, color.Black), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Chdir(dir)

		for _, href := range []string{"secret.png", " secret.png", "./secret.png", "file:///secret.png", "http://example.com/a.png"} {
			for _, attr := range []string{"xlink:href", "href"} {
				svg := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="64" height="64"><image ` + attr + `="` + href + `" width="64" height="64"/></svg>`
				if _, err := grayscaleJPEG(context.Background(), []byte(svg)); !errors.Is(err, errImageSkipped) {
					t.Errorf("%s=%q: err = %v, want errImageSkipped", attr, href, err)
				}
			}
		}
	})

	t.Run("skips SVGs that declare entities", func(t *testing.T) {
		svg := `<?xml version="1.0"?><!DOCTYPE svg [<!ENTITY a "aaaa">]><svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><text>&a;</text></svg>`
		if _, err := grayscaleJPEG(context.Background(), []byte(svg)); !errors.Is(err, errImageSkipped) {
			t.Errorf("err = %v, want errImageSkipped", err)
		}
	})

	t.Run("renders SVGs that reference only themselves and data URIs", func(t *testing.T) {
		svg := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="64" height="64"><defs><rect id="r" width="32" height="32"/></defs><use xlink:href="#r"/><image href="data:image/png;base64,` + base64.StdEncoding.EncodeToString(pngImage(t, 8, 8, color.Black)) + `" x="32" y="32" width="32" height="32"/></svg>`
		if _, err := grayscaleJPEG(context.Background(), []byte(svg)); err != nil {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("stops rendering an SVG when the context ends", func(t *testing.T) {
		if _, err := grayscaleJPEG(context.Background(), []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="10" height="10"/></svg>`)); err != nil {
			t.Fatal(err)
		}
		slow := `<svg xmlns="http://www.w3.org/2000/svg" width="4000" height="4000"><filter id="f" x="0" y="0" width="1" height="1"><feMorphology operator="dilate" radius="2000"/><feGaussianBlur stdDeviation="500"/><feMorphology operator="erode" radius="2000"/></filter><rect width="4000" height="4000" filter="url(#f)"/></svg>`
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		start := time.Now()
		_, err := grayscaleJPEG(ctx, []byte(slow))

		if err == nil || time.Since(start) > 5*time.Second {
			t.Errorf("returned after %s with err = %v, want an error shortly after the context ended", time.Since(start), err)
		}
		if _, err := grayscaleJPEG(context.Background(), []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="10" height="10"/></svg>`)); err != nil {
			t.Errorf("next SVG: err = %v", err)
		}
	})

	t.Run("waits for decode memory until the context ends", func(t *testing.T) {
		if err := decodeMemory.Acquire(context.Background(), maxDecodeBytes); err != nil {
			t.Fatal(err)
		}
		defer decodeMemory.Release(maxDecodeBytes)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if _, err := grayscaleJPEG(ctx, pngImage(t, 32, 32, color.Black)); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want context.DeadlineExceeded", err)
		}
	})
}

func pngHeader(width, height uint32, bitDepth byte) []byte {
	ihdr := binary.BigEndian.AppendUint32(nil, width)
	ihdr = binary.BigEndian.AppendUint32(ihdr, height)
	ihdr = append(ihdr, bitDepth, 6, 0, 0, 0)

	chunk := append([]byte("IHDR"), ihdr...)
	data := []byte("\x89PNG\r\n\x1a\n")
	data = binary.BigEndian.AppendUint32(data, uint32(len(ihdr)))
	data = append(data, chunk...)
	return binary.BigEndian.AppendUint32(data, crc32.ChecksumIEEE(chunk))
}

type panickingImages map[string][]byte

func (f panickingImages) FetchImage(_ context.Context, u *url.URL) ([]byte, error) {
	data, ok := f[u.String()]
	if !ok {
		panic("decoder bug")
	}
	return data, nil
}

func TestFetchImagesSkipsImagesThatPanic(t *testing.T) {
	good, _ := url.Parse("https://example.com/good.png")
	bad, _ := url.Parse("https://example.com/bad.png")
	fetcher := panickingImages{good.String(): pngImage(t, 32, 32, color.Black)}

	images := fetchImages(context.Background(), fetcher, []*url.URL{bad, good})

	if len(images) != 1 || images[0].source != good {
		t.Errorf("images = %v, want only %s", images, good)
	}
}
