package main

// 16x16 RGBA-encoded ICO icons. Tiny printer pictograms — a filled 12x10
// rectangle with a 12x3 paper feed strip on top. Color encodes status:
// green = healthy, red = agent unreachable.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

var (
	iconGreen = makeIcon(color.RGBA{R: 32, G: 156, B: 76, A: 255})
	iconRed   = makeIcon(color.RGBA{R: 204, G: 41, B: 54, A: 255})
)

func makeIcon(fill color.RGBA) []byte {
	const size = 16
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	// Paper strip on top (3 high)
	for y := 1; y < 4; y++ {
		for x := 3; x < 13; x++ {
			img.Set(x, y, color.White)
		}
	}
	// Printer body (10 high)
	for y := 4; y < 14; y++ {
		for x := 2; x < 14; x++ {
			img.Set(x, y, fill)
		}
	}
	// Small status LED dot
	for y := 6; y < 8; y++ {
		for x := 11; x < 13; x++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
