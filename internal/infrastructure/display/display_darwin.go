//go:build darwin

package display

/*
#cgo LDFLAGS: -framework CoreGraphics -framework Foundation
#include <CoreGraphics/CoreGraphics.h>

// readPrimaryDisplay fills the four out-params with:
//   - cssW / cssH    : logical (point) dimensions of the main display
//   - pxW  / pxH     : physical (device-pixel) dimensions
// Using CGDisplayBounds for points and CGDisplayPixelsWide/High for pixels
// is the canonical way to derive the backing scale on macOS without
// bringing AppKit into the build (we do not want to link Cocoa here).
static void readPrimaryDisplay(int* cssW, int* cssH, int* pxW, int* pxH) {
    CGDirectDisplayID id = CGMainDisplayID();
    CGRect bounds = CGDisplayBounds(id);
    *cssW = (int)bounds.size.width;
    *cssH = (int)bounds.size.height;
    *pxW  = (int)CGDisplayPixelsWide(id);
    *pxH  = (int)CGDisplayPixelsHigh(id);
}
*/
import "C"

// Primary returns the geometry of the main display. The CGo call has no
// failure path — the OS always reports a main display whenever there is at
// least one attached, which is implied for any GUI process.
func Primary() Info {
	var cssW, cssH, pxW, pxH C.int
	C.readPrimaryDisplay(&cssW, &cssH, &pxW, &pxH)
	scale := 1.0
	if cssW > 0 {
		scale = float64(pxW) / float64(cssW)
	}
	return Info{
		CSSWidth:  int(cssW),
		CSSHeight: int(cssH),
		Scale:     scale,
	}
}
