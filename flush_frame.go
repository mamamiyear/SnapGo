// flush_frame.go provides a tiny helper that yields the current goroutine
// long enough for the OS compositor to redraw after WindowHide / Unfullscreen.
//
// Without this, the screenshot can include the just-hidden overlay window
// because Cocoa hasn't redrawn yet by the time we ask for the pixels.
package main

import "time"

func flushFrame() {
	// 80ms is empirically enough on Apple Silicon at 120Hz; on slower devices
	// the worst case is a single redraw delay, still imperceptible to the user.
	time.Sleep(80 * time.Millisecond)
}
