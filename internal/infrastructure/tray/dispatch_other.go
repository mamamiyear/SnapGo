//go:build !darwin

package tray

// dispatchOnMain is a no-op on non-darwin platforms because:
//   - On Linux (GTK/AppIndicator), systray.Run / nativeStart can be invoked
//     from any goroutine; the GTK runloop is started internally.
//   - On Windows, status icons are tied to a hidden window owned by the
//     systray library itself, also independent of the Go main goroutine.
// Wrapping the call in dispatchOnMain on these platforms would just add a
// pointless layer of indirection.
func dispatchOnMain(fn func()) {
	if fn != nil {
		fn()
	}
}
