package escpos

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
)

// Image renders an image as a 1-bit raster bitmap and appends a GS v 0
// "print raster bitmap" command. The image is auto-thresholded (any pixel
// with luminance < 128 becomes black). For receipt printers, max width is
// 384 dots (58mm) or 576 dots (80mm); larger images are resized down-only
// by simple scaling.
//
//	GS v 0 m xL xH yL yH d1..dn
//	m = 0: normal density (8 dots/mm)
//
// d1..dn is `(w/8) * h` bytes, MSB-first, row-major.
func (b *Builder) Image(img image.Image, maxWidthDots int) *Builder {
	if maxWidthDots <= 0 {
		maxWidthDots = 576
	}
	src := scaleToWidth(img, maxWidthDots)
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Round width up to a multiple of 8.
	wBytes := (w + 7) / 8
	totalBytes := wBytes * h
	if totalBytes <= 0 {
		return b
	}

	xL := byte(wBytes & 0xFF)
	xH := byte((wBytes >> 8) & 0xFF)
	yL := byte(h & 0xFF)
	yH := byte((h >> 8) & 0xFF)

	b.buf.Write([]byte{GS, 'v', '0', 0x00, xL, xH, yL, yH})

	raster := make([]byte, totalBytes)
	for y := 0; y < h; y++ {
		rowBase := y * wBytes
		for x := 0; x < w; x++ {
			r, g, bl, _ := src.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			lum := (299*r + 587*g + 114*bl) / 1000
			if lum>>8 < 128 {
				byteIdx := rowBase + x/8
				bitPos := 7 - (x % 8)
				raster[byteIdx] |= 1 << bitPos
			}
		}
	}
	b.buf.Write(raster)
	return b
}

// ImageFromReader is a convenience: decodes PNG/JPEG bytes and prints.
func (b *Builder) ImageFromReader(r io.Reader, maxWidthDots int) error {
	img, _, err := image.Decode(r)
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}
	b.Image(img, maxWidthDots)
	return nil
}

// scaleToWidth returns a copy of the image scaled (nearest-neighbor) so its
// width does not exceed maxW. If it already fits, returns the original.
func scaleToWidth(img image.Image, maxW int) image.Image {
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= maxW {
		return img
	}
	ratio := float64(maxW) / float64(srcW)
	dstW := maxW
	dstH := int(float64(srcH) * ratio)
	if dstH < 1 {
		dstH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			sx := int(float64(x) / ratio)
			sy := int(float64(y) / ratio)
			if sx >= srcW {
				sx = srcW - 1
			}
			if sy >= srcH {
				sy = srcH - 1
			}
			dst.Set(x, y, img.At(bounds.Min.X+sx, bounds.Min.Y+sy))
		}
	}
	return dst
}

// silence unused color import on builds that strip image decoders.
var _ = color.Black
