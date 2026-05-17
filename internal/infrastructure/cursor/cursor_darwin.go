//go:build darwin

// cursor_darwin.go reads the global cursor position via Quartz Event
// Services. This API is preferable to NSEvent.mouseLocation for our use
// case because it returns the pointer in flipped (top-left origin)
// coordinates, which matches Wails / web pixel coordinates exactly.
package cursor

/*
#cgo LDFLAGS: -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>

// readCursor returns the current mouse position with origin at the
// top-left of the primary display, in logical pixels.
static void readCursor(double* x, double* y) {
    CGEventRef event = CGEventCreate(NULL);
    CGPoint p = CGEventGetLocation(event);
    if (event) CFRelease(event);
    *x = p.x;
    *y = p.y;
}
*/
import "C"

// Get returns the current mouse pointer position.
func Get() (Point, error) {
	var x, y C.double
	C.readCursor(&x, &y)
	return Point{X: int(x), Y: int(y)}, nil
}
