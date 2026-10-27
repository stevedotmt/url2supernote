package epub

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"sync"

	resvg "github.com/kanrichan/resvg-go"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	svgSniffLength = 2048
	svgFontFamily  = "Go"
)

func isSVG(raw []byte) bool {
	return bytes.Contains(raw[:min(len(raw), svgSniffLength)], []byte("<svg"))
}

type svgRenderer struct {
	mu     sync.Mutex
	worker *resvg.Worker
	fonts  *resvg.FontDB
}

var sharedSVGRenderer svgRenderer

func (r *svgRenderer) ready() error {
	if r.worker != nil {
		return nil
	}
	worker, err := resvg.NewDefaultWorker(context.Background())
	if err != nil {
		return err
	}
	fonts, err := worker.NewFontDBDefault()
	if err != nil {
		_ = worker.Close()
		return err
	}
	for _, face := range [][]byte{goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF} {
		if err := fonts.LoadFontData(face); err != nil {
			_ = worker.Close()
			return err
		}
	}
	for _, set := range []func(string) error{
		fonts.SetSansSerifFamily, fonts.SetSerifFamily, fonts.SetCursiveFamily, fonts.SetFantasyFamily,
	} {
		if err := set(svgFontFamily); err != nil {
			_ = worker.Close()
			return err
		}
	}
	r.worker, r.fonts = worker, fonts
	return nil
}

func (r *svgRenderer) size(raw []byte) (width, height float64, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ready(); err != nil {
		return 0, 0, err
	}
	tree, err := r.worker.NewTreeFromData(raw, &resvg.Options{FontFamily: svgFontFamily})
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tree.Close() }()
	w, h, err := tree.GetSize()
	return float64(w), float64(h), err
}

func (r *svgRenderer) render(raw []byte, width, height int, scale float64) (image.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ready(); err != nil {
		return nil, err
	}
	tree, err := r.worker.NewTreeFromData(raw, &resvg.Options{FontFamily: svgFontFamily})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tree.Close() }()
	if err := tree.ConvertText(r.fonts); err != nil {
		return nil, err
	}
	pixmap, err := r.worker.NewPixmap(uint32(width), uint32(height))
	if err != nil {
		return nil, err
	}
	defer func() { _ = pixmap.Close() }()
	if err := tree.Render(resvg.TransformFromScale(float32(scale), float32(scale)), pixmap); err != nil {
		return nil, err
	}
	encoded, err := pixmap.EncodePNG()
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(encoded))
}

func inspectSVG(raw []byte) (sourceImage, error) {
	if len(raw) > maxSVGBytes {
		return sourceImage{}, errImageSkipped
	}
	srcWidth, srcHeight, err := sharedSVGRenderer.size(raw)
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
		decode: func() (image.Image, error) {
			return sharedSVGRenderer.render(raw, int(width), int(height), scale)
		},
	}, nil
}
