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
	"golang.org/x/sync/semaphore"
)

const (
	// MaxImageWidth is the widest Supernote screen (A5X and Manta) in pixels.
	MaxImageWidth = 1404

	minImageSide       = 16
	maxImagePixels     = 40_000_000
	maxImageFetchBytes = 10 << 20
	maxSVGBytes        = 2 << 20
	maxDecodeBytes     = 128 << 20
	imageFetchWorkers  = 4
	jpegQuality        = 75
)

var errImageSkipped = errors.New("image skipped")

var decodeMemory = semaphore.NewWeighted(maxDecodeBytes)

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
				processed[i] = fetchGrayscaleJPEG(ctx, fetcher, sources[i])
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

func fetchGrayscaleJPEG(ctx context.Context, fetcher ImageFetcher, source *url.URL) (data []byte) {
	defer func() {
		if recover() != nil {
			data = nil
		}
	}()
	raw, err := fetcher.FetchImage(ctx, source)
	if err != nil {
		return nil
	}
	data, err = grayscaleJPEG(ctx, raw)
	if err != nil {
		return nil
	}
	return data
}

type sourceImage struct {
	width, height int
	bytesPerPixel int64
	decode        func() (image.Image, error)
}

func grayscaleJPEG(ctx context.Context, raw []byte) ([]byte, error) {
	src, err := inspectImage(raw)
	if err != nil {
		return nil, err
	}
	if src.width < minImageSide || src.height < minImageSide || int64(src.width)*int64(src.height) > maxImagePixels {
		return nil, errImageSkipped
	}

	width, height := src.width, src.height
	if width > MaxImageWidth {
		height = height * MaxImageWidth / width
		width = MaxImageWidth
	}
	height = max(height, 1)

	cost := int64(src.width)*int64(src.height)*src.bytesPerPixel + int64(width)*int64(height)
	if cost > maxDecodeBytes {
		return nil, errImageSkipped
	}
	if err := decodeMemory.Acquire(ctx, cost); err != nil {
		return nil, err
	}
	defer decodeMemory.Release(cost)

	decoded, err := src.decode()
	if err != nil {
		return nil, err
	}

	dst := image.NewGray(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), decoded, decoded.Bounds(), draw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func inspectImage(raw []byte) (sourceImage, error) {
	if isSVG(raw) {
		return inspectSVG(raw)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return sourceImage{}, err
	}
	return sourceImage{
		width:         cfg.Width,
		height:        cfg.Height,
		bytesPerPixel: bytesPerPixel(cfg.ColorModel),
		decode: func() (image.Image, error) {
			img, _, err := image.Decode(bytes.NewReader(raw))
			return img, err
		},
	}, nil
}

func bytesPerPixel(model color.Model) int64 {
	if _, ok := model.(color.Palette); ok {
		return 1
	}
	switch model {
	case color.GrayModel, color.AlphaModel:
		return 1
	case color.Gray16Model, color.Alpha16Model:
		return 2
	case color.YCbCrModel:
		return 3
	case color.RGBAModel, color.NRGBAModel, color.CMYKModel, color.NYCbCrAModel:
		return 4
	default:
		return 8
	}
}
