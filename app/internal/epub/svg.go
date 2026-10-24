package epub

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

const svgSniffLength = 2048

func isSVG(raw []byte) bool {
	return bytes.Contains(raw[:min(len(raw), svgSniffLength)], []byte("<svg"))
}

func rasterizeSVG(raw []byte) (image.Image, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(raw), oksvg.WarnErrorMode)
	if err != nil {
		return nil, err
	}
	if icon.ViewBox.W <= 0 || icon.ViewBox.H <= 0 {
		return nil, errImageSkipped
	}

	scale := min(1, MaxImageWidth/icon.ViewBox.W)
	width := int(math.Round(icon.ViewBox.W * scale))
	height := int(math.Round(icon.ViewBox.H * scale))
	if width < minImageSide || height < minImageSide || width*height > maxImagePixels {
		return nil, errImageSkipped
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	icon.SetTarget(0, 0, float64(width), float64(height))
	icon.Draw(rasterx.NewDasher(width, height, rasterx.NewScannerGV(width, height, dst, dst.Bounds())), 1)
	return dst, nil
}
