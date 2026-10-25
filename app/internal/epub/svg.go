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

func inspectSVG(raw []byte) (sourceImage, error) {
	if len(raw) > maxSVGBytes {
		return sourceImage{}, errImageSkipped
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(raw), oksvg.WarnErrorMode)
	if err != nil {
		return sourceImage{}, err
	}
	if icon.ViewBox.W <= 0 || icon.ViewBox.H <= 0 {
		return sourceImage{}, errImageSkipped
	}

	scale := min(1, MaxImageWidth/icon.ViewBox.W)
	width := math.Round(icon.ViewBox.W * scale)
	height := math.Round(icon.ViewBox.H * scale)
	if pixels := width * height; math.IsNaN(pixels) || pixels > maxImagePixels {
		return sourceImage{}, errImageSkipped
	}
	return sourceImage{
		width:         int(width),
		height:        int(height),
		bytesPerPixel: 4,
		decode: func() (image.Image, error) {
			return rasterizeSVG(icon, int(width), int(height)), nil
		},
	}, nil
}

func rasterizeSVG(icon *oksvg.SvgIcon, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	icon.SetTarget(0, 0, float64(width), float64(height))
	icon.Draw(rasterx.NewDasher(width, height, rasterx.NewScannerGV(width, height, dst, dst.Bounds())), 1)
	return dst
}
