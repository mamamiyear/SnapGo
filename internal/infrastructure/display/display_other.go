//go:build !darwin

package display

import "github.com/kbinani/screenshot"

// Primary returns the dimensions of display 0. We currently assume a 1× DPR
// because Wails on Windows / Linux already paints in physical pixels, so
// CSS coordinates and capture coordinates coincide. If a user ever runs at
// non-1× we will add a platform-specific scale lookup.
func Primary() Info {
	if screenshot.NumActiveDisplays() == 0 {
		return Info{CSSWidth: 1440, CSSHeight: 900, Scale: 1}
	}
	b := screenshot.GetDisplayBounds(0)
	return Info{
		CSSWidth:  b.Dx(),
		CSSHeight: b.Dy(),
		Scale:     1,
	}
}
