// Package display exposes platform-specific helpers for reading the
// geometry of the primary display, in both CSS (logical) and PHYSICAL
// pixels. The overlay flow needs both: CSS pixels to size the Wails window
// to "fullscreen" (since Wails uses CSS-px-equivalent units), and the
// backing scale factor to translate the user's CSS-px selection rectangle
// into the physical-pixel rectangle of the captured PNG.
package display

// Info describes the primary display.
//
// The struct is deliberately minimal: only the few fields the overlay flow
// actually needs. We can grow it later without affecting callers because
// it is returned by value.
type Info struct {
	// CSSWidth / CSSHeight are the logical dimensions of the primary
	// display in points (i.e. what CSS / Wails uses).
	CSSWidth  int
	CSSHeight int

	// Scale is backing factor (CSS-px → device-px). 2 on Retina, 1 on
	// non-Retina. We model it as float64 to leave room for fractional
	// scales (e.g. Linux 1.25×) even though macOS effectively only ever
	// reports 1 or 2.
	Scale float64
}
