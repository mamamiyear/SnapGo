package screencapture

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

// CropPNG decodes the supplied PNG, crops it to rect (in PHYSICAL pixels of
// the source image — i.e. the same coordinate system the PNG itself uses),
// and re-encodes the result.
//
// Why an extra abstraction: the overlay flow captures the entire screen as
// one big PNG and lets the user pick a region in the WebView. The selection
// rectangle the WebView reports is in CSS pixels, so callers must scale by
// the display's backing factor before invoking CropPNG. Splitting "scale"
// from "crop" keeps this helper focused and unit-testable.
func CropPNG(pngBytes []byte, rect image.Rectangle) ([]byte, error) {
	if rect.Dx() <= 0 || rect.Dy() <= 0 {
		return nil, fmt.Errorf("invalid crop rectangle: %v", rect)
	}
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("decode source png: %w", err)
	}
	// Clip the requested rectangle to the source bounds — defensive guard
	// against off-by-one errors from CSS-px → physical-px scaling.
	src := img.Bounds()
	clipped := rect.Intersect(src)
	if clipped.Empty() {
		return nil, fmt.Errorf("crop rectangle %v has no overlap with source %v", rect, src)
	}

	// SubImage is documented on most concrete image types; the standard
	// library decodes PNGs into one of those, so this assertion is safe in
	// practice. Fall back to a manual copy otherwise.
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	var cropped image.Image
	if si, ok := img.(subImager); ok {
		cropped = si.SubImage(clipped)
	} else {
		dst := image.NewRGBA(image.Rect(0, 0, clipped.Dx(), clipped.Dy()))
		for y := 0; y < clipped.Dy(); y++ {
			for x := 0; x < clipped.Dx(); x++ {
				dst.Set(x, y, img.At(clipped.Min.X+x, clipped.Min.Y+y))
			}
		}
		cropped = dst
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, cropped); err != nil {
		return nil, fmt.Errorf("encode cropped png: %w", err)
	}
	return buf.Bytes(), nil
}
