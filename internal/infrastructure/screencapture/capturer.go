// Package screencapture defines the cross-platform Capturer abstraction.
//
// Per-platform implementations live in sibling files:
//   - capturer_darwin.go    macOS, uses /usr/sbin/screencapture
//   - capturer_other.go     non-darwin, uses kbinani/screenshot
//
// The interface is kept here so callers do not need build tags.
package screencapture

import "image"

// Capturer is the abstraction that the application service depends on.
type Capturer interface {
	// CaptureRegion grabs the pixels within the supplied virtual-screen
	// rectangle and returns PNG-encoded bytes.
	CaptureRegion(rect image.Rectangle) ([]byte, error)

	// VirtualBounds returns the union rectangle of every connected display.
	VirtualBounds() image.Rectangle

	// CaptureInteractive opens a system-provided region picker (e.g. macOS's
	// `screencapture -i`) and returns the resulting PNG bytes once the user
	// confirms a selection. If the user cancels (Esc / right-click), it
	// returns (nil, ErrCancelled) so callers can short-circuit cleanly.
	CaptureInteractive() ([]byte, error)

	// CaptureFullScreen grabs the entire primary display silently and
	// returns the PNG bytes. Used as the backdrop for the in-app Snipaste-
	// style overlay; the per-region crop happens later in CropPNG once the
	// user finishes dragging.
	CaptureFullScreen() ([]byte, error)
}

// ErrCancelled signals that the user aborted an interactive capture.
// It is a sentinel value; callers should compare via errors.Is.
type cancelled struct{}

func (cancelled) Error() string { return "capture cancelled by user" }

// ErrCancelled is returned from CaptureInteractive when the user dismisses
// the system selection UI without confirming a region.
var ErrCancelled = cancelled{}
