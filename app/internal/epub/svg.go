package epub

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"strings"
	"time"

	resvg "github.com/kanrichan/resvg-go"
	"github.com/tetratelabs/wazero"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/sync/semaphore"
)

const (
	svgSniffLength      = 2048
	svgFontFamily       = "Go"
	svgTimeout          = 10 * time.Second
	svgMemoryLimitPages = 4096
	maxConcurrentSVGs   = 1
)

func isSVG(raw []byte) bool {
	return bytes.Contains(raw[:min(len(raw), svgSniffLength)], []byte("<svg"))
}

type svgRenderer struct {
	slots   *semaphore.Weighted
	session *svgSession
}

var (
	sharedSVGRenderer = svgRenderer{slots: semaphore.NewWeighted(maxConcurrentSVGs)}
	svgCompiledModule = wazero.NewCompilationCache()
)

type svgSession struct {
	worker *resvg.Worker
	fonts  *resvg.FontDB
	kill   context.CancelFunc
}

func newSVGSession() (*svgSession, error) {
	ctx, kill := context.WithCancel(context.Background())
	config := wazero.NewRuntimeConfig().
		WithCompilationCache(svgCompiledModule).
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(svgMemoryLimitPages)
	worker, err := resvg.NewWorker(ctx, config)
	if err != nil {
		kill()
		return nil, err
	}
	session := &svgSession{worker: worker, kill: kill}

	fonts, err := worker.NewFontDBDefault()
	if err != nil {
		session.discard()
		return nil, err
	}
	for _, face := range [][]byte{goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF} {
		if err := fonts.LoadFontData(face); err != nil {
			session.discard()
			return nil, err
		}
	}
	for _, set := range []func(string) error{
		fonts.SetSansSerifFamily, fonts.SetSerifFamily, fonts.SetCursiveFamily, fonts.SetFantasyFamily,
	} {
		if err := set(svgFontFamily); err != nil {
			session.discard()
			return nil, err
		}
	}
	session.fonts = fonts
	return session, nil
}

func (s *svgSession) discard() {
	s.kill()
	_ = s.worker.Close()
}

func (r *svgRenderer) run(ctx context.Context, work func(*svgSession) error) error {
	waitCtx, cancelWait := context.WithTimeout(ctx, svgTimeout)
	defer cancelWait()
	if err := r.slots.Acquire(waitCtx, 1); err != nil {
		return err
	}
	defer r.slots.Release(1)

	if r.session == nil {
		session, err := newSVGSession()
		if err != nil {
			return err
		}
		r.session = session
	}
	session := r.session

	ctx, cancel := context.WithTimeout(ctx, svgTimeout)
	defer cancel()
	stop := context.AfterFunc(ctx, session.kill)
	err := work(session)
	if !stop() || err != nil {
		session.discard()
		r.session = nil
	}
	return err
}

func (r *svgRenderer) size(ctx context.Context, raw []byte) (width, height float64, err error) {
	err = r.run(ctx, func(s *svgSession) error {
		tree, err := s.worker.NewTreeFromData(raw, &resvg.Options{FontFamily: svgFontFamily})
		if err != nil {
			return err
		}
		defer func() { _ = tree.Close() }()
		w, h, err := tree.GetSize()
		width, height = float64(w), float64(h)
		return err
	})
	return width, height, err
}

func (r *svgRenderer) render(ctx context.Context, raw []byte, width, height int, scale float64) (img image.Image, err error) {
	err = r.run(ctx, func(s *svgSession) error {
		tree, err := s.worker.NewTreeFromData(raw, &resvg.Options{FontFamily: svgFontFamily})
		if err != nil {
			return err
		}
		defer func() { _ = tree.Close() }()
		if err := tree.ConvertText(s.fonts); err != nil {
			return err
		}
		pixmap, err := s.worker.NewPixmap(uint32(width), uint32(height))
		if err != nil {
			return err
		}
		defer func() { _ = pixmap.Close() }()
		if err := tree.Render(resvg.TransformFromScale(float32(scale), float32(scale)), pixmap); err != nil {
			return err
		}
		encoded, err := pixmap.EncodePNG()
		if err != nil {
			return err
		}
		img, err = png.Decode(bytes.NewReader(encoded))
		return err
	})
	return img, err
}

// checkSVGSelfContained rejects SVGs that could make the renderer read files: resvg-go gives the wasm module
// the host's working directory as its filesystem and cannot be given an os.Root, so any href other than a
// fragment or data URI, and any entity declaration, is refused rather than rendered.
func checkSVGSelfContained(raw []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	for {
		token, err := decoder.RawToken()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: %w", errImageSkipped, err)
		}
		switch t := token.(type) {
		case xml.Directive:
			if bytes.Contains(bytes.ToUpper(t), []byte("<!ENTITY")) {
				return fmt.Errorf("%w: SVG declares entities", errImageSkipped)
			}
		case xml.StartElement:
			for _, a := range t.Attr {
				if a.Name.Local == "href" && !strings.HasPrefix(a.Value, "#") && !hasDataPrefix(a.Value) {
					return fmt.Errorf("%w: SVG references an external resource", errImageSkipped)
				}
			}
		}
	}
}

func hasDataPrefix(href string) bool {
	trimmed := strings.TrimSpace(href)
	return len(trimmed) >= 5 && strings.EqualFold(trimmed[:5], "data:")
}

func inspectSVG(ctx context.Context, raw []byte) (sourceImage, error) {
	if len(raw) > maxSVGBytes {
		return sourceImage{}, errImageSkipped
	}
	if err := checkSVGSelfContained(raw); err != nil {
		return sourceImage{}, err
	}
	srcWidth, srcHeight, err := sharedSVGRenderer.size(ctx, raw)
	if err != nil {
		return sourceImage{}, fmt.Errorf("parse SVG: %w", err)
	}
	if !(srcWidth > 0 && srcHeight > 0) || math.IsInf(srcWidth, 0) || math.IsInf(srcHeight, 0) {
		return sourceImage{}, errImageSkipped
	}

	scale := min(1, MaxImageWidth/srcWidth)
	width := math.Max(math.Round(srcWidth*scale), 1)
	height := math.Max(math.Round(srcHeight*scale), 1)
	if pixels := width * height; math.IsNaN(pixels) || pixels > maxImagePixels {
		return sourceImage{}, errImageSkipped
	}
	return sourceImage{
		width:         int(width),
		height:        int(height),
		bytesPerPixel: 8,
		decode: func(ctx context.Context) (image.Image, error) {
			return sharedSVGRenderer.render(ctx, raw, int(width), int(height), scale)
		},
	}, nil
}
