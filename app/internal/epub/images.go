package epub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"sync"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	// MaxImageWidth is the widest Supernote screen (A5X and Manta) in pixels.
	MaxImageWidth = 1404

	minImageSide       = 16
	maxImagePixels     = 40_000_000
	maxImageFetchBytes = 10 << 20
	imageFetchWorkers  = 4
	jpegQuality        = 75
)

var errImageSkipped = errors.New("image skipped")

// ImageFetcher downloads the raw bytes of an image referenced by an article.
type ImageFetcher interface {
	FetchImage(ctx context.Context, u *url.URL) ([]byte, error)
}

// HTTPImageFetcher fetches images over plain HTTP.
type HTTPImageFetcher struct {
	Client    *http.Client
	UserAgent string
}

func (f HTTPImageFetcher) FetchImage(ctx context.Context, u *url.URL) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "image/*")

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageFetchBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageFetchBytes {
		return nil, fmt.Errorf("%w: larger than %d bytes", errImageSkipped, maxImageFetchBytes)
	}
	return data, nil
}

type embeddedImage struct {
	source *url.URL
	href   string
	data   []byte
}

func fetchImages(ctx context.Context, fetcher ImageFetcher, sources []*url.URL) []embeddedImage {
	processed := make([][]byte, len(sources))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(imageFetchWorkers, len(sources)) {
		wg.Go(func() {
			for i := range jobs {
				raw, err := fetcher.FetchImage(ctx, sources[i])
				if err != nil {
					continue
				}
				if data, err := grayscaleJPEG(raw); err == nil {
					processed[i] = data
				}
			}
		})
	}
	for i := range sources {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	var images []embeddedImage
	for i, data := range processed {
		if data == nil {
			continue
		}
		images = append(images, embeddedImage{
			source: sources[i],
			href:   fmt.Sprintf("images/%03d.jpg", len(images)+1),
			data:   data,
		})
	}
	return images
}

func grayscaleJPEG(raw []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if cfg.Width < minImageSide || cfg.Height < minImageSide || cfg.Width*cfg.Height > maxImagePixels {
		return nil, errImageSkipped
	}

	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}

	width, height := cfg.Width, cfg.Height
	if width > MaxImageWidth {
		height = height * MaxImageWidth / width
		width = MaxImageWidth
	}

	dst := image.NewGray(image.Rect(0, 0, width, max(height, 1)))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
