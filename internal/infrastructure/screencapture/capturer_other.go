//go:build !darwin

package screencapture

import (
	"bytes"
	"fmt"
	"image"
	"image/png"

	"github.com/kbinani/screenshot"
)

// kbinaniCapturer is the default non-darwin Capturer.
type kbinaniCapturer struct{}

// New returns the default cross-platform Capturer for non-macOS systems.
func New() Capturer { return &kbinaniCapturer{} }

func (c *kbinaniCapturer) VirtualBounds() image.Rectangle {
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return image.Rect(0, 0, 0, 0)
	}
	bounds := screenshot.GetDisplayBounds(0)
	for i := 1; i < n; i++ {
		bounds = bounds.Union(screenshot.GetDisplayBounds(i))
	}
	return bounds
}

func (c *kbinaniCapturer) CaptureRegion(rect image.Rectangle) ([]byte, error) {
	if rect.Dx() <= 0 || rect.Dy() <= 0 {
		return nil, fmt.Errorf("invalid capture rectangle: %v", rect)
	}
	img, err := screenshot.CaptureRect(rect)
	if err != nil {
		return nil, fmt.Errorf("capture: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// CaptureInteractive on non-darwin platforms is not yet implemented. The
// frontend overlay still works there. We surface a clear error so callers
// can decide whether to fall back to the WebView overlay.
func (c *kbinaniCapturer) CaptureInteractive() ([]byte, error) {
	return nil, fmt.Errorf("interactive capture is not implemented on this platform yet")
}

// CaptureFullScreen grabs the primary display via kbinani/screenshot and
// PNG-encodes the result. Multi-display extension is deliberately deferred
// to keep parity with the macOS "primary only" UX.
func (c *kbinaniCapturer) CaptureFullScreen() ([]byte, error) {
	if screenshot.NumActiveDisplays() == 0 {
		return nil, fmt.Errorf("no active display detected")
	}
	bounds := screenshot.GetDisplayBounds(0)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, fmt.Errorf("capture full screen: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}
